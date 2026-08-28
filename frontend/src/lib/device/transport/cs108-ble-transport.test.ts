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

/**
 * TRA-1179 — teardown ownership.
 *
 * There are two ways down: the explicit `disconnect()` and the
 * `gattserverdisconnected` handler. They cleared different amounts. `disconnect()`
 * deleted `window.__TRANSPORT_MANAGER__`; `handleDisconnect()` -> `cleanup()` did
 * not, so after an *unexpected* drop the e2e trigger helpers kept injecting into
 * an orphaned characteristic.
 *
 * Observed live, not theorised: the 0.8.0-rc.1 hardware run produced three
 * `NOTIFY_CHAR_NOT_FOUND: No notify characteristic found in transport manager`
 * retries and failed `locate.spec.ts:184`.
 *
 * The invariant this pins: **cleanup() owns everything disconnect() owns.**
 * Sibling teardown paths that clear different amounts is how they drift.
 */
describe('CS108BLETransport teardown ownership', () => {
  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).__TRANSPORT_MANAGER__;
  });

  it('clears __TRANSPORT_MANAGER__ on cleanup, not only on explicit disconnect', async () => {
    const transport = new CS108BLETransport();
    const w = window as unknown as Record<string, unknown>;
    w.__TRANSPORT_MANAGER__ = { notifyCharacteristic: {} };

    await (transport as unknown as { cleanup: () => Promise<void> }).cleanup();

    expect(w.__TRANSPORT_MANAGER__).toBeUndefined();
  });

  it('clears it on an unexpected GATT drop, which routes through handleDisconnect', () => {
    const transport = new CS108BLETransport();
    const w = window as unknown as Record<string, unknown>;
    w.__TRANSPORT_MANAGER__ = { notifyCharacteristic: {} };

    (transport as unknown as { handleDisconnect: () => void }).handleDisconnect();

    expect(w.__TRANSPORT_MANAGER__).toBeUndefined();
  });
});

/**
 * TRA-1179 — a failing `stopNotifications()` must not vanish.
 *
 * `disconnect()` wrapped it in `try { … } catch { /* comment *\/ }` — an empty
 * catch. That was harmless only because the ble-mcp-test mock's
 * `stopNotifications()` is a no-op that returns `this` and cannot fail.
 *
 * TRA-1153 makes it a real subscription gate, at which point it can reject and
 * the empty catch would discard it in silence: no log, no throw, no failed test.
 * That is CLAUDE.md's first bug class, dormant, waiting for a precondition that
 * a correctness fix elsewhere is about to supply.
 *
 * Teardown must still complete — the requirement is visibility, not propagation.
 * Same shape as #583 for writes: it did not make writes blocking, it made
 * failures observable.
 */
describe('CS108BLETransport teardown error visibility', () => {
  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).__TRANSPORT_MANAGER__;
  });

  function transportMidDisconnect(stopNotifications: () => Promise<void>) {
    const transport = new CS108BLETransport();
    const posted: Array<{ type: string; error?: string }> = [];

    Object.assign(transport as unknown as Record<string, unknown>, {
      notifyCharacteristic: { stopNotifications, removeEventListener: () => {} },
      device: { removeEventListener: () => {}, gatt: { connected: false } },
      messagePort: {
        postMessage: (m: unknown) => posted.push(m as { type: string; error?: string }),
        close: () => {}
      }
    });

    return { transport, posted };
  }

  it('reports a rejecting stopNotifications instead of swallowing it', async () => {
    const { transport, posted } = transportMidDisconnect(() =>
      Promise.reject(new Error('transport gone'))
    );

    await transport.disconnect();

    const err = posted.find(m => m.type === 'ble:error');
    expect(err).toBeDefined();
    expect(err?.error).toContain('transport gone');
  });

  it('still completes teardown when stopNotifications rejects', async () => {
    const { transport } = transportMidDisconnect(() =>
      Promise.reject(new Error('transport gone'))
    );

    await expect(transport.disconnect()).resolves.toBeUndefined();
    expect(transport.isConnected()).toBe(false);
  });
});

