/**
 * BLE Transport implementation using Web Bluetooth API
 * Based on proven patterns from lib/rfid/cs108/transportManager.ts
 */

import type { Transport, BLEMessage } from './Transport';
// TYPE-ONLY, and it has to stay that way. ble-mcp-test is a devDependency and
// test tooling — the app reaches a CS108 solely through navigator.bluetooth — so
// a value import would put test tooling on the product path and break the
// production build. `import type` is erased at compile time and ships nothing.
import type { WriteErrorCode } from 'ble-mcp-test';

/**
 * The one write failure worth retrying. See `isRetryable`.
 *
 * Typed as `WriteErrorCode` on purpose: the annotation is what makes the
 * compiler fail if ble-mcp-test renames or drops this code, which is the check
 * a bare string literal could not give us. Nothing of the package survives into
 * the bundle — the type is erased, the string is ours.
 */
const RETRYABLE_WRITE_CODE: WriteErrorCode = 'WRITE_REJECTED';

// Type for test environment window extensions
interface TestWindow extends Window {
  __TRANSPORT_MANAGER__?: {
    notifyCharacteristic: BluetoothRemoteGATTCharacteristic | null;
  };
}

// CS108 BLE Service and Characteristic UUIDs
// Using full 128-bit UUIDs for clarity and Web Bluetooth compatibility
// Bridge server will normalize these as needed
export const CS108_BLE_SERVICE_UUID = '00009800-0000-1000-8000-00805f9b34fb';
export const CS108_BLE_WRITE_UUID = '00009900-0000-1000-8000-00805f9b34fb';
export const CS108_BLE_NOTIFY_UUID = '00009901-0000-1000-8000-00805f9b34fb';

// Web Bluetooth API type declarations
declare global {
  interface BluetoothDevice {
    id: string;
    name?: string;
    gatt?: BluetoothRemoteGATTServer;
    addEventListener(type: string, listener: EventListener): void;
    removeEventListener(type: string, listener: EventListener): void;
  }

  interface BluetoothRemoteGATTServer {
    connected: boolean;
    device: BluetoothDevice;
    connect(): Promise<BluetoothRemoteGATTServer>;
    disconnect(): void;
    getPrimaryService(service: string | number): Promise<BluetoothRemoteGATTService>;
  }

  interface BluetoothRemoteGATTService {
    uuid: string;
    device: BluetoothDevice;
    getCharacteristic(characteristic: string | number): Promise<BluetoothRemoteGATTCharacteristic>;
  }

  interface BluetoothRemoteGATTCharacteristic {
    uuid: string;
    service: BluetoothRemoteGATTService;
    value?: DataView;
    readValue(): Promise<DataView>;
    /**
     * Resolves when the bridge acknowledges THIS write, as of ble-mcp-test 0.9.0.
     * It previously resolved on enqueue, which made the write-failure path below
     * unreachable. See `WRITE_BUDGET_MS` — that change moved real time inside the
     * retry budget and is the reason its arithmetic was restated.
     */
    writeValue(value: BufferSource): Promise<void>;
    /**
     * Legal on the CS108, which advertises `write`. The mock rejects only if the
     * bridge actually wrote in without-response mode, since the guarantee this
     * name makes would otherwise be a lie.
     */
    writeValueWithResponse(value: BufferSource): Promise<void>;
    /**
     * Declared because real Web Bluetooth and the mock both have it — but
     * **illegal against the CS108**, whose write characteristic advertises
     * `properties=['write']` only. The mock rejects it, matching Chrome's
     * NotSupportedError. Use `writeValue` or `writeValueWithResponse`.
     *
     * These two were removed while 0.8.0 genuinely lacked them: a hand-written
     * interface asserting a shape nobody had checked, partially satisfied so it
     * read as validated. 0.9.0 ships both, so the declaration is now true — and
     * the reason to be careful moved from "the method does not exist" to "this
     * one is not permitted on this device," which the type cannot express and
     * this comment therefore must.
     */
    writeValueWithoutResponse(value: BufferSource): Promise<void>;
    startNotifications(): Promise<BluetoothRemoteGATTCharacteristic>;
    stopNotifications(): Promise<BluetoothRemoteGATTCharacteristic>;
    addEventListener(type: 'characteristicvaluechanged', listener: (event: Event) => void): void;
    removeEventListener(type: 'characteristicvaluechanged', listener: (event: Event) => void): void;
  }

  interface BluetoothLEScanFilter {
    services?: Array<string | number>;
    name?: string;
    namePrefix?: string;
  }
}

