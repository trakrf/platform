import { renderHook, act, cleanup } from '@testing-library/react';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { useMusterFeed } from './useMusterFeed';
import {
  openMusterStream,
  useMusterStore,
  type MusterStreamCallbacks,
  type MusterStreamHandle,
} from '@/stores/musterStore';
import { useOrgStore } from '@/stores/orgStore';

// Keep the real store reducer + setters; stub only the SSE transport so the test
// can drive frames and assert teardown/reopen across an org switch.
vi.mock('@/stores/musterStore', async (importActual) => {
  const actual = await importActual<typeof import('@/stores/musterStore')>();
  return { ...actual, openMusterStream: vi.fn() };
});

// refreshStatus hits the network on open; neutralize it for these lifecycle tests.
vi.spyOn(useMusterStore.getState(), 'refreshStatus').mockResolvedValue(undefined as never);

interface OpenedStream {
  callbacks: MusterStreamCallbacks;
  handle: MusterStreamHandle & { close: ReturnType<typeof vi.fn> };
}

function setActiveOrg(id: number) {
  useOrgStore.setState({ currentOrg: { id, name: `Org ${id}`, role: 'admin' } as never });
}

describe('useMusterFeed', () => {
  let opened: OpenedStream[];

  beforeEach(() => {
    opened = [];
    vi.mocked(openMusterStream).mockImplementation((opts) => {
      const handle = { close: vi.fn() };
      opened.push({ callbacks: opts.callbacks, handle });
      return handle;
    });
    useMusterStore.setState({
      zones: [],
      personsOnSite: 0,
      event: null,
      connection: 'idle',
      revealUnlocked: false,
      error: null,
    });
    setActiveOrg(1);
  });

  // Unmount between tests; a still-mounted hook would react to the next test's
  // org reset and open a stray stream.
  afterEach(() => cleanup());

  it('opens a stream on mount and sets connecting → live', () => {
    renderHook(() => useMusterFeed());
    expect(opened).toHaveLength(1);
    expect(useMusterStore.getState().connection).toBe('connecting');

    act(() => opened[0].callbacks.onOpen());
    expect(useMusterStore.getState().connection).toBe('live');
  });

  it('tears down the old stream and opens a fresh one when the active org changes', () => {
    renderHook(() => useMusterFeed());
    expect(opened).toHaveLength(1);

    // A snapshot lands for org 1.
    act(() =>
      opened[0].callbacks.onFrames([
        {
          type: 'snapshot',
          data: {
            zones: [{ location_id: 1, name: 'Z', muster_point: false, count: 2 }],
            persons_on_site: 2,
            event: null,
          },
        },
      ]),
    );
    expect(useMusterStore.getState().personsOnSite).toBe(2);

    // Switch orgs.
    act(() => setActiveOrg(2));

    // Old stream closed exactly once, a fresh one opened, and connection is
    // back to connecting — no stale org-1 stream still delivering.
    expect(opened[0].handle.close).toHaveBeenCalledTimes(1);
    expect(opened).toHaveLength(2);
    expect(useMusterStore.getState().connection).toBe('connecting');
  });

  it('closes the stream and goes idle on unmount', () => {
    const { unmount } = renderHook(() => useMusterFeed());
    expect(opened).toHaveLength(1);
    act(() => unmount());
    expect(opened[0].handle.close).toHaveBeenCalledTimes(1);
    expect(useMusterStore.getState().connection).toBe('idle');
  });
});
