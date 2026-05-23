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
});