// Minimal config - CS108 UUIDs are hardcoded
export interface CS108BLETransportConfig {
  retryCount?: number;
  retryDelays?: number[];
}

export class CS108BLETransport implements Transport {
  // CS108 UUIDs - hardcoded, not configurable
  private readonly serviceUUID = CS108_BLE_SERVICE_UUID;
  private readonly writeUUID = CS108_BLE_WRITE_UUID;
  private readonly notifyUUID = CS108_BLE_NOTIFY_UUID;
  
  // Configurable options
  private readonly retryCount: number;
  private readonly retryDelays: number[];
  private device: BluetoothDevice | null = null;
  private server: BluetoothRemoteGATTServer | null = null;
  private service: BluetoothRemoteGATTService | null = null;
  private writeCharacteristic: BluetoothRemoteGATTCharacteristic | null = null;
  private notifyCharacteristic: BluetoothRemoteGATTCharacteristic | null = null;
  private messagePort: MessagePort | null = null;
  
  // Command queue for reliable writes
  private commandQueue: Array<{
    data: Uint8Array;
    resolve: (success: boolean) => void;
    retriesLeft: number;
    /** Wall-clock point past which this write must be abandoned, not retried. */
    deadline: number;
  }> = [];
  private commandInProgress = false;
  private readonly MAX_QUEUE_LENGTH = 5;
  
  // Bound event handlers
  private boundHandleNotifications: (event: Event) => void;
  private boundHandleDisconnect: (event: Event) => void;
  
  /**
   * How long a single write may spend retrying, in total.
   *
   * Deliberately under `CommandManager.DEFAULT_TIMEOUT` (2500 ms,
   * `worker/cs108/command.ts`). The retries live *inside* a command's lifetime,
   * so a retry budget larger than the command timeout means sleeping on behalf
   * of a command that has already rejected — and then issuing the write anyway.
   * A write nobody is waiting for is not a retry, it is an injection into the
   * command stream. The old defaults summed to 7000 ms against that 2500 ms
   * timeout (TRA-1179).
   */
  private readonly WRITE_BUDGET_MS = 2000;

  /*
   * Ack latency is inside this budget, and the arithmetic below is not what it
   * looks like.
   *
   * `retryDelays` sums to 1750 ms, which reads as "1750 < 2000, comfortable."
   * That was true when the mock resolved `writeValue()` on enqueue. Since
   * ble-mcp-test 0.9.0 it resolves on the bridge's ack, so every attempt spends
   * real time too, and the sum of the sleeps is no longer the sum of the pass.
   *
   * `withinBudget` gates the SLEEP, not the write. So the budget self-corrects
   * on retry COUNT — as ack latency `L` rises, fewer retries fit — but it cannot
   * bound the final attempt, which starts inside the budget and finishes
   * wherever it finishes. With `L` folded in, the last write ends at:
   *
   *     3 retries fire (L≈0):    1750 ms
   *     2 retries fire:          3L +  750 ms
   *     1 retry   fires:         2L +  250 ms
   *
   * Set against `CommandManager.DEFAULT_TIMEOUT` (2500 ms), that overruns in
   * three DISJOINT windows, with safe bands between them:
   *
   *      584 ms <= L <  625 ms     2 retries, ends up to 2624 ms
   *     1126 ms <= L <= 1500 ms    1 retry,   ends up to 3250 ms
   *
   * The third window the raw arithmetic predicts (L >= 2501) is UNREACHABLE, and
   * the second is truncated from 1750 to 1500, because `L` is capped: the mock's
   * ack timeout rejects at 1500 ms, and that rejection is **not retryable**, so
   * past 1500 ms the loop stops on the first attempt.
   *
   * UNTIL 0.10.0 THAT HELD BY ACCIDENT. Window 3 was closed by a STRING MISMATCH
   * between this file's retry list and ble-mcp-test's timeout text — reword the
   * timeout to contain "busy" and the ack timeout became retryable, two capped
   * attempts plus a delay reached 3250 ms, and the window reopened with no other
   * signal. Nothing on either side would have caught it.
   *
   * It is now closed on purpose. The ack timeout arrives as a typed
   * `ACK_TIMEOUT` carrying `mayHaveReachedDevice: true`, and `isRetryable`
   * retries only an affirmative `WRITE_REJECTED`, so no wording of any message
   * can reopen the window. The messages are free to change; that freedom is the
   * point of the codes.
   *
   * Inside a window the last write lands after the command that owns it has
   * already rejected — precisely the "stale command injected into the stream"
   * that TRA-1179 removed, returning through ack latency instead of through
   * oversized delays.
   *
   * Two consequences worth stating, because both invite a wrong instinct:
   *
   * - **Worse is not monotonically worse.** L=700 ms is safe; L=600 ms is not.
   *   Watching a single threshold ("is p99 near 600?") cannot see the
   *   1126–1750 ms window at all.
   * - **A percentile cannot answer this.** The windows are narrow, so what
   *   matters is how much of the DISTRIBUTION sits inside one — and a p99 that
   *   summarises a bimodal sample hides exactly the second mode that would.
   *   Record the distribution (TRA-1189 Phase 1), not the percentile.
   *
   * Left at 2000 deliberately. Widening it would let more stale writes through,
   * not fewer; the fix if a window is ever occupied is to make the final attempt
   * respect the deadline too, and that wants soak evidence first.
   */

