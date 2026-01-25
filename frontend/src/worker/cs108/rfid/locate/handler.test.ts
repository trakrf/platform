/**
 * Tests for RFID locate mode notification handler
 * Including fragmentation handling for 38-byte LOCATE mode packets
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { PacketHandler } from '../../packet';
import { ReaderMode } from '../../../types/reader';
import type { NotificationContext } from '../../notification/types';
import type { CS108Packet } from '../../type';
import { INVENTORY_TAG_NOTIFICATION } from '../../event';
import type { LocateTagHandler } from './handler';

/**
 * Split a packet into BLE MTU-sized fragments (20 bytes each)
 * Used to simulate real BLE fragmentation behavior
 */
function fragmentPacket(packet: Uint8Array, mtuSize = 20): Uint8Array[] {
  const fragments: Uint8Array[] = [];
  for (let offset = 0; offset < packet.length; offset += mtuSize) {
    const size = Math.min(mtuSize, packet.length - offset);
    fragments.push(packet.slice(offset, offset + size));
  }
  return fragments;
}

/**
 * Build a LOCATE mode inventory notification payload
 * Format: mode(1) + protocol(1) + epcWordCount(1) + EPC bytes + RSSI data
 */
function buildLocatePayload(epcBytes: Uint8Array, rssiValue = 0xCA): Uint8Array {
  const payload = new Uint8Array(3 + epcBytes.length + 10);
  let offset = 0;

  // LOCATE mode indicator
  payload[offset++] = 0x03;
  // Protocol info
  payload[offset++] = 0x00;
  // EPC word count (in words, 2 bytes each)
  payload[offset++] = epcBytes.length / 2;
  // EPC bytes
  payload.set(epcBytes, offset);
  offset += epcBytes.length;
  // RSSI and additional data (10 bytes typical)
  payload[offset++] = 0x00;
  payload[offset++] = 0x00;
  payload[offset++] = rssiValue;
  payload[offset++] = 0x30;
  payload[offset++] = 0x00;
  payload[offset++] = 0x00;
  payload[offset++] = 0x00;
  payload[offset++] = 0x2E;
  payload[offset++] = 0x00;
  payload[offset++] = 0x80;

  return payload;
}

/**
 * Build a compact mode inventory notification payload
 * Used for canHandle/handle tests
 */
function buildCompactInventoryPayload(epcBytes: Uint8Array, rssiValue = 0x2D): Uint8Array {
  // Compact mode packet format:
  // RFID protocol header (8 bytes) + PC (2) + EPC (12) + RSSI (2)
  const payload = new Uint8Array(8 + 2 + epcBytes.length + 2);
  let offset = 0;

  // RFID protocol header (8 bytes)
  payload[offset++] = 0x04;       // pktVer = 0x04 for compact mode
  payload[offset++] = 0x00;       // flags
  payload[offset++] = 0x05;       // pktType low (0x8005 little-endian)
  payload[offset++] = 0x80;       // pktType high
  const payloadLen = 2 + epcBytes.length + 2; // PC + EPC + RSSI
  payload[offset++] = payloadLen & 0xFF;  // payloadLen low
  payload[offset++] = (payloadLen >> 8) & 0xFF;  // payloadLen high
  payload[offset++] = 0x01;       // antennaPort = 1
  payload[offset++] = 0x00;       // reserved

  // Tag data payload
  payload[offset++] = 0x30;       // PC high
  payload[offset++] = 0x00;       // PC low
  payload.set(epcBytes, offset);  // EPC bytes
  offset += epcBytes.length;
  payload[offset++] = rssiValue;  // NB_RSSI
  payload[offset++] = 0x00;

  return payload;
}

// Standard test EPC: E28068940000501EC3B8BAE9 (12 bytes)
const TEST_EPC = new Uint8Array([
  0xE2, 0x80, 0x68, 0x94, 0x00, 0x00, 0x50, 0x1E,
  0xC3, 0xB8, 0xBA, 0xE9
]);

// Mock the worker event posting
vi.mock('../../../types/events', () => ({
  WorkerEventType: {
    LOCATE_UPDATE: 'LOCATE_UPDATE'
  },
  postWorkerEvent: vi.fn()
}));

