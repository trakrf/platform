import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import toast from 'react-hot-toast';
import { AssetDetailPanel } from './AssetDetailPanel';
import { reportsApi } from '@/lib/api/reports';
import { generateAssetHistoryCSV } from '@/utils/export';
import { getDateRangeStart } from '@/lib/reports/utils';
import type { AssetHistoryParams, CurrentLocationItem } from '@/types/reports';

vi.mock('@/lib/api/reports');
vi.mock('@/stores/orgStore', () => ({
  useOrgStore: vi.fn((selector) => {
    const state = { currentOrg: { id: 1, name: 'Test Org' } };
    return selector ? selector(state) : state;
  }),
}));
vi.mock('@/hooks/reports/useReportHydration', () => ({
  useReportHydration: () => ({
    getAssetName: () => 'Forklift 7',
    getLocationName: (_id: number | null, key: string | null) => key ?? 'Unknown',
    isHydrating: false,
  }),
}));
vi.mock('@/utils/export', () => {
  const result = () => ({ blob: new Blob(['x']), filename: 'history', mimeType: 'text/csv' });
  return {
    generateAssetHistoryCSV: vi.fn(result),
    generateAssetHistoryExcel: vi.fn(result),
    generateAssetHistoryPDF: vi.fn(result),
  };
});
vi.mock('@/utils/shareUtils', () => ({
  downloadBlob: vi.fn(),
  shareFile: vi.fn(),
  canShareFiles: () => false,
  canShareFormat: () => false,
}));
vi.mock('react-hot-toast', () => ({
  default: { error: vi.fn(), success: vi.fn() },
}));

const NOW = new Date('2026-03-15T12:00:00Z');
const TOTAL = 250;

const asset: CurrentLocationItem = {
  asset_id: 42,
  asset_external_key: 'FL-007',
  location_id: 3,
  location_external_key: 'DOCK-3',
  asset_last_seen: '2026-03-15T11:59:00Z',
  asset_deleted_at: null,
  dwell_started_at: '2026-03-15T09:00:00Z',
  dwell_seconds: 10_740,
};

const rows = (n: number, start: number) =>
  Array.from({ length: n }, (_, i) => ({
    timestamp: '2026-03-10T10:00:00Z',
    location_id: start + i,
    location_external_key: `LOC-${start + i}`,
    duration_seconds: 60,
  }));

// The server holds TOTAL stays; serve whatever slice was asked for.
function serveHistory(_assetId: number, params: AssetHistoryParams = {}) {
  const limit = params.limit ?? 20;
  const offset = params.offset ?? 0;
  const count = Math.max(0, Math.min(limit, TOTAL - offset));
  return Promise.resolve({
    data: { data: rows(count, offset), limit, offset, total_count: TOTAL },
  }) as ReturnType<typeof reportsApi.listAssetHistory>;
}

function renderPanel() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return render(<AssetDetailPanel asset={asset} onClose={vi.fn()} />, { wrapper });
}

// Desktop panel and mobile sheet both render in jsdom; CSS hides one.
const first = (name: string | RegExp) => screen.getAllByRole('button', { name })[0];

describe('AssetDetailPanel history download', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(NOW);
    vi.mocked(reportsApi.listAssetHistory).mockImplementation(serveHistory);
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it('exports every stay in the selected range, not the loaded page', async () => {
    renderPanel();
    fireEvent.click(first('30 Days'));
    await waitFor(() => expect(first(/download history csv/i)).toBeEnabled());

    fireEvent.click(first(/download history csv/i));
    fireEvent.click(await screen.findByRole('button', { name: /^download$/i }));

    const from = getDateRangeStart('30days').toISOString();
    expect(reportsApi.listAssetHistory).toHaveBeenCalledWith(42, {
      from,
      limit: 200,
      offset: 0,
    });
    expect(reportsApi.listAssetHistory).toHaveBeenCalledWith(42, {
      from,
      limit: 200,
      offset: 200,
    });
    expect(generateAssetHistoryCSV).toHaveBeenCalledTimes(1);
    const [exported, opts] = vi.mocked(generateAssetHistoryCSV).mock.calls[0];
    expect(exported).toHaveLength(TOTAL);
    expect(opts).toMatchObject({ assetName: 'Forklift 7', assetKey: 'FL-007' });
  });

  it('shows an error and no export dialog when the history cannot be fetched', async () => {
    renderPanel();
    await waitFor(() => expect(first(/download history csv/i)).toBeEnabled());
    vi.mocked(reportsApi.listAssetHistory).mockRejectedValue(new Error('boom'));

    fireEvent.click(first(/download history csv/i));

    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: /^download$/i })).not.toBeInTheDocument();
    expect(generateAssetHistoryCSV).not.toHaveBeenCalled();
  });
});
