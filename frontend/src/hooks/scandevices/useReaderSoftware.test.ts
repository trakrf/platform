import { describe, it, expect, vi, afterEach } from 'vitest';
import { createElement, type ReactNode } from 'react';
import { renderHook, waitFor, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  updateOutcome,
  readerUpdateInFlight,
  useReaderSoftware,
  UPDATE_POLL_MS,
  UPDATE_WATCH_MS,
} from './useReaderSoftware';
import { scanDevicesApi } from '@/lib/api/scandevices';
import type { DaemonUpdateInfo, ReaderSoftware } from '@/types/scandevices';

vi.mock('@/lib/auth/orgContext', () => ({ ensureOrgContext: vi.fn(() => Promise.resolve()) }));
vi.mock('@/lib/api/scandevices', () => ({
  scanDevicesApi: { getReaderSoftware: vi.fn(), startReaderUpdate: vi.fn() },
}));

const sw = (version: string, update?: DaemonUpdateInfo): ReaderSoftware => ({
  daemon: { version, update },
  supports_update: true,
});

describe('updateOutcome', () => {
  const t0 = 1_000_000;

  it('is watching while the reader is still on the old version or restarting', () => {
    expect(updateOutcome('v2', sw('v1'), t0, t0 + 1000)).toBe('watching');
    expect(updateOutcome('v2', undefined, t0, t0 + 1000)).toBe('watching');
    expect(updateOutcome('v2', sw('v2', { state: 'pending', target: 'v2' }), t0, t0 + 1000)).toBe(
      'watching'
    );
  });

  it('is done once the reader runs the target and has committed it', () => {
    expect(updateOutcome('v2', sw('v2', { state: 'committed', target: 'v2' }), t0, t0 + 1000)).toBe(
      'done'
    );
  });

  it('reports a rollback or failure of this target', () => {
    expect(
      updateOutcome(
        'v2',
        sw('v1', { state: 'rolled_back', target: 'v2', error: 'crash_loop' }),
        t0,
        t0 + 1000
      )
    ).toBe('rolled_back');
    expect(
      updateOutcome(
        'v2',
        sw('v1', { state: 'failed', target: 'v2', error: 'sha256 mismatch' }),
        t0,
        t0 + 1000
      )
    ).toBe('failed');
  });

  it('ignores an outcome recorded for a different, earlier target', () => {
    expect(
      updateOutcome('v3', sw('v1', { state: 'rolled_back', target: 'v2' }), t0, t0 + 1000)
    ).toBe('watching');
  });

  it('does not judge a retry of the same version by a report fetched before it started', () => {
    // The previous attempt at v2 failed; the operator retries v2. The cached
    // report still says failed for v2 until a fresh one arrives.
    const stale = sw('v1', { state: 'failed', target: 'v2', error: 'download: status 404' });
    expect(updateOutcome('v2', stale, t0, t0 + 1000, t0 - 1)).toBe('watching');
    expect(updateOutcome('v2', stale, t0, t0 + 6000, t0 + 5000)).toBe('failed');
  });

  it('times out after the rollback window has certainly passed', () => {
    expect(updateOutcome('v2', sw('v1'), t0, t0 + UPDATE_WATCH_MS + 1)).toBe('timeout');
    expect(UPDATE_WATCH_MS).toBeGreaterThanOrEqual(20 * 60 * 1000);
  });

  it('does not time out on a clock that ran while nobody checked the reader', () => {
    // A background tab: the clock kept running, refetches paused. The last
    // check was long before the deadline, so the reader has not been asked.
    const late = t0 + UPDATE_WATCH_MS + 60_000;
    expect(updateOutcome('v2', sw('v1'), t0, late, t0 + 1000, t0 + 1000)).toBe('watching');
    // A check after the deadline — even a failed one — settles it.
    expect(updateOutcome('v2', sw('v1'), t0, late, t0 + 1000, late)).toBe('timeout');
  });

  it('reports a target left running with nothing to roll back to', () => {
    expect(
      updateOutcome(
        'v2',
        sw('v2', { state: 'unguarded', target: 'v2', error: 'no_commit_no_from' }),
        t0,
        t0 + 1000
      )
    ).toBe('unguarded');
  });
});

