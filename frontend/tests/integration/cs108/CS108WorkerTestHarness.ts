/**
 * CS108 Worker Test Harness — the production transport, driven from vitest.
 *
 * INTEGRATION TEST RULES:
 *
 * ✅ ALLOWED - Public API Only:
 * - connect(), disconnect()
 * - setMode(), setSettings()
 * - startScanning(), stopScanning()
 * - waitForEvent(), getEvents()
 * - getReaderState(), getReaderMode() [read-only observation]
 *
 * ❌ FORBIDDEN - No Internal Access:
 * - Direct worker property access
 * - Private method calls
 * - State manipulation
 *
 * If a test needs internal access to pass, the production code is broken.
 *
 * ## What this harness stopped doing, and why (TRA-1187 item 3)
 *
 * It used to build a pair of hand-written `MockMessagePort`s and pump bytes
 * between `CS108Reader` and a `RfidReaderTestClient` wrapping ble-mcp-test's
 * flat `NodeBleClient` API. The consequence was that the two suites reached the
 * bridge by different routes:
 *
 *   e2e (Playwright): worker -> CS108BLETransport -> navigator.bluetooth -> bridge
 *   integration (old): worker -> harness -> flat NodeBleClient API -> bridge
 *
 * So integration never exercised `CS108BLETransport` at all — the production
 * transport layer had no automated coverage on any suite — while
 * INTEGRATION-TEST-PRINCIPLES.md opens with "Integration tests MUST interact
 * with the worker exactly like production code does." That principle held above
 * the seam and broke at it, and nothing could detect the break.
 *
 * Both suites now share one route. The harness installs the mock onto jsdom's
 * navigator and then does what `src/lib/device/device-manager.ts` does: build a
 * `CS108BLETransport`, `connect()` it, size the link profile from
 * `isNetworked()`, and hand the resulting `MessagePort` to the worker. The port
 * is a real `MessageChannel` port created by the transport, not a stand-in.
 *
 * ## What went away with it
 *
 * The traffic-capture surface (`dumpTraffic`, `wasCommandSent`,
 * `getOutboundCommands`, `getTransportMessages`, `traffic`, `onWorkerMessage`,
 * `simulateBarcodeRead`, `forwardBleData`) existed to instrument the fake ports
 * and had **no callers in any spec** — verified before deletion. Re-add any of
 * it as a tap on the real port if a test ever needs it; do not resurrect the
 * fake ports to get it back.
 */

import { CS108Reader } from '@/worker/cs108/reader';
import { CS108BLETransport } from '@/lib/device/transport/cs108-ble-transport';
import { ReaderMode } from '@/worker/types/reader';
import { WorkerEventType } from '@/worker/types/events';

import { installWebBluetoothMock } from '../ble-mcp-test/install-web-bluetooth-mock';
import type { ReaderSettings, ReaderModeType, ReaderStateType } from '@/worker/types/reader';

// Domain event types from spec
export type DataReadEvent =
  | { type: 'INVENTORY'; data: InventoryRead[] }
  | { type: 'LOCATE'; data: LocateRead[] }
  | { type: 'BARCODE'; data: BarcodeRead[] };

export type SystemEvent =
  | { type: 'READER_STATE_CHANGED'; payload: { readerState: ReaderStateType } }
  | { type: 'READER_MODE_CHANGED'; payload: { mode: ReaderModeType } }
  | { type: 'BATTERY_UPDATE'; payload: { percentage: number } }
  | { type: 'TRIGGER_STATE_CHANGED'; payload: { pressed: boolean } }
  | { type: 'SETTINGS_UPDATED'; payload: { settings: Partial<ReaderSettings> } }
  | { type: 'CONFIGURATION_COMPLETE'; payload: { mode: ReaderModeType; duration: number } }
  | { type: 'CONFIGURATION_FAILED'; payload: { mode: ReaderModeType; error: string } };

export type NotificationEvent = DataReadEvent | SystemEvent;

interface InventoryRead {
  epc: string;
  rssi: number;
  timestamp: number;
}

interface LocateRead {
  nbRssi: number;
  wbRssi?: number;
  phase?: number;
  timestamp: number;
}

interface BarcodeRead {
  symbology: string;
  data: string;
  timestamp: number;
}

/**
 * The slice of ble-mcp-test's testing API this harness uses.
 *
 * Declared locally and deliberately narrow. `@types/web-bluetooth` has no
 * `testing` member — it describes real Web Bluetooth, which is the point of the
 * whole exercise — so reaching it needs a cast either way. Naming exactly the
 * one method used keeps the cast from quietly widening into "trust me about
 * this object", which is the failure mode TRA-1187 was written about.
 */
interface MockTestingApi {
  simulateNotification(options: {
    characteristic: unknown;
    data: Uint8Array;
    delay?: number;
  }): Promise<void>;
}

