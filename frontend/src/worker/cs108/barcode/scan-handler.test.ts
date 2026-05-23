import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock the module that exports postWorkerEvent BEFORE importing the handler,
// so the handler's import receives our mock.
vi.mock('../../types/events', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../types/events')>();
  return {
    ...actual,
    postWorkerEvent: vi.fn(),
  };
});

import { BarcodeDataHandler } from './scan-handler';
import { postWorkerEvent, WorkerEventType } from '../../types/events';
import { ReaderMode, ReaderState } from '../../types/reader';
import type { NotificationContext } from '../notification/types';
import type { CS108Packet } from '../type';
import { BARCODE_DATA_NOTIFICATION } from '../event';

const postMock = vi.mocked(postWorkerEvent);

// Helper: hex string -> Uint8Array.
const hex = (s: string): Uint8Array =>
  new Uint8Array(s.trim().split(/\s+/).map(b => parseInt(b, 16)));

// Helper: build a minimal CS108Packet for 0x9100 with the given raw payload bytes.
// rawPayload is the bytes AFTER the 2-byte event code (what the accumulator gets).
function buildBarcodePacket(rawPayload: Uint8Array): CS108Packet {
  return {
    prefix: 0xA7B3,
    transport: 0xB3,
    length: rawPayload.length + 2,
    module: 0x6A,
    reserve: 0x82,
    direction: 0x9E,
    crc: 0,
    eventCode: 0x9100,
    event: BARCODE_DATA_NOTIFICATION,
    rawPayload,
    payload: undefined, // not used in the new accumulator path
    totalExpected: 10 + rawPayload.length,
    isComplete: true,
  };
}

function buildContext(overrides: Partial<NotificationContext> = {}): NotificationContext {
  return {
    currentMode: ReaderMode.BARCODE,
    readerState: ReaderState.SCANNING,
    emitNotificationEvent: vi.fn(),
    metadata: { debug: false },
    ...overrides,
  };
}

describe('BarcodeDataHandler', () => {
  let handler: BarcodeDataHandler;

  beforeEach(() => {
    postMock.mockReset();
    handler = new BarcodeDataHandler();
  });

  afterEach(() => {
    handler.cleanup();
  });

  it('emits one BARCODE_READ for a single-packet clean read', async () => {
    const packet = buildBarcodePacket(hex(
      '06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D'
    ));

    await handler.handle(packet, buildContext());

    const reads = postMock.mock.calls
      .map(c => c[0])
      .filter(e => e.type === WorkerEventType.BARCODE_READ);
    expect(reads).toHaveLength(1);
    expect(reads[0].payload).toMatchObject({
      barcode: '712AC12F1007000000224401',
      symbology: 'QR Code',
    });
  });

  it('assembles one BARCODE_READ from a data-split across two packets', async () => {
    const head = buildBarcodePacket(hex(
      '06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30'
    ));
    const tail = buildBarcodePacket(hex('31 05 01 11 16 03 04 0D'));

    await handler.handle(head, buildContext());
    await handler.handle(tail, buildContext());

    const reads = postMock.mock.calls
      .map(c => c[0])
      .filter(e => e.type === WorkerEventType.BARCODE_READ);
    expect(reads).toHaveLength(1);
    expect(reads[0].payload).toMatchObject({
      barcode: '712AC12F1007000000224401',
      symbology: 'QR Code',
    });
  });

  it('emits two BARCODE_READs for a bundled second-record-in-one-packet shape', async () => {
    // Two distinct values to bypass the 500ms dup filter and prove both records parsed.
    // Record A ends ...22440 + "1" => "712AC12F1007000000224401"
    // Record B ends ...22440 + "2" => "712AC12F1007000000224402"
    const recordA = hex(
      '06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D'
    );
    const recordB = hex(
      '02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30 32 05 01 11 16 03 04 0D'
    );
    const bundled = new Uint8Array(recordA.length + recordB.length);
    bundled.set(recordA);
    bundled.set(recordB, recordA.length);

    await handler.handle(buildBarcodePacket(bundled), buildContext());

    const reads = postMock.mock.calls
      .map(c => c[0])
      .filter(e => e.type === WorkerEventType.BARCODE_READ);
    expect(reads).toHaveLength(2);
    expect(reads[0].payload.barcode).toBe('712AC12F1007000000224401');
    expect(reads[1].payload.barcode).toBe('712AC12F1007000000224402');
  });

  it('coalesces duplicate values within the 500ms window', async () => {
    const packet = buildBarcodePacket(hex(
      '06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D'
    ));

    await handler.handle(packet, buildContext());
    await handler.handle(packet, buildContext());

    const reads = postMock.mock.calls
      .map(c => c[0])
      .filter(e => e.type === WorkerEventType.BARCODE_READ);
    expect(reads).toHaveLength(1); // second was dup-filtered
  });

  it('ignores a status-ping 0x9100 payload [0x06]', async () => {
    await handler.handle(buildBarcodePacket(hex('06')), buildContext());

    const reads = postMock.mock.calls
      .map(c => c[0])
      .filter(e => e.type === WorkerEventType.BARCODE_READ);
    expect(reads).toHaveLength(0);
  });

  it('requests auto-stop at most once per call even when multiple records emit', async () => {
    const recordA = hex(
      '06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D'
    );
    const recordB = hex(
      '02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30 32 05 01 11 16 03 04 0D'
    );
    const bundled = new Uint8Array(recordA.length + recordB.length);
    bundled.set(recordA);
    bundled.set(recordB, recordA.length);

    const ctx = buildContext();
    await handler.handle(buildBarcodePacket(bundled), ctx);

    const stopRequests = (ctx.emitNotificationEvent as ReturnType<typeof vi.fn>).mock.calls
      .map(c => c[0])
      .filter(e => e.type === WorkerEventType.BARCODE_AUTO_STOP_REQUEST);
    expect(stopRequests).toHaveLength(1);
  });

  it('does not request auto-stop when reader is not SCANNING', async () => {
    const packet = buildBarcodePacket(hex(
      '06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D'
    ));
    const ctx = buildContext({ readerState: ReaderState.CONNECTED });

    await handler.handle(packet, ctx);

    const stopRequests = (ctx.emitNotificationEvent as ReturnType<typeof vi.fn>).mock.calls
      .map(c => c[0])
      .filter(e => e.type === WorkerEventType.BARCODE_AUTO_STOP_REQUEST);
    expect(stopRequests).toHaveLength(0);
  });
});