  constructor(config: CS108BLETransportConfig = {}) {
    this.retryCount = config.retryCount || 3;
    // Sums to 1750 ms of SLEEP. Ack latency is extra — see WRITE_BUDGET_MS.
    this.retryDelays = config.retryDelays || [250, 500, 1000];
    
    // Bind event handlers
    this.boundHandleNotifications = this.handleNotifications.bind(this);
    this.boundHandleDisconnect = this.handleDisconnect.bind(this);
  }
  
  /**
   * Connect to BLE device and set up MessagePort communication
   */
  async connect(): Promise<MessagePort> {
    if (!this.isSupported()) {
      throw new Error('Web Bluetooth is not supported in this browser');
    }
    
    const connectStartedAt = Date.now();

    try {
      // Request device selection
      // Selection is by service UUID only. Web Bluetooth ORs the filters array,
      // so a second { name } filter would WIDEN the match rather than narrow it.
      const filters: BluetoothLEScanFilter[] = [
        { services: [this.serviceUUID] }
      ];

      this.device = await navigator.bluetooth.requestDevice({
        filters,
        optionalServices: [this.serviceUUID]
      });
      
      if (!this.device) {
        throw new Error('No device selected');
      }
      
      // Set up disconnect listener (if supported by the device implementation)
      if (typeof this.device.addEventListener === 'function') {
        this.device.addEventListener('gattserverdisconnected', this.boundHandleDisconnect);
      }
      
      // Connect to GATT server
      this.server = await this.device.gatt!.connect();
      
      // Get service
      this.service = await this.server.getPrimaryService(this.serviceUUID);
      
      // Get characteristics
      this.writeCharacteristic = await this.service.getCharacteristic(this.writeUUID);
      this.notifyCharacteristic = await this.service.getCharacteristic(this.notifyUUID);
      
      // Subscribe to notifications
      await this.notifyCharacteristic.startNotifications();
      this.notifyCharacteristic.addEventListener('characteristicvaluechanged', this.boundHandleNotifications);

      // Expose for E2E testing - allows simulateTriggerPress to inject notifications
      if (typeof window !== 'undefined' && (import.meta.env.MODE === 'test' || import.meta.env.MODE === 'development')) {
        const testWindow = window as TestWindow;
        testWindow.__TRANSPORT_MANAGER__ = {
          notifyCharacteristic: this.notifyCharacteristic
        };
        // Exposed __TRANSPORT_MANAGER__ for testing
      } else {
        // Not exposing __TRANSPORT_MANAGER__ (not in test mode)
      }

      // Create MessageChannel for worker communication
      const channel = new MessageChannel();
      this.messagePort = channel.port1;
      
      // Set up message handling from worker
      this.messagePort.onmessage = (event) => {
        // Received message from worker
        const message = event.data as BLEMessage;
        if (message.type === 'ble:write' && message.data) {
          // Queueing write command
          this.queueWrite(message.data);
        } else {
          // Ignoring non-write message
        }
      };
      
      // Notify worker of connection
      this.messagePort.postMessage({ 
        type: 'ble:connected' 
      } as BLEMessage);
      
      console.info(`Connected to ${this.device.name || 'BLE Device'}`);
      this.recordConnect(connectStartedAt, null);
      
      // Return port2 for worker
      return channel.port2;
      
    } catch (error) {
      this.recordConnect(connectStartedAt, error);
      // Clean up on error
      await this.cleanup();
      throw error;
    }
  }