/*
 * REMOVED 2026-08-29: `CONNECTION_COOLDOWN_MS = 250` and its `lastDisconnectAt`
 * companion. The measurement that justified them is kept here because it is the
 * evidence for the removal, not against it (TRA-1193).
 *
 * MEASURED 2026-08-28 by the bridge session, probing reconnect directly --
 * disconnect, then reattempt with no client pacing:
 *
 *   0ms    ->  25% connect   (15/20 refused "Device is busy")
 *   100ms  ->  95%           (1/20 refused)
 *   250ms  -> 100%           (0/20)
 *   500ms  -> 100%
 *   1000ms -> 100%
 *
 * That curve is real, and it is a curve about a caller that did not await its
 * own teardown. `installWebBluetoothMock`'s uninstall discarded the mock's
 * `teardown()` promise, so `cleanup()` resolved with socket closes in flight and
 * the next connect raced them -- which is exactly what a 0ms gap reproduces.
 * This constant was the compensation for that, and it worked.
 *
 * The uninstall now awaits teardown, and teardown awaits `gatt.disconnect()`
 * for every device it minted, which closes the socket and then waits the mock's
 * own `postDisconnectDelay`. That value is 250ms, measured independently over
 * 997 real cycles (socket close -> device released: median 16ms, p99 21ms, max
 * 30ms; 250 keeps ~8x margin). Two independent measurements landing on the same
 * number is corroboration, not coincidence.
 *
 * So the wait still happens -- it happens inside the path we now await, once,
 * owned by the side that measured it. Keeping a second copy here would pay it
 * twice and reintroduce the drift the mock's own history warns about: its
 * browser bundle once carried 1100 by esbuild `define` while the source read
 * 250, "one contract, two behaviours, selected by packaging".
 *
 * It also could not have fired where it was needed. `lastDisconnectAt` was
 * module-level, and vitest's per-file isolation reset it to 0 for every spec
 * file -- so the guard's `lastDisconnectAt > 0` test skipped it on the first
 * `initialize()` of each file, which is precisely the file boundary its own
 * doc comment said it existed for.
 *
 * Do not reintroduce a cooldown here without first showing that awaiting the
 * teardown is insufficient. See TRA-1193 and TRA-1189.
 *
 * ## That precondition WAS met once, and the answer still is not a client-side wait
 *
 * REMOVED 2026-08-31: `connectPastABusyRelease()`, an interim retry of a
 * `DEVICE_BUSY` connect on `[250, 500, 1000]`ms. It was added because awaiting
 * the teardown demonstrably did not cover one path — on the 2026-08-31 200-rep
 * arm, 63 connects were refused by our own session id and the holder released
 * 12-21ms LATER every time (median 16ms), which is the bridge's close-processing
 * cost and not ours to shrink.
 *
 * So the evidence was real and the retry was correct for its window. What made
 * it interim is that it overrode the mock's stated policy from the outside:
 * ble-mcp-test excluded `DEVICE_BUSY` from its retryable set on the grounds that
 * "another connection owns the command path, and no amount of waiting changes
 * that" — true of a FOREIGN holder, false of our own claim still closing.
 *
 * ble-mcp-test 0.16.0 splits those: `DEVICE_BUSY_SELF` is retried inside the
 * mock's own connect loop, `DEVICE_BUSY` stays loud and unretried. The wait is
 * now paid once, by the side that measured it — the same argument that removed
 * `CONNECTION_COOLDOWN_MS` above.
 *
 * ⚠ Keeping the retry after 0.16.0 would not have been merely redundant, it
 * would have been WRONG: a self-collision no longer arrives as `DEVICE_BUSY`, so
 * the only thing still reaching that matcher is a genuinely foreign holder —
 * exactly the case that must fail fast. An obsolete retry inverts its own
 * purpose rather than idling.
 *
 * `frontend/tests/config/installed-mock-retryable-connect-codes.test.ts` is the
 * guard: it asserts `DEVICE_BUSY_SELF` is in the installed package's retryable
 * set, so a downgrade or a silently-failed bump goes red here instead of
 * quietly restoring the gap this removal depends on. See TRA-1216.
 */

export class CS108WorkerTestHarness {
  private worker!: CS108Reader;
  private transport: CS108BLETransport | null = null;
  private uninstallMock: (() => Promise<void>) | null = null;
  private domainEvents: NotificationEvent[] = [];
  private eventWaiters: Map<string, (event: NotificationEvent) => void> = new Map();

