/**
 * CS108 BLE transport - notification handling and link classification.
 *
 * Covers TRA-1148 item 2: handleNotifications must honour the DataView's
 * byteOffset/byteLength rather than copying the whole backing ArrayBuffer.
 */

import { describe, it, expect, afterEach, vi } from 'vitest';
import { CS108BLETransport } from './cs108-ble-transport';

/**
 * Drive the private notification handler with a characteristic whose `value` is
 * the given DataView, and capture what gets posted to the worker.
 */
function deliverNotification(value: DataView): Uint8Array | undefined {
  const transport = new CS108BLETransport();
  let posted: Uint8Array | undefined;

  // Stand in for the MessagePort the worker would be listening on.
  (transport as unknown as { messagePort: { postMessage: (m: unknown) => void } }).messagePort = {
    postMessage: (msg: unknown) => {
      const message = msg as { type: string; data: Uint8Array };
      if (message.type === 'ble:data') posted = message.data;
    }
  };

  const event = { target: { value } } as unknown as Event;
  (transport as unknown as { handleNotifications: (e: Event) => void }).handleNotifications(event);

  return posted;
}

describe('CS108BLETransport notification handling', () => {
  it('forwards an exact-size buffer at offset 0 unchanged', () => {
    // Today's ble-mcp-test path: JSON.parse'd text frame -> new Uint8Array(msg.data),
    // which is always exact-size at offset 0. This is the case that already worked.
    const bytes = new Uint8Array([0xa7, 0xb3, 0x03, 0xc2, 0x82, 0x9e, 0x00, 0x00]);
    const posted = deliverNotification(new DataView(bytes.buffer));

    expect(posted).toEqual(bytes);
  });

  it('forwards only the view when the notification sits inside a larger pooled buffer', () => {
    // The defect: a pooled/shared allocator, a Node Buffer, or a binary/base64 WS
    // frame decoded into a shared buffer hands over a VIEW, not a whole buffer.
    // Copying value.buffer wholesale yielded the entire pool starting at offset 0.
    const pool = new Uint8Array(64).fill(0xff);
    const notification = [0xa7, 0xb3, 0x03, 0xc2, 0x82, 0x9e, 0x12, 0x34];
    pool.set(notification, 16);

    const posted = deliverNotification(new DataView(pool.buffer, 16, notification.length));

    expect(posted).toEqual(new Uint8Array(notification));
    expect(posted?.length).toBe(notification.length);
    // Would have been 64 bytes of mostly 0xff before the fix.
    expect(posted?.[0]).toBe(0xa7);
  });

  it('detaches the copy from the pool so later reuse cannot corrupt it', () => {
    // .slice() must produce an independent buffer: a pooled allocator will
    // overwrite those bytes for the very next notification, and the worker may
    // not have processed this one yet.
    const pool = new Uint8Array(32).fill(0x00);
    pool.set([0xa7, 0xb3, 0x0a], 8);

    const posted = deliverNotification(new DataView(pool.buffer, 8, 3));
    expect(posted).toEqual(new Uint8Array([0xa7, 0xb3, 0x0a]));

    pool.fill(0xee); // pool recycled underneath us
    expect(posted).toEqual(new Uint8Array([0xa7, 0xb3, 0x0a]));
  });

  it('ignores a notification with no value', () => {
    const transport = new CS108BLETransport();
    const post = vi.fn();
    (transport as unknown as { messagePort: { postMessage: unknown } }).messagePort = {
      postMessage: post
    };

    const event = { target: { value: undefined } } as unknown as Event;
    (transport as unknown as { handleNotifications: (e: Event) => void }).handleNotifications(event);

    expect(post).not.toHaveBeenCalled();
  });
});

