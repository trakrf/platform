/**
 * A failed `connect()` must not report a disconnect that did not happen.
 *
 * `DeviceManager.create()` has two failure shapes and they need opposite
 * treatment, which the store's single `catch` did not give them:
 *
 *   - **Construction failed.** `create()` destroys its own half-built singleton
 *     (TRA-1250), so `getInstance()` is null and DISCONNECTED is the truth.
 *   - **The guard refused.** `create()` throws `Device already connected. Call
 *     destroy() first.` BEFORE touching anything, so the existing manager, its
 *     worker and its transport are all alive and working.
 *
 * In the second case the old code published DISCONNECTED anyway — telling the
 * UI the reader was gone while it sat there connected, and doing it with no
 * transport event, so neither `link-close` nor `link-teardown` accompanied it.
 *
 * That is TRA-1259's signature exactly: the store loses CONNECTED, the link is
 * intact, and nothing in the run log says why. These tests close the route.
 * They are NOT a claim that this is what produced the observed `hold-sweep`
 * failure — nothing yet establishes that — only that a route to the signature
 * should not exist either way.
 *
 * Refs: TRA-1259.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { useDeviceStore } from './deviceStore';
import { DeviceManager } from '@/lib/device/device-manager';
import { ReaderState } from '@/worker/types/reader';

vi.mock('@/lib/device/device-manager', () => ({
  DeviceManager: {
    create: vi.fn(),
    getInstance: vi.fn()
  }
}));

// `@/lib/openreplay` is deliberately NOT mocked. `createStoreWithTracking`
// reaches it for `getZustandPlugin` at module scope, so a partial mock breaks
// the store's construction rather than isolating it — and the real module is
// inert with no tracker configured, which is the state under vitest.

describe('connect() failure and the reader state it publishes', () => {
  beforeEach(() => {
    useDeviceStore.setState({
      readerState: ReaderState.CONNECTED,
      isConnected: true,
      deviceName: 'CS108'
    });
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.spyOn(console, 'trace').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.clearAllMocks();
  });

  it('restores the reader state when a live manager survives the failure', async () => {
    // The guard-refusal shape: create() threw without touching the singleton.
    vi.mocked(DeviceManager.create).mockRejectedValue(
      new Error('Device already connected. Call destroy() first.')
    );
    vi.mocked(DeviceManager.getInstance).mockReturnValue({} as DeviceManager);

    await expect(useDeviceStore.getState().connect()).rejects.toThrow('Device already connected');

    // THE ASSERTION. A working reader must not be reported as gone.
    expect(useDeviceStore.getState().readerState).toBe(ReaderState.CONNECTED);
    expect(useDeviceStore.getState().isConnected).toBe(true);
  });

  it('does not strand the reader in CONNECTING, which is transient', async () => {
    // `connect()` publishes CONNECTING before it knows whether there is
    // anything to connect. Simply declining to publish DISCONNECTED would
    // leave that behind — and CONNECTING is a state `waitForSettledState` and
    // the e2e trigger helpers both PARK on, so it is worse than the disconnect
    // it replaced. Named separately because the obvious form of this fix has
    // exactly that bug.
    vi.mocked(DeviceManager.create).mockRejectedValue(
      new Error('Device already connected. Call destroy() first.')
    );
    vi.mocked(DeviceManager.getInstance).mockReturnValue({} as DeviceManager);

    await expect(useDeviceStore.getState().connect()).rejects.toThrow();

    expect(useDeviceStore.getState().readerState).not.toBe(ReaderState.CONNECTING);
  });

  it('does not clobber a state the surviving worker published in the meantime', async () => {
    // The live manager's worker keeps emitting. If it has moved the reader on
    // while connect() was failing, that is the truth and the remembered state
    // is stale.
    vi.mocked(DeviceManager.getInstance).mockReturnValue({} as DeviceManager);
    vi.mocked(DeviceManager.create).mockImplementation(async () => {
      useDeviceStore.getState().setReaderState(ReaderState.SCANNING);
      throw new Error('Device already connected. Call destroy() first.');
    });

    await expect(useDeviceStore.getState().connect()).rejects.toThrow();

    expect(useDeviceStore.getState().readerState).toBe(ReaderState.SCANNING);
  });

  it('says so in the log rather than failing silently', async () => {
    // The whole point of this branch is that it deviates from the obvious
    // behaviour. A deviation nobody can see in a run log is how the next
    // person concludes the store simply never updated.
    vi.mocked(DeviceManager.create).mockRejectedValue(
      new Error('Device already connected. Call destroy() first.')
    );
    vi.mocked(DeviceManager.getInstance).mockReturnValue({} as DeviceManager);

    await expect(useDeviceStore.getState().connect()).rejects.toThrow();

    expect(console.warn).toHaveBeenCalledWith(
      expect.stringContaining('live DeviceManager remains')
    );
  });

  it('still publishes DISCONNECTED when construction genuinely failed', async () => {
    // The other shape, and the one that must keep working: create() cleaned up
    // after itself, so there is nothing connected and the UI has to say so.
    vi.mocked(DeviceManager.create).mockRejectedValue(new Error('Command rejected'));
    vi.mocked(DeviceManager.getInstance).mockReturnValue(null);

    await expect(useDeviceStore.getState().connect()).rejects.toThrow('Command rejected');

    expect(useDeviceStore.getState().readerState).toBe(ReaderState.DISCONNECTED);
    expect(useDeviceStore.getState().isConnected).toBe(false);
  });

  it('propagates the original failure either way', async () => {
    // The operator needs the reason the connect failed, not a verdict about
    // reader state.
    vi.mocked(DeviceManager.create).mockRejectedValue(new Error('the real cause'));
    vi.mocked(DeviceManager.getInstance).mockReturnValue(null);

    await expect(useDeviceStore.getState().connect()).rejects.toThrow('the real cause');
  });
});