  /**
   * Install the mock, connect the production transport, hand the worker its port.
   *
   * Mirrors `device-manager.ts` — including setting the link profile from
   * `transport.isNetworked()` before the port is attached, so fragment
   * reassembly is sized for the bridge rather than for a local radio. The old
   * harness never did this: it left the worker on the `'networked'` default by
   * accident rather than by measurement.
   */
  async initialize(): Promise<void> {
    // No cooldown here any more: `cleanup()` awaits the mock's teardown, which
    // does not resolve until the socket has closed and the mock's own measured
    // post-disconnect wait has elapsed. See the note above the class.
    this.uninstallMock = installWebBluetoothMock();

    // Before the worker exists: it can emit during construction.
    this.setupEventCapture();
    this.worker = new CS108Reader();

    this.transport = new CS108BLETransport();

    // A plain connect. The interim `connectPastABusyRelease()` that used to
    // wrap this is gone as of ble-mcp-test 0.16.0, which retries the one busy
    // case that waiting actually fixes — see the note above the class.
    const port = await this.transport.connect();

    const linkProfile = this.transport.isNetworked() ? 'networked' : 'native';
    this.worker.setLinkProfile(linkProfile);
    this.worker.setTransportPort(port);

    // Deliberate, and the only logging left in this harness. `[Harness]` is the
    // capture canary in scripts/suite-run-signals.mjs: a repetition whose
    // console output was never captured reads as zero of every other signal,
    // which is indistinguishable from a clean run. Rewriting this harness for
    // TRA-1187 removed the hundreds of `[Harness]` lines the canary was counting,
    // so one line per connect keeps that detector honest. It also records the
    // resolved link profile, which nothing else states.
    console.log(`[Harness] transport connected, link profile ${linkProfile}`);
  }

  /**
   * Capture domain events the worker posts.
   *
   * The worker calls `globalThis.postMessage`, which in jsdom is a real window
   * method that throws on the shapes the worker sends — so this replaces it
   * rather than wrapping it.
   */
  private setupEventCapture(): void {
    globalThis.postMessage = (message: NotificationEvent) => {
      this.domainEvents.push(message);

      const waiter = this.eventWaiters.get(message.type);
      if (waiter) {
        // The waiter decides whether this one matches; it removes itself.
        waiter(message);
      }
    };
  }

  /**
   * Inject a notification the way the bridge delivers one.
   *
   * Goes through `navigator.bluetooth.testing.simulateNotification` onto the
   * characteristic the transport actually subscribed to — the same mechanism
   * `tests/e2e/helpers/trigger-utils.ts` uses. The old harness pushed bytes
   * straight into a fake port's `onmessage`, which teleported past the mock's
   * subscription gate, the transport's listener, and its DataView handling. A
   * packet that arrived that way proved nothing about whether a real one would.
   *
   * The characteristic is taken by reference from `__TRANSPORT_MANAGER__`
   * rather than re-resolved through `getCharacteristic()` — this helper wants
   * THE instance the transport subscribed, not an equivalent one.
   *
   * An earlier version of this comment said the mock "mints a fresh
   * characteristic object per call". That was true before 0.8.0 and false when
   * it was written: TRA-1153 item 1 made the mock cache per canonical UUID. It
   * was copied here from tests/e2e/helpers/trigger-utils.ts without checking,
   * which is the failure this codebase keeps re-learning -- a claim inherited
   * rather than verified.
   */
  private async injectNotification(packet: Uint8Array): Promise<void> {
    const testing = (navigator.bluetooth as unknown as { testing?: MockTestingApi })?.testing;
    if (!testing?.simulateNotification) {
      throw new Error(
        'navigator.bluetooth.testing.simulateNotification is missing — the ble-mcp-test ' +
          'mock is not installed. Did initialize() run?'
      );
    }

    const characteristic = window.__TRANSPORT_MANAGER__?.notifyCharacteristic;
    if (!characteristic) {
      throw new Error(
        'No notify characteristic on window.__TRANSPORT_MANAGER__ — the transport is not ' +
          'connected, or it was built outside test/development mode so it never exposed one.'
      );
    }

    await testing.simulateNotification({ characteristic, data: packet });
  }

  /**
   * Wait for a specific domain event type, with an optional filter.
   */
  async waitForEvent(
    eventType: string,
    filter?: (event: NotificationEvent) => boolean,
    timeoutMs: number = 5000
  ): Promise<NotificationEvent> {
    const existing = this.domainEvents.find((e) => {
      if (e.type !== eventType) return false;
      if (filter) return filter(e);
      return true;
    });
    if (existing) {
      return existing;
    }

    return new Promise((resolve, reject) => {
      const timeoutHandle = setTimeout(() => {
        this.eventWaiters.delete(eventType);
        reject(new Error(`Timeout waiting for event: ${eventType}`));
      }, timeoutMs);

      this.eventWaiters.set(eventType, (event) => {
        if (filter && !filter(event)) {
          // Not this one. Stay registered for the next.
          return;
        }
        clearTimeout(timeoutHandle);
        this.eventWaiters.delete(eventType);
        resolve(event);
      });
    });
  }