  /**
   * Connect-duration instrument. One line per connect attempt, either outcome.
   *
   * Exists to keep a known slow path from being read as the radio. In
   * ble-mcp-test's `connect()`, `onclose` rejects promptly ONLY on WebSocket
   * close codes 4000-4999, and the Python bridge never sends one — so an
   * ordinary close before `connected` arrives (bridge down, refused, killed
   * mid-handshake) falls through to the 10s connect timeout instead of failing
   * immediately. Every real connect-time close therefore costs the full 10s.
   *
   * A soak of this suite is substantially a connect/disconnect cycle test, so
   * that path is on the measured route. Failures clustered near 10s are THIS,
   * not the reader — `scripts/ack-latency-report.mjs` reports the cluster so the
   * attribution is made from the data rather than from memory.
   *
   * Known and deliberately unfixed upstream: rewriting connect rejection
   * semantics immediately before a connect-heavy baseline would destroy the
   * before/after it is meant to establish.
   */
  private recordConnect(startedAt: number, error: unknown): void {
    const finishedAt = Date.now();
    const outcome = error === null
      ? 'ok'
      : ((error as { code?: string })?.code ?? 'UNTYPED');

    console.info(
      `[ble-timing] connect t=${finishedAt} ms=${finishedAt - startedAt} outcome=${outcome}`
    );
  }
  
  /**
   * Disconnect from BLE device
   */
  async disconnect(): Promise<void> {
    // Clear command queue
    this.clearCommandQueue('Device disconnecting');
    
    // Stop notifications
    if (this.notifyCharacteristic) {
      try {
        await this.notifyCharacteristic.stopNotifications();
      } catch (e) {
        // Report, do not rethrow: teardown must still complete. Visibility is
        // the requirement, not propagation — the same trade #583 made for
        // writes. This catch was dead while stopNotifications() was a no-op;
        // TRA-1153 makes it a real gate that can reject (TRA-1179).
        this.reportTransportError(
          `stopNotifications failed: ${e instanceof Error ? e.message : String(e)}`
        );
      }

      // Unhook regardless — a listener left on a characteristic we are dropping
      // is exactly the orphan cleanup() exists to prevent.
      this.notifyCharacteristic.removeEventListener(
        'characteristicvaluechanged',
        this.boundHandleNotifications
      );
    }
    
    // Remove disconnect listener
    if (this.device && typeof this.device.removeEventListener === 'function') {
      this.device.removeEventListener(
        'gattserverdisconnected',
        this.boundHandleDisconnect
      );
    }
    
    // Disconnect GATT, and wait for it.
    //
    // Real Web Bluetooth returns void here, so this is a no-op await. The
    // ble-mcp-test mock returns a settleable value, and per TRA-1153 the
    // command-path release lands when the *server* processes the socket close —
    // not when `server.connected` flips, which happens synchronously before it.
    // Fire-and-forget lets the next connect race ahead of the release and be
    // refused as busy by our own previous session, which reads as an ownership
    // bug rather than a lifecycle one (TRA-1179).
    if (this.device?.gatt?.connected) {
      await Promise.resolve(this.device.gatt.disconnect());
    }
    
    // Notify worker and close port
    if (this.messagePort) {
      this.messagePort.postMessage({ 
        type: 'ble:disconnected' 
      } as BLEMessage);
      this.messagePort.close();
    }
    
    // Clean up — cleanup() owns the test exposure too, so an unexpected GATT
    // drop clears exactly as much as an explicit disconnect (TRA-1179).
    await this.cleanup();
  }
  
  /**
   * Check if connected
   */
  isConnected(): boolean {
    // server.connected matters: between a GATT drop and handleDisconnect running,
    // the objects are all still here while the link is gone.
    return !!(this.device && this.server?.connected && this.writeCharacteristic);
  }
  
  /**
   * Get transport type
   */
  getType(): string {
    return 'ble';
  }

  /**
   * Real Web Bluetooth is local; the ble-mcp-test mock is not.
   *
   * The mock replaces navigator.bluetooth in place and flags itself with
   * __webBluetoothBridged, so getType() still reports 'ble' while every
   * notification is in fact arriving over a WebSocket from the bridge host.
   * That flag is the only thing that distinguishes the two here.
   */
  isNetworked(): boolean {
    return typeof window !== 'undefined' && !!window.__webBluetoothBridged;
  }
  