describe('readerUpdateInFlight', () => {
  it('is true only while the reader reports an update that has not settled', () => {
    expect(readerUpdateInFlight(sw('v1', { state: 'downloading', target: 'v2' }))).toBe(true);
    expect(readerUpdateInFlight(sw('v1', { state: 'installing', target: 'v2' }))).toBe(true);
    expect(readerUpdateInFlight(sw('v2', { state: 'pending', target: 'v2' }))).toBe(true);
    expect(readerUpdateInFlight(sw('v2', { state: 'committed', target: 'v2' }))).toBe(false);
    expect(readerUpdateInFlight(sw('v1', { state: 'rolled_back', target: 'v2' }))).toBe(false);
    expect(readerUpdateInFlight(sw('v1', { state: 'failed', target: 'v2' }))).toBe(false);
    expect(readerUpdateInFlight(sw('v1'))).toBe(false);
    expect(readerUpdateInFlight(undefined)).toBe(false);
  });
});

describe('useReaderSoftware', () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it('keeps polling an update it did not start until the reader settles it', async () => {
    // Opened mid-update (a reload, or another admin's update): the panel must
    // not freeze on "has not committed yet".
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const get = vi.mocked(scanDevicesApi.getReaderSoftware);
    const reply = (s: ReaderSoftware) => Promise.resolve({ data: { data: s } } as never);
    get
      .mockReturnValueOnce(reply(sw('v2', { state: 'pending', target: 'v2' })))
      .mockReturnValue(reply(sw('v2', { state: 'committed', target: 'v2' })));
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: qc }, children);

    const { result } = renderHook(() => useReaderSoftware(10), { wrapper });
    await waitFor(() => expect(result.current.software?.daemon?.update?.state).toBe('pending'));
    expect(result.current.inFlight).toBe(true);

    await vi.advanceTimersByTimeAsync(UPDATE_POLL_MS + 100);
    await waitFor(() => expect(result.current.software?.daemon?.update?.state).toBe('committed'));
    expect(result.current.inFlight).toBe(false);

    const calls = get.mock.calls.length;
    await vi.advanceTimersByTimeAsync(UPDATE_POLL_MS * 3);
    expect(get.mock.calls.length).toBe(calls);
  });

  function renderSoftware() {
    vi.mocked(scanDevicesApi.getReaderSoftware).mockReturnValue(
      Promise.resolve({ data: { data: sw('v1') } } as never)
    );
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: qc }, children);
    return renderHook(() => useReaderSoftware(10), { wrapper });
  }

  it('watches the reader when the start request reached it but the reply was lost', async () => {
    // 504: Daemon.Update was sent and may be running. Saying "could not start"
    // would be wrong, and not watching would miss its outcome.
    vi.mocked(scanDevicesApi.startReaderUpdate).mockRejectedValue({ response: { status: 504 } });
    const { result } = renderSoftware();
    await waitFor(() => expect(result.current.software).toBeDefined());

    await result.current.startUpdate('v2').catch(() => undefined);
    await waitFor(() => expect(result.current.target).toBe('v2'));
    expect(result.current.outcome).toBe('watching');
    expect(result.current.startError).toBeNull();
  });

  it('does not watch an update that was never sent', async () => {
    vi.mocked(scanDevicesApi.startReaderUpdate).mockRejectedValue({ response: { status: 502 } });
    const { result } = renderSoftware();
    await waitFor(() => expect(result.current.software).toBeDefined());

    await result.current.startUpdate('v2').catch(() => undefined);
    await waitFor(() => expect(result.current.startError).not.toBeNull());
    expect(result.current.target).toBeNull();
    expect(result.current.outcome).toBeNull();
  });
});
