/**
 * A failed connect must leave nothing behind.
 *
 * `DeviceManager.create()` assigns the singleton BEFORE the four things that
 * can still fail — `worker.initialize()`, `setSettings()`, the activeTab
 * subscription, and the opening `setMode()`. When one of them threw, the
 * singleton stayed set, and the guard at the top of `create()` then refused
 * every retry with `Device already connected. Call destroy() first.` The
 * operator was stuck until a page reload. Observed on preview 2026-09-04 over
 * a direct Web Bluetooth link, after `setMode(Inventory)` failed.
 *
 * `TRANSPORT_DISCONNECTED` already destroys the singleton for the same reason,
 * with a comment naming this error — so the fault was only ever reachable
 * through the CONSTRUCTION path, which is what these tests cover.
 *
 * Refs: TRA-1250.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { endpointSymbol } from 'vite-plugin-comlink/symbol';
import { DeviceManager } from './device-manager';
import { TransportFactory } from './transport/transport-factory';

vi.mock('./transport/transport-factory', () => ({
  TransportFactory: { create: vi.fn() }
}));

/** Everything `create()` reaches on the worker, all resolving by default. */
function fakeWorker() {
  return {
    initialize: vi.fn().mockResolvedValue(true),
    disconnect: vi.fn().mockResolvedValue(undefined),
    setMode: vi.fn().mockResolvedValue(undefined),
    setSettings: vi.fn().mockResolvedValue(undefined),
    startScanning: vi.fn().mockResolvedValue(undefined),
    stopScanning: vi.fn().mockResolvedValue(undefined),
    setLogLevel: vi.fn(),
    setRssiDebug: vi.fn(),
    // `setupEventCallback()` reaches through this to install `onmessage`.
    [endpointSymbol]: { onmessage: null }
  };
}

function fakeTransport() {
  return {
    connect: vi.fn().mockResolvedValue({} as MessagePort),
    disconnect: vi.fn().mockResolvedValue(undefined),
    isConnected: vi.fn().mockReturnValue(true),
    isNetworked: vi.fn().mockReturnValue(false)
  };
}

describe('a create() that fails leaves no singleton behind', () => {
  let worker: ReturnType<typeof fakeWorker>;
  let transport: ReturnType<typeof fakeTransport>;

  beforeEach(() => {
    worker = fakeWorker();
    transport = fakeTransport();
    (TransportFactory.create as ReturnType<typeof vi.fn>).mockReturnValue(transport);
    // Injected by vite-plugin-comlink at build time, so it is a global here.
    (globalThis as unknown as { ComlinkWorker: unknown }).ComlinkWorker = vi
      .fn()
      .mockImplementation(() => worker);
  });

  afterEach(async () => {
    await DeviceManager.getInstance()?.destroy().catch(() => {});
    vi.restoreAllMocks();
  });

  /**
   * The four post-assignment stages, each failing the way it actually can.
   *
   * Table-driven because the defect is positional, not specific: anything
   * after the assignment has the same hole, and a test that covered only
   * `setMode` would pass again the moment someone added a fifth step.
   */
  const stages: [string, () => void][] = [
    ['worker.initialize returning false', () => {
      worker.initialize.mockResolvedValue(false);
    }],
    ['worker.initialize rejecting', () => {
      worker.initialize.mockRejectedValue(new Error('initialize blew up'));
    }],
    ['the opening setMode rejecting', () => {
      worker.setMode.mockRejectedValue(
        new Error('Command rejected: Wrong header prefix (0x0000)')
      );
    }],
  ];

  it.each(stages)('clears the singleton when create() fails at %s', async (_name, arrange) => {
    arrange();

    await expect(DeviceManager.create({})).rejects.toThrow();

    expect(
      DeviceManager.getInstance(),
      'a failed create() must not leave a singleton for the guard to trip over'
    ).toBeNull();
  });

  it('lets the next attempt through after a failure — the user-facing symptom', async () => {
    // The whole point. Before this fix the retry died on
    // `Device already connected. Call destroy() first.` and the only way out
    // was a page reload.
    worker.setMode.mockRejectedValueOnce(new Error('Command rejected'));
    await expect(DeviceManager.create({})).rejects.toThrow('Command rejected');

    await expect(DeviceManager.create({})).resolves.toBeInstanceOf(DeviceManager);
    expect(DeviceManager.getInstance()).not.toBeNull();
  });

  it('propagates the original failure, not whatever cleanup hits on the way out', async () => {
    // Cleanup runs against a half-built manager, so it can fail on its own.
    // If it does, the operator must still be told what actually went wrong.
    worker.setMode.mockRejectedValue(new Error('Command rejected: the real cause'));
    worker.disconnect.mockRejectedValue(new Error('cleanup also failed'));

    await expect(DeviceManager.create({})).rejects.toThrow('the real cause');
    expect(DeviceManager.getInstance()).toBeNull();
  });

  it('still tears the transport down when it gives up', async () => {
    worker.setMode.mockRejectedValue(new Error('Command rejected'));

    await expect(DeviceManager.create({})).rejects.toThrow();

    expect(transport.disconnect).toHaveBeenCalled();
  });
});