  /**
   * Check if Web Bluetooth is supported
   */
  private isSupported(): boolean {
    return 'bluetooth' in navigator;
  }
  
  /**
   * Handle BLE notifications
   */
  private handleNotifications(event: Event): void {
    const characteristic = event.target as unknown as BluetoothRemoteGATTCharacteristic;
    const value = characteristic.value;
    
    if (!value) return;

    // Clone data immediately to avoid DataView detachment.
    //
    // Honour byteOffset/byteLength: a DataView is a *view* onto its backing
    // ArrayBuffer, and that buffer may be larger than the notification (a pooled
    // allocator, a Node Buffer, or a binary/base64 WebSocket frame decoded into a
    // shared buffer). Copying `value.buffer` wholesale would hand the worker the
    // whole pool starting at offset 0 — garbage packets rather than a clean error.
    // Exact-size buffers at offset 0 (today's JSON text frames) are unaffected.
    const data = new Uint8Array(value.buffer, value.byteOffset, value.byteLength).slice();

    // Send to worker via MessagePort
    if (this.messagePort) {
      this.messagePort.postMessage({
        type: 'ble:data',
        data
      } as BLEMessage);
    }
  }
  
  /**
   * Handle disconnection
   */
  private handleDisconnect(): void {
    // BLE device disconnected
    //
    // `inflight` is the load-bearing field: a link close while a write is
    // outstanding is the exact condition ble-mcp-test's `failPendingWrites`
    // exists to make impossible. Logged before the queue is cleared, because
    // clearing it is what destroys the evidence.
    console.info(
      `[ble-timing] link-close t=${Date.now()} inflight=${this.commandInProgress ? 1 : 0} ` +
      `queued=${this.commandQueue.length}`
    );

    // Clear command queue
    this.clearCommandQueue('Device disconnected');
    
    // Notify worker
    if (this.messagePort) {
      this.messagePort.postMessage({ 
        type: 'ble:disconnected' 
      } as BLEMessage);
    }
    
    // Clean up
    this.cleanup();
  }
  
  /**
   * Queue a write operation with retry logic
   */
  private async queueWrite(data: Uint8Array): Promise<boolean> {
    return new Promise<boolean>((resolve) => {
      if (this.commandQueue.length >= this.MAX_QUEUE_LENGTH) {
        this.reportTransportError('Command queue full, write dropped');
        resolve(false);
        return;
      }
      
      this.commandQueue.push({
        data,
        resolve,
        retriesLeft: this.retryCount,
        deadline: Date.now() + this.WRITE_BUDGET_MS
      });
      
      this.processNextCommand();
    });
  }
  
  /**
   * Process next command in queue
   */
  private async processNextCommand(): Promise<void> {
    if (this.commandInProgress || this.commandQueue.length === 0) {
      return;
    }
    
    const command = this.commandQueue.shift()!;
    this.commandInProgress = true;
    
    try {
      if (!this.isConnected()) {
        this.reportTransportError('Transport not connected, write dropped');
        command.resolve(false);
        return;
      }

      // Writing to BLE characteristic
      // Create a new Uint8Array to ensure proper ArrayBuffer type
      const startedAt = Date.now();
      try {
        await this.writeCharacteristic!.writeValue(new Uint8Array(command.data));
      } catch (error) {
        this.recordWriteAck(command, startedAt, error);
        throw error;
      }
      this.recordWriteAck(command, startedAt, null);
      // BLE write completed successfully
      command.resolve(true);
      
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      
      const command_ = command;
      const delayIndex = this.retryCount - command_.retriesLeft;
      const delay = this.retryDelays[Math.min(delayIndex, this.retryDelays.length - 1)];

      // A retry is only worth attempting if the command that owns it will still
      // be listening when the write lands. Past the deadline it is not a retry,
      // it is a stale command injected into the stream (TRA-1179).
      const withinBudget = Date.now() + delay < command_.deadline;

      if (this.isRetryable(error, errorMessage) && command_.retriesLeft > 0 && withinBudget) {
        command_.retriesLeft--;
        const attempt = this.retryCount - command_.retriesLeft;

        // Leave a trace. This branch used to be silent — the only marker was a
        // comment — so a retry firing and a retry never firing looked identical
        // from outside. That is how `retryDelays` came to sum to 7000ms inside a
        // 2500ms command timeout without anyone noticing: the path had been
        // unobservable for as long as it had been wrong (TRA-1179).
        console.warn(
          `[CS108BLETransport] write retry ${attempt}/${this.retryCount} in ${delay}ms: ${errorMessage}`
        );

        await new Promise(r => setTimeout(r, delay));

        // Put command back at front of queue
        this.commandQueue.unshift(command_);
      } else {
        this.reportTransportError(errorMessage);
        command.resolve(false);
      }
    } finally {
      this.commandInProgress = false;
      this.processNextCommand();
    }
  }
  
