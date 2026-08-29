/**
 * Raw CS108 command/response over the production transport's MessagePort.
 *
 * This replaces `RfidReaderTestClient`, which wrapped ble-mcp-test's flat
 * `NodeBleClient` API (`connect` / `onNotification` / `sendCommandAsync` /
 * client-level `writeValue`). None of those have a Web Bluetooth counterpart —
 * they are a second, parallel client surface that the browser path never had.
 *
 * ## Why the MessagePort rather than the characteristic
 *
 * These specs are byte-level: they send firmware commands and read the raw
 * replies, with no worker involved. The obvious move would be to write straight
 * to the notify/write characteristics. Going through the transport's port
 * instead is deliberate:
 *
 *  - it satisfies TRA-1187 item 3 literally — the bridge is reached *through*
 *    `CS108BLETransport`, not merely through some Web Bluetooth object
 *  - it exercises the transport's write queue, retry budget and DataView
 *    handling, which is where the interesting transport bugs actually live
 *    (see WRITE_BUDGET_MS and handleNotifications in cs108-ble-transport.ts)
 *  - it keeps both integration suites on exactly one path to the hardware
 *
 * The port is `CS108BLETransport`'s real public interface: it posts
 * `{ type: 'ble:write', data }` in, and `{ type: 'ble:data', data }` comes back.
 *
 * ## On `sendCommand`: this is TRA-1187 item 4, answered
 *
 * `sendCommandAsync` was the one piece of the flat API with real behaviour
 * behind it — request/response correlation — and the ticket asks explicitly
 * whether to promote it into the client contract or delete it.
 *
 * The answer taken here is **neither: it moves into platform's test tooling.**
 * Correlation is not a Web Bluetooth concept. Real GATT gives you a write and a
 * notification stream with nothing joining them, and any correlation is the
 * device protocol's business — for the CS108 that is the worker's
 * `CommandManager`, which is a far more capable version of this and is what
 * production actually uses. Putting correlation in the client contract would
 * oblige every future packaging (pytest included) to reimplement a CS108-shaped
 * idea that Web Bluetooth does not have, which also cuts against TRA-1188
 * removing CS108 knowledge from ble-mcp-test entirely.
 *
 * So `sendCommand` below is deliberately dumb: write, then take the next
 * inbound frame. That is all `sendCommandAsync` did for these specs, and it is
 * honest about being a smoke-test convenience rather than a protocol.
 */

import { CS108BLETransport } from '@/lib/device/transport/cs108-ble-transport';
import { installWebBluetoothMock } from './install-web-bluetooth-mock';

interface BLEPortMessage {
  type: string;
  data?: Uint8Array;
}

export class TransportCommandClient {
  private transport: CS108BLETransport | null = null;
  private port: MessagePort | null = null;
  private uninstallMock: (() => Promise<void>) | null = null;

  /**
   * Waiters queued by `sendCommand`, oldest first.
   *
   * A frame arriving with no waiter is delivered to `onNotification` and then
   * dropped, which is correct for this client: the CS108 streams unsolicited
   * battery, trigger and inventory frames continuously, so buffering them
   * against some future `sendCommand` would mean answering a command with a
   * notification that predates it.
   */
  private waiters: Array<(data: Uint8Array) => void> = [];
  private notificationCallback?: (data: Uint8Array) => void;

  async connect(): Promise<void> {
    this.uninstallMock = installWebBluetoothMock();

    this.transport = new CS108BLETransport();
    this.port = await this.transport.connect();

    // Assigning onmessage starts the port; addEventListener would need start().
    this.port.onmessage = (event: MessageEvent<BLEPortMessage>) => {
      const message = event.data;
      if (message?.type !== 'ble:data' || !message.data) {
        return;
      }

      const data = message.data;
      this.notificationCallback?.(data);

      // Hand it to the oldest waiter. With none, it has already gone to
      // onNotification and is dropped — see the `waiters` comment.
      this.waiters.shift()?.(data);
    };
  }

  /** Observe every inbound frame, correlated or not. */
  onNotification(callback: (data: Uint8Array) => void): void {
    this.notificationCallback = callback;
  }

  /**
   * Send bytes and resolve with the next inbound frame.
   *
   * Smoke-test convenience, not a protocol — see the file header. "Next frame"
   * is genuinely next-in-time, so an unsolicited battery notification landing
   * between the write and the reply will be what this resolves with.
   */
  async sendCommand(command: Uint8Array, timeoutMs: number = 5000): Promise<Uint8Array> {
    const port = this.requirePort();

    const response = new Promise<Uint8Array>((resolve, reject) => {
      const timer = setTimeout(() => {
        const index = this.waiters.indexOf(waiter);
        if (index !== -1) {
          this.waiters.splice(index, 1);
        }
        reject(
          new Error(
            `No response within ${timeoutMs}ms to ` +
              `${Array.from(command, (b) => b.toString(16).padStart(2, '0')).join(' ')}`
          )
        );
      }, timeoutMs);

      const waiter = (data: Uint8Array) => {
        clearTimeout(timer);
        resolve(data);
      };
      this.waiters.push(waiter);
    });

    port.postMessage({ type: 'ble:write', data: command } as BLEPortMessage);
    return response;
  }

  /** Send bytes and do not wait. Responses arrive via `onNotification`. */
  sendRawBytes(command: Uint8Array): void {
    this.requirePort().postMessage({ type: 'ble:write', data: command } as BLEPortMessage);
  }

  isConnected(): boolean {
    return this.transport?.isConnected() ?? false;
  }

  async disconnect(): Promise<void> {
    try {
      await this.transport?.disconnect();
    } finally {
      this.transport = null;
      this.port = null;
      this.waiters = [];
      // AWAIT it — the mock's teardown closes the socket and waits its measured
      // post-disconnect delay. Discarding the promise let the next connect race
      // the release and be refused as busy by this same session (TRA-1193).
      await this.uninstallMock?.();
      this.uninstallMock = null;
    }
  }

  private requirePort(): MessagePort {
    if (!this.port) {
      throw new Error('Not connected - call connect() first');
    }
    return this.port;
  }
}