describe('LocateHandler', () => {
  let handler: LocateTagHandler;
  let packetHandler: PacketHandler;

  beforeEach(async () => {
    vi.clearAllMocks();
    // Import and instantiate after mocks are cleared
    const { LocateTagHandler: HandlerClass } = await import('./handler');
    handler = new HandlerClass();
    packetHandler = new PacketHandler();
  });

  afterEach(() => {
    handler.cleanup();
  });

  describe('canHandle', () => {
    it('should only handle packets in LOCATE mode', () => {
      // Build a valid compact mode inventory packet using helper
      const rawPayload = buildCompactInventoryPayload(TEST_EPC);

      const packet: CS108Packet = {
        prefix: 0xA7B3,
        transport: 0xB3,
        length: rawPayload.length + 2, // payload + event code
        module: 0xC2,
        reserve: 0x82,
        direction: 0x9E,
        crc: 0,
        eventCode: 0x8100,
        event: INVENTORY_TAG_NOTIFICATION,
        rawPayload,
        payload: undefined, // Parser will generate this
        totalExpected: 8 + rawPayload.length + 2,
        isComplete: true
      };

      // Should not handle in INVENTORY mode
      const inventoryContext: NotificationContext = {
        currentMode: ReaderMode.INVENTORY,
        currentConnection: null,
        metadata: {}
      };
      expect(handler.canHandle(packet, inventoryContext)).toBe(false);

      // Should handle in LOCATE mode
      const locateContext: NotificationContext = {
        currentMode: ReaderMode.LOCATE,
        currentConnection: null,
        metadata: {}
      };
      expect(handler.canHandle(packet, locateContext)).toBe(true);
    });
  });

  describe('fragmentation handling', () => {
    it('should handle fragmented LOCATE mode packets from real hardware', () => {
      // Build a LOCATE mode inventory notification using packet builder
      const locatePayload = buildLocatePayload(TEST_EPC);
      const completePacket = packetHandler.buildNotification(INVENTORY_TAG_NOTIFICATION, locatePayload);

      // Fragment into BLE MTU chunks (20 bytes each)
      const fragments = fragmentPacket(completePacket);
      expect(fragments.length).toBeGreaterThan(1); // Should fragment

      // Process fragments with a fresh handler
      const receiver = new PacketHandler();
      let result: ReturnType<typeof receiver.processIncomingData> = [];

      for (let i = 0; i < fragments.length - 1; i++) {
        result = receiver.processIncomingData(fragments[i]);
        expect(result).toEqual([]); // Should return empty - waiting for more data
      }

      // Process last fragment - should complete the packet
      result = receiver.processIncomingData(fragments[fragments.length - 1]);
      expect(result.length).toBe(1);

      const packet = result[0];
      expect(packet).toBeDefined();
      expect(packet.prefix).toBe(0xA7B3); // Combined prefix value
      expect(packet.eventCode).toBe(0x8100); // INVENTORY_TAG_NOTIFICATION

      // The packet should have the LOCATE mode signature in the payload
      // rawPayload starts after the event code, so first byte is 0x03 (LOCATE mode)
      expect(packet.rawPayload[0]).toBe(0x03);
    });

    it('should handle multiple consecutive fragmented packets', () => {
      // Build two different LOCATE mode packets with different EPC endings
      const epc1 = new Uint8Array([...TEST_EPC]);
      const epc2 = new Uint8Array([...TEST_EPC]);
      epc2[11] = 0xEA; // Different last byte

      const packet1 = packetHandler.buildNotification(INVENTORY_TAG_NOTIFICATION, buildLocatePayload(epc1, 0xCA));
      const packet2 = packetHandler.buildNotification(INVENTORY_TAG_NOTIFICATION, buildLocatePayload(epc2, 0xC5));

      // Fragment both packets
      const fragments1 = fragmentPacket(packet1);
      const fragments2 = fragmentPacket(packet2);

      const receiver = new PacketHandler();
      let totalPacketsReceived = 0;

      // Process first packet's fragments
      for (let i = 0; i < fragments1.length - 1; i++) {
        const result = receiver.processIncomingData(fragments1[i]);
        expect(result).toEqual([]);
      }
      let result = receiver.processIncomingData(fragments1[fragments1.length - 1]);
      expect(result.length).toBe(1);
      expect(result[0].rawPayload[0]).toBe(0x03); // LOCATE mode indicator
      totalPacketsReceived++;

      // Process second packet's fragments
      for (let i = 0; i < fragments2.length - 1; i++) {
        const result = receiver.processIncomingData(fragments2[i]);
        expect(result).toEqual([]);
      }
      result = receiver.processIncomingData(fragments2[fragments2.length - 1]);
      expect(result.length).toBe(1);
      expect(result[0].rawPayload[0]).toBe(0x03); // LOCATE mode indicator
      totalPacketsReceived++;

      expect(totalPacketsReceived).toBe(2);
    });

    it('should recover when receiving new packet instead of expected fragment', () => {
      // Build a LOCATE mode packet and get its first fragment only
      const locatePayload = buildLocatePayload(TEST_EPC);
      const incompletePacket = packetHandler.buildNotification(INVENTORY_TAG_NOTIFICATION, locatePayload);
      const incompleteFragments = fragmentPacket(incompletePacket);

      // Start with first fragment only
      let result = packetHandler.processIncomingData(incompleteFragments[0]);
      expect(result).toEqual([]); // Waiting for more data

      // Instead of sending fragment2, send a NEW complete (small) packet
      // This simulates lost fragments scenario
      const smallPayload = new Uint8Array([0x01, 0x00, 0x04, 0x00, 0x00, 0x56, 0x78, 0x00]);
      const newCompletePacket = packetHandler.buildNotification(INVENTORY_TAG_NOTIFICATION, smallPayload);

      result = packetHandler.processIncomingData(newCompletePacket);
      expect(result.length).toBe(1); // Should process the new packet
      expect(result[0].prefix).toBe(0xA7B3);
      expect(result[0].eventCode).toBe(0x8100); // INVENTORY_TAG_NOTIFICATION
    });

    it('should timeout and reset on incomplete fragments', async () => {
      vi.useFakeTimers();

      // Build a LOCATE mode packet and get its first fragment only
      const locatePayload = buildLocatePayload(TEST_EPC);
      const incompletePacket = packetHandler.buildNotification(INVENTORY_TAG_NOTIFICATION, locatePayload);
      const incompleteFragments = fragmentPacket(incompletePacket);

      // Send first fragment only
      const result = packetHandler.processIncomingData(incompleteFragments[0]);
      expect(result).toEqual([]);

      // Advance time past fragment timeout (200ms)
      vi.advanceTimersByTime(250);

      // Send a new complete packet - should work despite previous incomplete fragment
      const smallPayload = new Uint8Array([0x01, 0x00, 0x04, 0x00, 0x00, 0x56, 0x78, 0x00]);
      const newPacket = packetHandler.buildNotification(INVENTORY_TAG_NOTIFICATION, smallPayload);

      const newResult = packetHandler.processIncomingData(newPacket);
      expect(newResult.length).toBe(1);
      expect(newResult[0].eventCode).toBe(0x8100); // INVENTORY_TAG_NOTIFICATION

      vi.useRealTimers();
    });
  });

  describe('handle', () => {
    it('should emit LOCATE_UPDATE events with RSSI smoothing', async () => {
      // Set up Date.now mock to control throttling timing
      let mockTime = 1000000;
      vi.spyOn(Date, 'now').mockImplementation(() => mockTime);

      const context: NotificationContext = {
        currentMode: ReaderMode.LOCATE,
        currentConnection: null,
        metadata: {}
      };

      // Handle multiple packets to test RSSI smoothing
      for (let i = 0; i < 5; i++) {
        // Create packet with varying RSSI using helper
        const rssiValue = 0x2D - i; // Vary RSSI slightly (-45 to -49)
        const rawPayload = buildCompactInventoryPayload(TEST_EPC, rssiValue);

        const testPacket: CS108Packet = {
          prefix: 0xA7B3,
          transport: 0xB3,
          length: rawPayload.length + 2,
          module: 0xC2,
          reserve: 0x82,
          direction: 0x9E,
          crc: 0,
          eventCode: 0x8100,
          event: INVENTORY_TAG_NOTIFICATION,
          rawPayload,
          payload: undefined, // Parser will generate this
          totalExpected: 8 + rawPayload.length + 2,
          isComplete: true
        };

        const canHandle = handler.canHandle(testPacket, context);
        if (canHandle) {
          try {
            handler.handle(testPacket, context);
            // Advance mock time by 60ms to bypass throttling (MIN_UPDATE_INTERVAL_MS = 50)
            mockTime += 60;
          } catch (error) {
            console.error(`Error handling packet ${i}:`, error);
          }
        }
      }

      vi.restoreAllMocks();

      // Verify handler processed the updates correctly
      const stats = handler.getStats();
      expect(stats.updatesProcessed).toBe(5);

      // The RSSI values were 0x2D to 0x29 (-45 to -41 in signed byte)
      // Parser converts these properly, so we expect proper RSSI averaging
      expect(stats.averageRssi).toBeLessThan(-20); // Should be negative RSSI
      expect(stats.smoothedRssi).toBeLessThan(-20); // Should be negative RSSI
    });
  });
});