/**
 * Transport Factory
 *
 * There is exactly one transport and one degenerate case, and this file now
 * says so rather than maintaining a selection framework for choices nobody
 * makes (TRA-1177 §5).
 *
 * What was here before: a four-mode enum ('auto' | 'ble' | 'bridge' | 'mock'),
 * a detectMode() consulting TRANSPORT_MODE, __TEST_MODE__ and __BRIDGE_URL__, a
 * switch over all four modes, and an isAvailable() probe. None of it ran in
 * production. The only production caller — deviceStore.ts, via
 * DeviceManager.create — passed mode: 'auto', which is truthy, so
 * `config.mode || this.detectMode()` never evaluated its right-hand side and
 * the switch never left its default. Nothing in the tree ever set any of the
 * three inputs detectMode() consulted, and no test constructed BridgeTransport
 * or MockTransport.
 *
 * Why the fallback had to go rather than merely be tidied: createAutoTransport
 * ended in `return new MockTransport(...)`, which fabricates an 85% battery
 * level and streams three invented EPCs at 100ms intervals. That was reachable
 * by a real user — ScanControls.tsx calls connect() with no browser-support
 * gate, and DeviceManager.create performs no Bluetooth precheck — so on Safari,
 * Firefox or any iOS browser the app reported a healthy scanner that did not
 * exist. Absence of Web Bluetooth is now an error.
 *
 * Under ble-mcp-test the injected mock assigns navigator.bluetooth, so the
 * mocked path and the hardware path are the same branch here. That is what
 * CLAUDE.md describes: the app reaches a CS108 solely via navigator.bluetooth.
 */

import type { Transport } from './Transport';
import { CS108BLETransport, type CS108BLETransportConfig } from './cs108-ble-transport';

export interface TransportFactoryConfig {
  ble?: CS108BLETransportConfig;
}

/**
 * Shown when the browser cannot do Web Bluetooth at all. Deliberately phrased
 * for a user rather than a developer — this reaches a toast.
 */
export const NO_WEB_BLUETOOTH_MESSAGE =
  'This browser does not support Web Bluetooth, so it cannot reach a scanner. ' +
  'Use Chrome, Edge or Opera on desktop and Android, or Bluefy on iOS.';

export class TransportFactory {
  static create(config: TransportFactoryConfig = {}): Transport {
    if (typeof navigator === 'undefined' || !('bluetooth' in navigator)) {
      throw new Error(NO_WEB_BLUETOOTH_MESSAGE);
    }

    return new CS108BLETransport(config.ble);
  }
}