/**
 * TRA-1179 — the retry budget must fit inside the command timeout it lives in.
 *
 * `retryDelays` was [500, 1500, 5000] with retryCount 3, nested inside
 * `CommandManager.DEFAULT_TIMEOUT = 2500ms` (worker/cs108/command.ts:40). Worst
 * case: 7000ms of sleeping for a command that rejected at 2500ms — and the retry
 * then *still issued the write*, delivering a stale command into the stream
 * after its owner had given up. The third delay alone is 2x the timeout.
 *
 * A write nobody is waiting for is not a retry, it is an injection.
 *
 * Also: 'GATT Server is disconnected' was treated as retryable. A disconnected
 * server does not recover by waiting, and a late write landing on a reconnected
 * link is the most harmful version of the above.
 */
describe('CS108BLETransport write retry budget', () => {
  it('does not sleep longer than the command budget across all retries', () => {
    const transport = new CS108BLETransport();
    const priv = transport as unknown as { retryDelays: number[]; WRITE_BUDGET_MS: number };

    const total = priv.retryDelays.reduce((a, b) => a + b, 0);

    expect(total).toBeLessThanOrEqual(priv.WRITE_BUDGET_MS);
  });

  it('abandons a retry once the budget has elapsed rather than writing late', async () => {
    const transport = new CS108BLETransport({ retryCount: 3, retryDelays: [1, 1, 1] });
    const posted: Array<{ type: string }> = [];
    let writes = 0;

    Object.assign(transport as unknown as Record<string, unknown>, {
      device: {},
      server: { connected: true },
      writeCharacteristic: {
        writeValue: () => {
          writes++;
          return Promise.reject(new Error('GATT operation already in progress'));
        }
      },
      messagePort: { postMessage: (m: unknown) => posted.push(m as { type: string }) }
    });

    const priv = transport as unknown as { queueWrite: (d: Uint8Array) => Promise<boolean> };

    const ok = await priv.queueWrite(new Uint8Array([0x01]));

    expect(ok).toBe(false);
    expect(writes).toBeLessThanOrEqual(4);
    expect(posted.some(m => m.type === 'ble:error')).toBe(true);
  });

  /**
   * Ack latency lives INSIDE the retry budget, and the budget cannot bound the
   * last attempt.
   *
   * ble-mcp-test 0.9.0 resolves `writeValue()` on the bridge's ack rather than on
   * enqueue, so every attempt costs real time. `withinBudget` gates the SLEEP
   * before a retry, never the write itself — so the final attempt starts inside
   * the budget and finishes wherever it finishes.
   *
   * This drives the real transport with a slow-failing characteristic rather than
   * recomputing the arithmetic, because a test that re-implements the thing it is
   * checking passes on its own mistakes.
   *
   * Documenting a hazard, not endorsing one: at ~600ms ack latency the last write
   * lands after `CommandManager.DEFAULT_TIMEOUT` (2500ms) has already rejected the
   * command that owns it. If the final attempt is ever made to respect the
   * deadline too, this goes red — update it deliberately. TRA-1189 Phase 1 is
   * measuring whether the distribution actually occupies the window.
   */
  it('lets the final attempt outlive the command timeout at ~600ms ack latency (known — TRA-1189)', async () => {
    const ACK_MS = 600;
    const COMMAND_TIMEOUT_MS = 2500; // CommandManager.DEFAULT_TIMEOUT
    const transport = new CS108BLETransport();

    let lastWriteEndedAt = 0;
    const started = Date.now();

    Object.assign(transport as unknown as Record<string, unknown>, {
      device: {},
      server: { connected: true },
      writeCharacteristic: {
        writeValue: async () => {
          await new Promise(r => setTimeout(r, ACK_MS));
          lastWriteEndedAt = Date.now() - started;
          throw new Error('GATT operation already in progress');
        }
      },
      messagePort: { postMessage: () => {} }
    });

    const priv = transport as unknown as { queueWrite: (d: Uint8Array) => Promise<boolean> };
    await priv.queueWrite(new Uint8Array([0x01]));

    expect(lastWriteEndedAt).toBeGreaterThan(COMMAND_TIMEOUT_MS);
  }, 15000);

  it('does not retry a disconnected GATT server', () => {
    const transport = new CS108BLETransport();
    const priv = transport as unknown as { isRetryable: (m: string) => boolean };

    expect(priv.isRetryable('GATT operation already in progress')).toBe(true);
    expect(priv.isRetryable('Device busy')).toBe(true);
    expect(priv.isRetryable('GATT Server is disconnected')).toBe(false);
  });
});