  /**
   * Tell the worker a write did not reach the device.
   *
   * Without this the worker cannot distinguish "command sent, awaiting ACK" from
   * "command never left", so every dropped write surfaced only as the command's
   * own 5s timeout. Two of the three failure paths previously reported nothing.
   */
  /**
   * Ack-latency instrument. One line per write ATTEMPT, success or failure.
   *
   * `L` — the time `writeValue()` takes — became a real quantity in ble-mcp-test
   * 0.9.0, which moved the resolve from enqueue to the bridge's ack. It sits
   * INSIDE `WRITE_BUDGET_MS` and the budget structurally cannot bound it (see
   * that field's comment), so the overrun windows are narrow, disjoint and
   * non-monotonic: 700ms is safe while 600ms is not.
   *
   * Consequently this records the DISTRIBUTION and nothing else. It deliberately
   * computes no percentile: a p99 over a bimodal sample is arithmetically correct
   * and answers a question nobody asked, hiding the second mode that is the whole
   * reason to measure. Summarise with `scripts/ack-latency-report.mjs`.
   *
   * `outcome` reads `error.code` when the package supplies one and reports
   * `UNTYPED` otherwise. It is NOT a list of the codes ble-mcp-test defines —
   * enumerating another repo's values is the defect this instrument exists to
   * watch for. A code added upstream appears here without a change.
   *
   * Baseline for TRA-1189 Phase 1.
   */
  private recordWriteAck(
    command: { retriesLeft: number },
    startedAt: number,
    error: unknown
  ): void {
    const finishedAt = Date.now();
    const attempt = this.retryCount - command.retriesLeft + 1;
    const outcome = error === null
      ? 'ok'
      : ((error as { code?: string })?.code ?? 'UNTYPED');

    // `t` is the END of the attempt and `ms` its duration, so a close event's
    // timestamp can be tested against the half-open window [t - ms, t] — the
    // join that decides whether an ack timeout and a link drop hit the same
    // write. That co-occurrence is the signature of an unwired failPendingWrites
    // and is a stop-the-soak finding on a single occurrence, not a rate.
    console.info(
      `[ble-timing] write-ack t=${finishedAt} ms=${finishedAt - startedAt} ` +
      `attempt=${attempt}/${this.retryCount} outcome=${outcome}`
    );
  }

