import { renderHook, act, cleanup } from '@testing-library/react';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { useReaderFeed } from './useReaderFeed';
import { openReadStream } from '@/lib/readerfeed/stream';
import { useOrgStore } from '@/stores/orgStore';
import type { ReadStreamCallbacks, ReadStreamHandle } from '@/lib/readerfeed/stream';
import type { TagState } from '@/types/readerfeed';

vi.mock('@/lib/readerfeed/stream');

const tag = (over: Partial<TagState> = {}): TagState => ({
  epc: 'EPC-A',
  readerKey: 'dock-1',
  antennaPort: 2,
  firstSeen: 1,
  lastSeen: 1,
  readCount: 1,
  lastRssi: -55,
  rssiAvg: -55,
  rssiMin: -55,
  rssiMax: -55,
  ...over,
});

// Each openReadStream call records its callbacks + a close spy so the test can
// drive events into the live stream and assert teardown on reconnect.
interface OpenedStream {
  callbacks: ReadStreamCallbacks;
  handle: ReadStreamHandle & { close: ReturnType<typeof vi.fn> };
}

function setActiveOrg(id: number) {
  useOrgStore.setState({ currentOrg: { id, name: `Org ${id}`, role: 'admin' } as never });
}

describe('useReaderFeed', () => {
  let opened: OpenedStream[];

  beforeEach(() => {
    opened = [];
    vi.mocked(openReadStream).mockImplementation((opts) => {
      const handle = { close: vi.fn() };
      opened.push({ callbacks: opts.callbacks, handle });
      return handle;
    });
    setActiveOrg(1);
  });

  // Unmount the hook between tests; otherwise a still-mounted instance reacts to
  // the next test's org reset and opens a stray stream.
  afterEach(() => cleanup());

  it('clears the read list and reopens the stream when the active org changes', () => {
    const { result } = renderHook(() => useReaderFeed());

    expect(opened).toHaveLength(1);

    // A read arrives for org 1.
    act(() => {
      opened[0].callbacks.onEvents([
        { type: 'snapshot', data: { tags: [tag()], uniqueTags: 1, readRate: 3 } },
      ]);
    });
    expect(result.current.tags).toHaveLength(1);

    // Switch the active org.
    act(() => setActiveOrg(2));

    // The previous stream is torn down and a fresh one is opened...
    expect(opened[0].handle.close).toHaveBeenCalledTimes(1);
    expect(opened).toHaveLength(2);
    // ...and the stale org-1 reads are gone (no page refresh needed).
    expect(result.current.tags).toHaveLength(0);
  });

  it('keeps the list when the client clock runs ahead of server time (clock-skew backstop)', () => {
    // Repro of the Live Reads flap: a laptop whose clock is ~2 min fast made the
    // backstop compute every tag as 120s "stale" against Date.now() and wipe the
    // map on its 1s tick, then repopulate on the next read — flapping between the
    // empty state and the list. The backstop must use a server-derived clock, not
    // the raw browser clock.
    vi.useFakeTimers();
    try {
      const serverNow = 1_700_000_000_000;
      vi.setSystemTime(serverNow + 120_000); // browser clock 2 minutes fast

      const { result } = renderHook(() => useReaderFeed());

      act(() => {
        opened[0].callbacks.onEvents([
          {
            type: 'snapshot',
            data: { tags: [tag({ lastSeen: serverNow, firstSeen: serverNow })], uniqueTags: 1, readRate: 3 },
          },
        ]);
      });
      expect(result.current.tags).toHaveLength(1);

      // Let the 1s backstop tick fire. Under the old code the tag is dropped here.
      act(() => {
        vi.advanceTimersByTime(1500);
      });
      expect(result.current.tags).toHaveLength(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it('reconnect() tears down the stream, clears the list, and reopens', () => {
    const { result } = renderHook(() => useReaderFeed());
    expect(opened).toHaveLength(1);

    act(() => {
      opened[0].callbacks.onEvents([
        { type: 'snapshot', data: { tags: [tag()], uniqueTags: 1, readRate: 3 } },
      ]);
    });
    expect(result.current.tags).toHaveLength(1);

    // Clear ≈ reconnect: a fresh server session zeroes the per-session counts.
    act(() => result.current.reconnect());

    expect(opened[0].handle.close).toHaveBeenCalledTimes(1);
    expect(opened).toHaveLength(2);
    expect(result.current.tags).toHaveLength(0);
  });
});
