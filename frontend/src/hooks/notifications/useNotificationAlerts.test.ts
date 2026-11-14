import { renderHook } from '@testing-library/react';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { useNotificationAlerts, alertsMockEnabled } from './useNotificationAlerts';
import { buildMockAlerts } from '@/lib/notifications/mockAlerts';
import { getAppConfig } from '@/lib/appConfig';
import { useAssets } from '@/hooks/assets';
import { useNotificationRecipients } from './useNotificationRecipients';
import type { NotificationAlertFilters, NotificationRecipient } from '@/types/notifications';
import type { Asset } from '@/types/assets';

vi.mock('@/lib/appConfig', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/appConfig')>()),
  getAppConfig: vi.fn(),
}));
vi.mock('@/hooks/assets', () => ({ useAssets: vi.fn() }));
vi.mock('./useNotificationRecipients', () => ({ useNotificationRecipients: vi.fn() }));

const assets = Array.from(
  { length: 10 },
  (_, i) => ({ id: 1000 + i, name: `Asset ${i}`, external_key: `ASSET-${i}` }) as Asset
);

const recipients = [
  { id: 1, name: 'Ana', email: 'ana@example.com', phone: '+15550000001' },
  { id: 2, name: 'Ben', email: 'ben@example.com' },
  { id: 3, name: 'Cy', phone: '+15550000003' },
] as NotificationRecipient[];

const NOW = new Date('2026-10-21T12:00:00Z');

const setLabel = (environmentLabel: string) =>
  vi.mocked(getAppConfig).mockReturnValue({ environmentLabel });

const run = (filters: NotificationAlertFilters = {}) =>
  renderHook(() => useNotificationAlerts(filters)).result.current;

describe('useNotificationAlerts', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Pin the clock so the hook and buildMockAlerts below share one `now`.
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(NOW);
    // Deployed build: only the environment label can turn mock data on.
    vi.stubEnv('DEV', false);
    setLabel('preview');
    vi.mocked(useAssets).mockReturnValue({ assets, isLoading: false } as unknown as ReturnType<
      typeof useAssets
    >);
    vi.mocked(useNotificationRecipients).mockReturnValue({
      recipients,
      isLoading: false,
    } as unknown as ReturnType<typeof useNotificationRecipients>);
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.useRealTimers();
  });

  describe('environment rule', () => {
    it.each(['prod', 'production'])('returns nothing when the label is %s', (label) => {
      setLabel(label);
      expect(alertsMockEnabled()).toBe(false);
      const result = run();
      expect(result.alerts).toEqual([]);
      expect(result.total).toBe(0);
      expect(useAssets).toHaveBeenCalledWith({ enabled: false });
    });

    it.each(['preview', 'staging'])('returns mock alerts when the label is %s', (label) => {
      setLabel(label);
      expect(alertsMockEnabled()).toBe(true);
      expect(run().total).toBeGreaterThanOrEqual(40);
    });

    it('returns mock alerts in local dev with no label', () => {
      vi.stubEnv('DEV', true);
      setLabel('');
      expect(run().total).toBeGreaterThan(0);
    });
  });

  describe('filters', () => {
    const all = buildMockAlerts(assets, recipients, NOW);

    it('narrows by asset', () => {
      const target = all[0].asset_id;
      const result = run({ asset_id: target, per_page: 100 });
      expect(result.total).toBe(all.filter((a) => a.asset_id === target).length);
      expect(result.total).toBeLessThan(all.length);
      expect(result.alerts.every((a) => a.asset_id === target)).toBe(true);
    });

    it.each(['alarm', 'location_change'] as const)('narrows by type %s', (type) => {
      const result = run({ type, per_page: 100 });
      expect(result.total).toBeGreaterThan(0);
      expect(result.total).toBeLessThan(all.length);
      expect(result.alerts.every((a) => a.type === type)).toBe(true);
    });

    it('narrows by recipient', () => {
      const result = run({ recipient_id: 2, per_page: 100 });
      expect(result.total).toBeGreaterThan(0);
      expect(result.alerts.every((a) => a.recipient_id === 2)).toBe(true);
      // Ben has only an email address.
      expect(result.alerts.every((a) => a.channel === 'email')).toBe(true);
    });

    it.each(['email', 'sms'] as const)('narrows by channel %s', (channel) => {
      const result = run({ channel, per_page: 100 });
      expect(result.total).toBeGreaterThan(0);
      expect(result.total).toBeLessThan(all.length);
      expect(result.alerts.every((a) => a.channel === channel)).toBe(true);
      expect(result.alerts.every((a) => (a.subject === null) === (channel === 'sms'))).toBe(true);
    });

    it('narrows by an inclusive sent_at range', () => {
      const sorted = [...all].sort((a, b) => Date.parse(a.sent_at) - Date.parse(b.sent_at));
      const from = sorted[5].sent_at;
      const to = sorted[15].sent_at;
      const result = run({ from, to, per_page: 100 });
      expect(result.total).toBe(
        all.filter((a) => a.sent_at >= from && a.sent_at <= to).length
      );
      expect(result.alerts.map((a) => a.sent_at)).toEqual(expect.arrayContaining([from, to]));
    });
  });

  it('sorts newest sent_at first', () => {
    const times = run({ per_page: 100 }).alerts.map((a) => Date.parse(a.sent_at));
    expect(times).toEqual([...times].sort((a, b) => b - a));
  });

  it('paginates with a default of 20 per page', () => {
    const first = run();
    expect(first.page).toBe(1);
    expect(first.perPage).toBe(20);
    expect(first.alerts).toHaveLength(20);

    const lastPage = Math.ceil(first.total / 20);
    const last = run({ page: lastPage });
    expect(last.total).toBe(first.total);
    expect(last.alerts).toHaveLength(first.total - (lastPage - 1) * 20);

    const ids = new Set([...first.alerts, ...run({ page: 2 }).alerts].map((a) => a.id));
    expect(ids.size).toBe(40);
  });

  describe('buildMockAlerts', () => {
    const now = NOW;

    it('is deterministic for the same input', () => {
      expect(buildMockAlerts(assets, recipients, now)).toEqual(
        buildMockAlerts([...assets].reverse(), recipients, now)
      );
    });

    it('keeps ids unique and sent_at within the last 30 days', () => {
      const rows = buildMockAlerts(assets, recipients, now);
      expect(new Set(rows.map((a) => a.id)).size).toBe(rows.length);
      const earliest = now.getTime() - 30 * 24 * 60 * 60 * 1000;
      expect(rows.every((a) => Date.parse(a.occurred_at) >= earliest)).toBe(true);
    });

    it('falls back to sample assets and contacts when the org has none', () => {
      const rows = buildMockAlerts([], recipients, now);
      expect(rows.length).toBeGreaterThan(0);
      expect(new Set(rows.map((a) => a.asset_name))).toEqual(
        new Set(['Blue Wind RV', 'Sea Breeze 24ft', 'Trailer #12'])
      );
      expect(new Set(rows.map((a) => a.recipient_name))).toEqual(
        new Set(['Maria Nguyen', 'Sam Okafor'])
      );
    });
  });
});