  /**
   * Which write failures are worth another attempt.
   *
   * TWO conditions, and the second is the one that is easy to drop.
   *
   * 1. The write must not have reached the device — otherwise a retry
   *    duplicates it. ble-mcp-test 0.10.0 states this as a fact on the error
   *    (`mayHaveReachedDevice`) instead of leaving us to infer it from prose.
   *
   * 2. The link must still be up. That is NOT implied by the first: `LINK_LOST`
   *    and `NOT_CONNECTED` are both non-duplicative, and both must still refuse
   *    to retry. A disconnected server does not recover by waiting, so the retry
   *    spends the budget for nothing — and if the link *does* come back, it lands
   *    a stale command on a fresh connection, the most harmful outcome available
   *    (TRA-1179).
   *
   * `mayHaveReachedDevice` alone is NECESSARY, NOT SUFFICIENT. The package can
   * see whether its own write left; it cannot see whether this transport still
   * has a link, so sufficiency has to be decided here.
   *
   * AND `isConnected()` CANNOT SUPPLY IT ON ITS OWN. It reads `server.connected`,
   * which by its own comment lags a GATT drop — between the drop and
   * `handleDisconnect` running, the objects are all still present while the link
   * is gone. That is exactly the interval in which `LINK_LOST` is thrown, so
   * `!mayHaveReachedDevice && isConnected()` would retry a link loss whenever the
   * upstream close handler happens to settle the write before the mock flips its
   * server flag. A test asserting the LINK_LOST limb is what surfaced this; the
   * ordering it depends on lives in another repo and is nobody's stated contract.
   *
   * So the predicate demands AFFIRMATIVE evidence rather than the absence of a
   * counter-signal: retry only what the peer says it refused outright. That is
   * `WRITE_REJECTED`, which ble-mcp-test's own contract names as the only code it
   * can promise is worth retrying.
   *
   * This does reference one upstream value, and the direction of its staleness is
   * the whole justification. A code added upstream is not retried until this is
   * updated — a missed retry, which costs a command one attempt. The inverse
   * spelling (`!mayHaveReachedDevice`, minus a list of known-bad codes) fails the
   * other way: a new code is retried by default, and the failure mode is the
   * TRA-1179 stale-write harm. Prefer the arm that goes stale toward doing less.
   * The code is pinned by the package's own `WriteErrorCode` type, so a rename
   * upstream is a compile error here rather than a silent behaviour change.
   *
   * The Chrome fallback is production behaviour, not mock coupling. Real Web
   * Bluetooth throws a `DOMException` carrying no such property, and 'GATT
   * operation already in progress' is genuine Chrome text — the app reaches a
   * CS108 through `navigator.bluetooth`, so that is the path that matters most.
   *
   * `'Device busy'` was the other arm and never once fired: the bridge says
   * `Device is busy`, and `'Device is busy'.includes('Device busy')` is false.
   * It came from the original baseline copy, not from anything the peer sends.
   * Deleted rather than corrected — that refusal is deliberately non-retryable
   * upstream, so matching it would have turned a dead branch into a live wrong
   * one. Note the shape: a predicate taking `errorMessage: string` could only
   * ever be written as a string match, so the signature was the defect and the
   * dead arm was its symptom. It takes the error now.
   *
   * Discrimination is by `name`, not `instanceof`. The mock reaches this code by
   * two routes — an ESM import and an injected browser bundle — and class
   * identity is scoped to the module instance that defined it, so `instanceof`
   * fails across the injected copy while the object is perfectly well-formed.
   */
  private isRetryable(error: unknown, errorMessage: string): boolean {
    // Condition 2, checked first because it overrides everything below it.
    if (!this.isConnected()) return false;

    const typed = error as
      { name?: string; code?: string; mayHaveReachedDevice?: boolean } | null;
    if (typed?.name === 'WriteError') {
      // Both limbs: the code says the peer refused it, the property confirms it
      // cannot have been delivered. If upstream ever changed one without the
      // other, this stops retrying rather than guessing which to believe.
      return typed.code === RETRYABLE_WRITE_CODE
        && typed.mayHaveReachedDevice === false;
    }

    return errorMessage.includes('GATT operation already in progress');
  }

  private reportTransportError(error: string): void {
    // Not "write failed" any more — teardown reports through here too.
    console.error('[CS108BLETransport]', error);
    if (this.messagePort) {
      this.messagePort.postMessage({ type: 'ble:error', error } as BLEMessage);
    }
  }
  
  /**
   * Clear command queue
   */
  private clearCommandQueue(_reason: string): void {
    if (this.commandQueue.length > 0) {
      // Clearing queued commands
      this.commandQueue.forEach(cmd => cmd.resolve(false));
      this.commandQueue = [];
    }
    this.commandInProgress = false;
  }
  
  /**
   * Clean up resources
   */
  /**
   * Release everything this transport owns.
   *
   * This is the single teardown owner: both the explicit `disconnect()` and the
   * `gattserverdisconnected` handler route here, so they cannot clear different
   * amounts. They used to — `disconnect()` deleted `__TRANSPORT_MANAGER__` and
   * `handleDisconnect()` did not, which left the e2e trigger helpers injecting
   * into an orphaned characteristic after any unexpected drop. That surfaced as
   * `NOTIFY_CHAR_NOT_FOUND` on real hardware (TRA-1179).
   */
  private async cleanup(): Promise<void> {
    this.device = null;
    this.server = null;
    this.service = null;
    this.writeCharacteristic = null;
    this.notifyCharacteristic = null;

    // The test hook points at a characteristic that no longer receives anything.
    if (typeof window !== 'undefined') {
      const testWindow = window as TestWindow;
      if (testWindow.__TRANSPORT_MANAGER__) {
        delete testWindow.__TRANSPORT_MANAGER__;
      }
    }

    if (this.messagePort) {
      this.messagePort.close();
      this.messagePort = null;
    }
  }
}