  /** All captured domain events. */
  getEvents(): NotificationEvent[] {
    return [...this.domainEvents];
  }

  /** Captured domain events of one type. */
  getEventsByType(type: string): NotificationEvent[] {
    return this.domainEvents.filter((e) => e.type === type);
  }

  clearEvents(): void {
    this.domainEvents = [];
  }

  private getWorker(): CS108Reader {
    if (!this.worker) {
      throw new Error('Worker not initialized - call initialize() first');
    }
    return this.worker;
  }

  /**
   * Call a worker method through its public API.
   *
   * A helper to reduce duplication — it ONLY calls public methods.
   */
  private async callWorkerMethod<T>(method: string, ...args: unknown[]): Promise<T> {
    // Indexed by name because this helper fronts a dozen worker methods; the
    // `typeof` guard below is what makes the lookup safe, not the type.
    const worker = this.getWorker() as unknown as Record<string, (...a: unknown[]) => Promise<T>>;
    if (typeof worker[method] !== 'function') {
      throw new Error(`Worker method not found: ${method}`);
    }
    return worker[method](...args);
  }

  async connect(): Promise<boolean> {
    return this.callWorkerMethod('connect');
  }

  async disconnect(): Promise<void> {
    return this.callWorkerMethod('disconnect');
  }

  /**
   * @param mode - Reader mode to set
   * @param options - Optional parameters (e.g. targetEPC for LOCATE mode)
   */
  async setMode(mode: string, options?: { targetEPC?: string }): Promise<void> {
    return this.callWorkerMethod('setMode', mode, options);
  }

  async setSettings(settings: Partial<ReaderSettings>): Promise<void> {
    return this.callWorkerMethod('setSettings', settings);
  }

  getSettings(): Promise<ReaderSettings> {
    return this.callWorkerMethod('getSettings');
  }

  async startScanning(): Promise<void> {
    return this.callWorkerMethod('startScanning');
  }

  async stopScanning(): Promise<void> {
    return this.callWorkerMethod('stopScanning');
  }

  async waitForState(state: string, timeoutMs: number = 5000): Promise<void> {
    if (this.getReaderState() === state) {
      return;
    }

    await this.waitForEvent(
      'READER_STATE_CHANGED',
      (event) => 'payload' in event && (event.payload as { readerState?: string })?.readerState === state,
      timeoutMs
    );
  }

  /** OBSERVE: read-only. Never modify state directly. */
  getReaderState(): string {
    return (this.getWorker() as unknown as { readerState: string }).readerState;
  }

  /** OBSERVE: read-only. Never modify mode directly. */
  getReaderMode(): string | null {
    return (this.getWorker() as unknown as { readerMode: string | null }).readerMode;
  }

  /** OBSERVE: last percentage seen on a BATTERY_UPDATE event. */
  getBatteryPercentage(): number | null {
    const batteryEvents = this.getEventsByType(WorkerEventType.BATTERY_UPDATE);
    if (batteryEvents.length > 0) {
      const lastEvent = batteryEvents[batteryEvents.length - 1] as Extract<
        SystemEvent,
        { type: 'BATTERY_UPDATE' }
      >;
      return lastEvent.payload.percentage;
    }
    return null;
  }

  /** Whether the worker believes it has a device. */
  isConnected(): boolean {
    return this.getReaderState() !== 'Disconnected';
  }

  async simulateTriggerPress(): Promise<void> {
    const { TestPackets } = await import('../../config/cs108-packet-builder');
    await this.injectNotification(TestPackets.triggerPress());
  }

  async simulateTriggerRelease(): Promise<void> {
    const { TestPackets } = await import('../../config/cs108-packet-builder');
    await this.injectNotification(TestPackets.triggerRelease());
  }

  /**
   * Put the reader down safely, then drop the link and the mock.
   *
   * The stop-scanning / IDLE steps come first and swallow their errors on
   * purpose: they exist so the next test does not inherit a running scan or a
   * buzzing vibrator, and a reader that is already gone cannot be quieted.
   */
  async cleanup(): Promise<void> {
    if (this.worker) {
      try {
        await this.worker.stopScanning();
      } catch {
        // Not scanning.
      }

      try {
        await this.worker.setMode(ReaderMode.IDLE);
      } catch {
        // Already disconnected.
      }
    }

    try {
      await this.transport?.disconnect();
    } catch {
      // Teardown must complete; the transport reports its own errors.
    }
    this.transport = null;

    // AWAIT it. This is what collects the mock's post-disconnect wait, and what
    // stops the next spec file's connect racing this file's socket closes and
    // being refused as busy by our own previous session (TRA-1193).
    await this.uninstallMock?.();
    this.uninstallMock = null;

    this.domainEvents = [];
    this.eventWaiters.clear();
  }
}