/**
 * TRA-1179 / TRA-1153 cross-repo contract — `gatt.disconnect()` must be awaited.
 *
 * Real Web Bluetooth returns `void` here, so nothing was awaited and nothing
 * needed to be. The ble-mcp-test mock returns a settleable value, and per
 * TRA-1153 the command-path release lands when the *server* processes the
 * socket close — not when `server.connected` flips, which happens synchronously
 * before it.
 *
 * So a fire-and-forget disconnect lets the next connect race ahead of the
 * release and be refused as busy **by its own previous session** — an error that
 * reads as an ownership bug rather than a lifecycle one. The bridge session hit
 * exactly this in its e2e helpers and had to fix it twice.
 *
 * `await Promise.resolve(...)` is correct against both: a no-op for `void`, a
 * real await for a thenable.
 */
describe('CS108BLETransport disconnect awaits the GATT close', () => {
  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).__TRANSPORT_MANAGER__;
  });

  it('does not resolve before a promise-returning gatt.disconnect settles', async () => {
    const transport = new CS108BLETransport();
    let closed = false;

    Object.assign(transport as unknown as Record<string, unknown>, {
      device: {
        removeEventListener: () => {},
        gatt: {
          connected: true,
          disconnect: () =>
            new Promise<void>(r =>
              setTimeout(() => {
                closed = true;
                r();
              }, 5)
            )
        }
      },
      messagePort: { postMessage: () => {}, close: () => {} }
    });

    await transport.disconnect();

    // If disconnect() returned before the close settled, this is still false.
    expect(closed).toBe(true);
  });
});

/**
 * TRA-1179 — a retry must leave a trace.
 *
 * The retry branch was silent: the only marker was a `// Retrying write after
 * delay` *comment*. So a retry firing and a retry never firing were
 * indistinguishable from outside, which is why nobody noticed that
 * `retryDelays` summed to 7000ms inside a 2500ms command timeout — the path had
 * been unobservable for as long as it had been wrong.
 *
 * It also means a green hardware run proves nothing about this path: if the
 * mechanism cannot announce itself, "no issues" and "never executed" look the
 * same. Pin the observability, not just the behaviour.
 */
describe('CS108BLETransport retry observability', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('logs each retry with attempt, delay and cause', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const transport = new CS108BLETransport({ retryCount: 2, retryDelays: [1, 1] });

    Object.assign(transport as unknown as Record<string, unknown>, {
      device: {},
      server: { connected: true },
      writeCharacteristic: {
        writeValue: () => Promise.reject(new Error('GATT operation already in progress'))
      },
      messagePort: { postMessage: () => {} }
    });

    await (transport as unknown as {
      queueWrite: (d: Uint8Array) => Promise<boolean>;
    }).queueWrite(new Uint8Array([0x01]));

    const retryLogs = warn.mock.calls.filter(c => String(c[0]).includes('retry'));

    expect(retryLogs.length).toBeGreaterThan(0);
    // The cause must travel with it — a retry count alone does not tell you why.
    expect(String(retryLogs[0][0])).toContain('GATT operation already in progress');
  });
});
