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
