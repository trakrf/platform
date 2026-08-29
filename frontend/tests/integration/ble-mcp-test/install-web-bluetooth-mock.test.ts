/**
 * The uninstall function returned by `installWebBluetoothMock` must AWAIT the
 * mock's `teardown()`, not fire it and return.
 *
 * TRA-1193. `MockBluetooth.teardown()` is `Promise<void>` — it releases every
 * device the instance minted, which closes their sockets. Discarding that
 * promise lets `cleanup()` resolve while those closes are still in flight, so
 * the next spec file's connect races them and the bridge refuses it as
 * `Device is busy` owned by our own previous session.
 *
 * Measured on the TRA-1189 soak: the next connection opened 1ms after the close,
 * and the bridge's release landed 13-18ms later — 12 times across 111 reps.
 * Independently corroborated by ble-mcp-test's own 997-cycle measurement of
 * socket-close -> device-released (median 16ms, p99 21ms, max 30ms), which is
 * why the mock's `postDisconnectDelay` is 250ms.
 *
 * No hardware: this asserts the uninstall contract, not a BLE link.
 */
import { describe, it, expect } from 'vitest';
import { installWebBluetoothMock } from './install-web-bluetooth-mock';

describe('installWebBluetoothMock uninstall contract (TRA-1193)', () => {
  it('does not resolve until the mock teardown has settled', async () => {
    const uninstall = installWebBluetoothMock();

    // The stub's teardown settles on a TIMER, deliberately.
    //
    // An earlier version of this test gated on a manually-resolved promise and
    // PASSED against the unfixed code: `await undefined` still yields one
    // microtask, and the teardown's continuation happened to land in that same
    // drain. It looked like a control and could not go red. A macrotask cannot
    // be covered by a microtask tick, so only genuinely awaiting the teardown
    // satisfies this.
    let tornDown = false;

    (window.navigator as unknown as { bluetooth: unknown }).bluetooth = {
      teardown: async () => {
        await new Promise<void>((resolve) => setTimeout(resolve, 10));
        tornDown = true;
      },
    };

    await uninstall();

    expect(tornDown).toBe(true);
  });

  it('still removes the injected bluetooth when teardown rejects', async () => {
    const uninstall = installWebBluetoothMock();

    (window.navigator as unknown as { bluetooth: unknown }).bluetooth = {
      teardown: async () => {
        throw new Error('teardown blew up');
      },
    };

    // Idempotent and never throws is the mock's own contract for teardown; the
    // uninstall must not become the place that breaks it, or a failing teardown
    // leaves navigator.bluetooth pointing at a dead instance.
    await expect(uninstall()).resolves.toBeUndefined();
    expect((window.navigator as { bluetooth?: unknown }).bluetooth).toBeUndefined();
  });
});
