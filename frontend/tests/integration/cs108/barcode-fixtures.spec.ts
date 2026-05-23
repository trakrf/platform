/**
 * TRA-821 Fixture Replay Tests
 *
 * Feeds each canonical shape from the curated 100-cycle bridge log through
 * BarcodeDataHandler and asserts the assembled barcode value.
 * No hardware required — pure fixture replay.
 */

import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import * as path from 'node:path';

// Mock postWorkerEvent BEFORE any handler imports so the handler module
// receives the mock on its first import (same pattern as scan-handler.test.ts).
vi.mock('@/worker/types/events', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/worker/types/events')>();
  return {
    ...actual,
    postWorkerEvent: vi.fn(),
  };
});

import { BarcodeDataHandler } from '@/worker/cs108/barcode/scan-handler';
import { postWorkerEvent, WorkerEventType } from '@/worker/types/events';
import { ReaderMode, ReaderState } from '@/worker/types/reader';
import { BARCODE_DATA_NOTIFICATION } from '@/worker/cs108/event';
import type { NotificationContext } from '@/worker/cs108/notification/types';
import type { CS108Packet } from '@/worker/cs108/type';

const postMock = vi.mocked(postWorkerEvent);

// ---------------------------------------------------------------------------
// Fixture types
// ---------------------------------------------------------------------------

interface DataPacketEntry {
  ts: string;
  lenByte: number;
  expectedTotal: number;
  hex: string;
  payloadHex: string;
}

interface CanonicalShape {
  shape: string;
  dataPacketHex: DataPacketEntry[];
}

interface CuratedFixture {
  canonicals: Record<string, CanonicalShape>;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const hex = (s: string): Uint8Array =>
  new Uint8Array(s.trim().split(/\s+/).map(b => parseInt(b, 16)));

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
    payload: undefined,
    totalExpected: 10 + rawPayload.length,
    isComplete: true,
  } as never;
}

function buildContext(): NotificationContext {
  return {
    currentMode: ReaderMode.BARCODE,
    readerState: ReaderState.SCANNING,
    emitNotificationEvent: vi.fn(),
    metadata: { debug: false },
  } as never;
}

// ---------------------------------------------------------------------------
// Suite
// ---------------------------------------------------------------------------

describe('TRA-821 fixture replay', () => {
  let curated: CuratedFixture;

  beforeAll(async () => {
    const fixturesPath = path.resolve(
      path.dirname(fileURLToPath(import.meta.url)),
      '../../fixtures/cs108/tra-821/curated.json'
    );
    curated = JSON.parse(await readFile(fixturesPath, 'utf8')) as CuratedFixture;
  });

  beforeEach(() => {
    postMock.mockClear();
  });

  /**
   * Feed every data packet in a canonical shape through a fresh
   * BarcodeDataHandler and return only BARCODE_READ events.
   */
  async function replayShape(shape: string) {
    const handler = new BarcodeDataHandler();
    const ctx = buildContext();
    const packets = curated.canonicals[shape].dataPacketHex;

    for (const pkt of packets) {
      const rawPayload = hex(pkt.payloadHex);
      await handler.handle(buildBarcodePacket(rawPayload), ctx);
    }

    return postMock.mock.calls
      .map(([e]) => e)
      .filter((e): e is { type: WorkerEventType.BARCODE_READ; payload: { barcode: string } } =>
        (e as { type: string }).type === WorkerEventType.BARCODE_READ
      );
  }

  it('CLEAN_SINGLE canonical emits one read', async () => {
    const reads = await replayShape('CLEAN_SINGLE');
    expect(reads).toHaveLength(1);
    expect(reads[0].payload.barcode).toBe('712AC12F1007000000224401');
  });

  it('DATA_SPLIT_2PKT canonical emits one assembled read (no truncation, no leak)', async () => {
    const reads = await replayShape('DATA_SPLIT_2PKT');
    expect(reads).toHaveLength(1);
    expect(reads[0].payload.barcode).toBe('712AC12F1007000000224401');
  });

  it('BUNDLED_SECOND_PKT canonical recovers the bundled second record (one read after dup coalesce)', async () => {
    // Both records have the same barcode value; dup filter coalesces to one emit.
    const reads = await replayShape('BUNDLED_SECOND_PKT');
    expect(reads).toHaveLength(1);
    expect(reads[0].payload.barcode).toBe('712AC12F1007000000224401');
  });

  it('BUNDLED_MIXED canonical emits the assembled record (others dup-coalesced)', async () => {
    const reads = await replayShape('BUNDLED_MIXED');
    expect(reads).toHaveLength(1);
    expect(reads[0].payload.barcode).toBe('712AC12F1007000000224401');
  });
});