describe('CS108BLETransport link classification', () => {
  afterEach(() => {
    delete window.__webBluetoothBridged;
  });

  it('reports a real Web Bluetooth link as local', () => {
    expect(new CS108BLETransport().isNetworked()).toBe(false);
  });

  it('reports the injected ble-mcp-test bridge as networked', () => {
    // The mock keeps getType() === 'ble' while every notification actually
    // crosses a WebSocket, so this flag is the only discriminator.
    window.__webBluetoothBridged = true;

    const transport = new CS108BLETransport();
    expect(transport.getType()).toBe('ble');
    expect(transport.isNetworked()).toBe(true);
  });
});

/**
 * Write-failure visibility.
 *
 * Every write-failure path used to converge on silence: the queue entry's
 * `resolve` was built as `() => resolve()`, discarding the success boolean, and
 * two of the three failure paths told the worker nothing at all. The worker
 * therefore could not distinguish "command sent, awaiting ACK" from "command
 * never left", and simply waited out its 5s timeout — surfacing as
 * "Command timeout" no matter what actually went wrong.
 */
type WritePrivates = {
  queueWrite: (d: Uint8Array) => Promise<boolean>;
  commandQueue: unknown[];
  commandInProgress: boolean;
  messagePort: { postMessage: (m: unknown) => void };
};

function transportWithCapturedPort(): { transport: CS108BLETransport; posted: Array<{ type: string; error?: string }> } {
  const transport = new CS108BLETransport();
  const posted: Array<{ type: string; error?: string }> = [];
  (transport as unknown as WritePrivates).messagePort = {
    postMessage: (msg: unknown) => posted.push(msg as { type: string; error?: string })
  };
  return { transport, posted };
}

describe('CS108BLETransport write-failure visibility', () => {
  it('resolves false when the transport is not connected', async () => {
    const { transport } = transportWithCapturedPort();

    const ok = await (transport as unknown as WritePrivates).queueWrite(new Uint8Array([0x01]));

    expect(ok).toBe(false);
  });

  it('tells the worker when a write is dropped because the transport is not connected', async () => {
    const { transport, posted } = transportWithCapturedPort();

    await (transport as unknown as WritePrivates).queueWrite(new Uint8Array([0x01]));

    expect(posted.some(m => m.type === 'ble:error')).toBe(true);
  });

  it('tells the worker when a write is dropped because the queue is full', async () => {
    const { transport, posted } = transportWithCapturedPort();
    const priv = transport as unknown as WritePrivates;
    // Fill to MAX_QUEUE_LENGTH and hold the pump so nothing drains.
    priv.commandInProgress = true;
    priv.commandQueue = [{}, {}, {}, {}, {}];

    const ok = await priv.queueWrite(new Uint8Array([0x01]));

    expect(ok).toBe(false);
    expect(posted.some(m => m.type === 'ble:error')).toBe(true);
  });

  it('treats a GATT server that has dropped the link as not connected', () => {
    const transport = new CS108BLETransport();
    Object.assign(transport as unknown as Record<string, unknown>, {
      device: {},
      server: { connected: false },
      writeCharacteristic: {}
    });

    expect(transport.isConnected()).toBe(false);
  });
});

describe('CS108BLETransport device selection', () => {
  // Restore rather than delete: removing globalThis.navigator outright leaks out of
  // this file and breaks every later test that reads it (HelpScreen's browser guidance).
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('selects the device by service UUID alone, never by name', async () => {
    // Web Bluetooth ORs the filters array, so a second {name} filter WIDENS the
    // match rather than narrowing it. Selection is by service UUID by decision.
    let captured: { filters: Array<Record<string, unknown>> } | undefined;
    vi.stubGlobal('navigator', {
      bluetooth: {
        requestDevice: (opts: { filters: Array<Record<string, unknown>> }) => {
          captured = opts;
          return Promise.reject(new Error('stop here'));
        }
      }
    });

    const transport = new CS108BLETransport({ deviceNameFilter: 'CS108' } as never);
    await expect(transport.connect()).rejects.toThrow();

    expect(captured!.filters).toHaveLength(1);
    expect(captured!.filters[0]).not.toHaveProperty('name');
  });
});
