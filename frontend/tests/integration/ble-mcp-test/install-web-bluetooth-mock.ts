/**
 * Put ble-mcp-test's Web Bluetooth implementation onto jsdom's navigator.
 *
 * This is the vitest counterpart of the `transformIndexHtml` injection in
 * `vite.config.ts`. Both call the same `injectWebBluetoothMock`, with the same
 * config, against the same bridge — the only difference is how the code gets
 * there: Playwright's page loads the esbuild IIFE from `ble-mcp-test/browser`,
 * and vitest imports the classes from the package root.
 *
 * That symmetry is the point. TRA-1187's premise is that the clients are one
 * contract packaged per runtime, and the test that matters is "the same test
 * runs from vitest as from Playwright". Before this file, integration went
 * `worker -> harness -> RfidReaderTestClient -> flat NodeBleClient API -> bridge`
 * and never touched `CS108BLETransport` at all, so the production transport
 * layer had no automated coverage on any suite.
 *
 * ## Why `injectWebBluetoothMock` rather than `new MockBluetooth(...)`
 *
 * `CS108BLETransport.connect()` reads `navigator.bluetooth` itself — it takes no
 * injection point, by design, because that is what the shipped app does. Handing
 * the harness a `MockBluetooth` instance would mean adding a seam to production
 * code purely so a test could use it, which is precisely the "no test-only
 * hooks" rule in `tests/integration/cs108/INTEGRATION-TEST-PRINCIPLES.md`.
 *
 * `injectWebBluetoothMock` guards on `typeof window === 'undefined'` and returns
 * early — jsdom satisfies that guard, so it works here unmodified.
 *
 * ## Known degradation under the vitest setup, and why it is harmless
 *
 * `test-utils/vitest.setup.ts` makes `fetch` reject to keep the unit suite off
 * the network (TRA-1050). The mock's `testing.getReaderState()` reaches the
 * bridge over `fetch` — but `MockBluetooth.fetchStatus()` catches everything and
 * returns `null`, so it degrades to "nobody could be asked" rather than
 * throwing. Nothing on the connect path uses it. If a test ever needs a real
 * reader-state read, it has to unstub `fetch` for itself and say why.
 */
import { injectWebBluetoothMock } from 'ble-mcp-test';
import { getViteMockConfig } from '../../config/ble-bridge.config';

/**
 * Install the mock, and hand back the teardown.
 *
 * The returned function is not optional politeness. `pool: 'forks'` with
 * `singleFork: true` means every test file in the run shares one jsdom, so a
 * `navigator.bluetooth` left behind outlives the file that installed it — the
 * same hazard `test-utils/bluetoothEnvironment.ts` was written for (TRA-1078).
 * A later file that expects no Web Bluetooth would find a live one pointed at
 * the bridge.
 */
export function installWebBluetoothMock(): () => Promise<void> {
  const config = getViteMockConfig();

  injectWebBluetoothMock(config);

  // Mirror what the vite plugin does after injecting. `CS108BLETransport`
  // reports `isNetworked()` off this flag and nothing else — the mock replaces
  // navigator.bluetooth in place, so without it a bridge link is
  // indistinguishable from a local radio and the worker sizes its fragment
  // reassembly timeout for the wrong link (see FRAGMENT_TIMEOUT_MS in
  // src/worker/cs108/packet.ts).
  window.__webBluetoothBridged = true;

  return async () => {
    // Tear the transport down rather than merely dropping the reference. The
    // mock's own injection path documents why: an orphaned instance keeps its
    // socket open and its listeners attached while navigator.bluetooth points
    // elsewhere, and goes on believing it is connected.
    //
    // AWAIT it. `teardown()` is `Promise<void>` and it releases every device the
    // instance minted, which closes their sockets. Firing it and returning let
    // `cleanup()` resolve with those closes still in flight, so the next spec
    // file's connect raced them and the bridge refused it as `Device is busy`
    // owned by our own previous session — 12 times across the 111-rep TRA-1189
    // soak, with the next connection opening 1ms after the close and the release
    // landing 13-18ms later. That interval is not ours to shrink: it is the
    // bridge's close-processing cost, bound to `_write`'s `finally`, and
    // ble-mcp-test measured the same quantity independently over 997 cycles
    // (median 16ms, p99 21ms, max 30ms) — which is why its `postDisconnectDelay`
    // is 250ms. Awaiting is what collects that wait; discarding the promise
    // skipped it (TRA-1193).
    const installed = (window.navigator as { bluetooth?: unknown }).bluetooth;
    if (installed && typeof (installed as { teardown?: unknown }).teardown === 'function') {
      try {
        await (installed as { teardown: () => Promise<void> }).teardown();
      } catch {
        // The mock documents teardown as idempotent and never-throwing. Do not
        // become the place that breaks it: a teardown that rejects must still
        // leave navigator.bluetooth removed, or the next file finds a live
        // instance pointing at a socket nobody owns. The discarded promise used
        // to hide this as an unhandled rejection; awaiting surfaces it, so it
        // needs an explicit decision rather than an implicit one.
      }
    }

    delete (window.navigator as { bluetooth?: unknown }).bluetooth;
    window.__webBluetoothBridged = undefined;
  };
}
