import { describe, it, expect } from 'vitest';
import { updateOutcome, UPDATE_WATCH_MS } from './useReaderSoftware';
import type { DaemonUpdateInfo, ReaderSoftware } from '@/types/scandevices';

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
});
