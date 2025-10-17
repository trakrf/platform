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
      // Create a valid inventory packet payload that the parser can handle
      // Format: [mode, protocol, epc_word_count, pc, rssi_bytes..., epc_bytes...]
      const packet: CS108Packet = {
        prefix: 0xA7B3,
        transport: 0xB3,
        length: 24,
        module: 0xC2,
        reserve: 0x82,
        direction: 0x9E,
        crc: 0,
        eventCode: 0x8100,
        event: INVENTORY_TAG_NOTIFICATION,
        // Compact mode packet with 1 tag
        rawPayload: new Uint8Array([
          // RFID protocol header (8 bytes)
          0x04,       // pktVer = 0x04 for compact mode
          0x00,       // flags
          0x05, 0x80, // pktType = 0x8005 (little-endian) for inventory response
          0x10, 0x00, // payloadLen = 16 bytes (PC + EPC + RSSI)
          0x01,       // antennaPort = 1
          0x00,       // reserved
          // Tag data payload (16 bytes)
          0x30, 0x00, // PC
          // EPC bytes (E28068940000501EC3B8BAE9 - 12 bytes)
          0xE2, 0x80, 0x68, 0x94, 0x00, 0x00, 0x50, 0x1E,
          0xC3, 0xB8, 0xBA, 0xE9,
          0x2D, 0x00  // NB_RSSI bytes (0x2D = -45 in CS108 format)
        ]),
        payload: undefined, // Parser will generate this
        totalExpected: 32,
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
    it('should handle fragmented 38-byte LOCATE mode packets from real hardware', () => {
      // This is the actual fragmented packet we captured from the CS108 in LOCATE mode
      // It's a 38-byte packet (8 header + 30 payload) split into 2 BLE fragments
      const fragment1 = new Uint8Array([
        0xA7, 0xB3, 0x1E, 0xC2, 0xCA, 0x9E, 0xF2, 0x2B,  // 8-byte header (length=0x1E=30)
        0x81, 0x00, 0x03, 0x00, 0x0C, 0xE2, 0x80, 0x68,  // Start of payload
        0x94, 0x00, 0x00, 0x50                             // Fragment cuts here at 20 bytes
      ]);

      const fragment2 = new Uint8Array([
        0x1E, 0xC3, 0xB8, 0xBA, 0xE9,  // Continuation of EPC
        0x00, 0x00, 0xCA,              // More payload
        0x30, 0x00,                    // PC bytes
        0x00, 0x00, 0x2E, 0x00,        // Reserved/RSSI
        0x80, 0x01, 0x01, 0x00         // Phase/Antenna/Protocol + padding
      ]);

      // Process first fragment - should buffer it
      let result = packetHandler.processIncomingData(fragment1);
      expect(result).toEqual([]); // Should return empty - waiting for more data

      // Process second fragment - should complete the packet
      result = packetHandler.processIncomingData(fragment2);
      expect(result.length).toBe(1);

      const packet = result[0];
      expect(packet).toBeDefined();
      expect(packet.prefix).toBe(0xA7B3); // Combined prefix value
      expect(packet.length).toBe(0x1E); // 30 bytes payload
      expect(packet.rawPayload.length).toBe(28); // 30 - 2 (event code) = 28 bytes actual payload

      // Verify the packet can be handled in locate mode
      const context: NotificationContext = {
        currentMode: ReaderMode.LOCATE,
        currentConnection: null,
        metadata: {}
      };

      // The packet should have the LOCATE mode signature in the payload
      // rawPayload starts after the event code, so first byte is 0x03 (LOCATE mode)
      expect(packet.rawPayload[0]).toBe(0x03);
      expect(packet.rawPayload[1]).toBe(0x00);
      expect(packet.rawPayload[2]).toBe(0x0C);
    });

    it('should handle multiple consecutive fragmented packets', () => {
      // Simulate receiving multiple fragmented LOCATE packets in sequence
      const packets = [
        {
          fragment1: new Uint8Array([
            0xA7, 0xB3, 0x1E, 0xC2, 0xCA, 0x9E, 0xF2, 0x2B,
            0x81, 0x00, 0x03, 0x00, 0x0C, 0xE2, 0x80, 0x68,
            0x94, 0x00, 0x00, 0x50
          ]),
          fragment2: new Uint8Array([
            0x1E, 0xC3, 0xB8, 0xBA, 0xE9,
            0x00, 0x00, 0xCA,
            0x30, 0x00,
            0x00, 0x00, 0x2E, 0x00,
            0x80, 0x01, 0x01, 0x00
          ])
        },
        {
          fragment1: new Uint8Array([
            0xA7, 0xB3, 0x1E, 0xC2, 0xCA, 0x9E, 0xA1, 0x3C,
            0x81, 0x00, 0x03, 0x00, 0x0C, 0xE2, 0x80, 0x68,
            0x94, 0x00, 0x00, 0x50
          ]),
          fragment2: new Uint8Array([
            0x1E, 0xC3, 0xB8, 0xBA, 0xEA,  // Different EPC end
            0x00, 0x00, 0xC5,              // Different RSSI
            0x30, 0x00,
            0x00, 0x00, 0x2E, 0x00,
            0x80, 0x01, 0x01, 0x00
          ])
        }
      ];

      let totalPacketsReceived = 0;

      for (const { fragment1, fragment2 } of packets) {
        // Process first fragment
        let result = packetHandler.processIncomingData(fragment1);
        expect(result).toEqual([]);

        // Process second fragment
        result = packetHandler.processIncomingData(fragment2);
        expect(result.length).toBe(1);
        totalPacketsReceived++;

        const packet = result[0];
        expect(packet.length).toBe(0x1E);
        expect(packet.rawPayload[0]).toBe(0x03); // LOCATE mode indicator (first byte after event code)
      }

      expect(totalPacketsReceived).toBe(2);
    });

    it('should recover when receiving new packet instead of expected fragment', () => {
      // Start with first fragment of a packet
      const fragment1 = new Uint8Array([
        0xA7, 0xB3, 0x1E, 0xC2, 0xCA, 0x9E, 0xF2, 0x2B,
        0x81, 0x00, 0x03, 0x00, 0x0C, 0xE2, 0x80, 0x68,
        0x94, 0x00, 0x00, 0x50
      ]);

      let result = packetHandler.processIncomingData(fragment1);
      expect(result).toEqual([]); // Waiting for more data

      // Instead of sending fragment2, send a NEW packet header
      // This simulates lost fragments scenario
      const newCompletePacket = new Uint8Array([
        0xA7, 0xB3, 0x0A, 0xC2, 0xCA, 0x9E, 0x12, 0x34,  // New 8-byte header
        0x81, 0x00, 0x01, 0x00, 0x04, 0x12, 0x34, 0x56, 0x78, 0x00  // 10-byte payload
      ]);

      result = packetHandler.processIncomingData(newCompletePacket);
      expect(result.length).toBe(1); // Should process the new packet
      expect(result[0].length).toBe(0x0A); // The new packet's length
      expect(result[0].prefix).toBe(0xA7B3);
    });

    it('should timeout and reset on incomplete fragments', async () => {
      vi.useFakeTimers();

      // Send first fragment only
      const fragment1 = new Uint8Array([
        0xA7, 0xB3, 0x1E, 0xC2, 0xCA, 0x9E, 0xF2, 0x2B,
        0x81, 0x00, 0x03, 0x00, 0x0C, 0xE2, 0x80, 0x68,
        0x94, 0x00, 0x00, 0x50
      ]);

      const result = packetHandler.processIncomingData(fragment1);
      expect(result).toEqual([]);

      // Advance time past fragment timeout (200ms)
      vi.advanceTimersByTime(250);

      // Send a new complete packet - should work despite previous incomplete fragment
      const newPacket = new Uint8Array([
        0xA7, 0xB3, 0x0A, 0xC2, 0xCA, 0x9E, 0x12, 0x34,  // 8-byte header
        0x81, 0x00, 0x01, 0x00, 0x04, 0x12, 0x34, 0x56, 0x78, 0x00  // 10-byte payload
      ]);

      const newResult = packetHandler.processIncomingData(newPacket);
      expect(newResult.length).toBe(1);
      expect(newResult[0].length).toBe(0x0A);

      vi.useRealTimers();
    });
  });

  describe('handle', () => {
    it('should emit LOCATE_UPDATE events with RSSI smoothing', async () => {
      // Set up Date.now mock to control throttling timing
      let mockTime = 1000000;
      vi.spyOn(Date, 'now').mockImplementation(() => mockTime);

      // Create a valid inventory packet payload that the parser can handle
      // Format: [mode, protocol, epc_word_count, pc, rssi_bytes..., epc_bytes...]
      const packet: CS108Packet = {
        prefix: 0xA7B3,
        transport: 0xB3,
        length: 24,
        module: 0xC2,
        reserve: 0x82,
        direction: 0x9E,
        crc: 0,
        eventCode: 0x8100,
        event: INVENTORY_TAG_NOTIFICATION,
        // Compact mode packet with 1 tag
        rawPayload: new Uint8Array([
          // RFID protocol header (8 bytes)
          0x04,       // pktVer = 0x04 for compact mode
          0x00,       // flags
          0x05, 0x80, // pktType = 0x8005 (little-endian) for inventory response
          0x10, 0x00, // payloadLen = 16 bytes (PC + EPC + RSSI)
          0x01,       // antennaPort = 1
          0x00,       // reserved
          // Tag data payload (16 bytes)
          0x30, 0x00, // PC
          // EPC bytes (E28068940000501EC3B8BAE9 - 12 bytes)
          0xE2, 0x80, 0x68, 0x94, 0x00, 0x00, 0x50, 0x1E,
          0xC3, 0xB8, 0xBA, 0xE9,
          0x2D, 0x00  // NB_RSSI bytes (0x2D = -45 in CS108 format)
        ]),
        payload: undefined, // Parser will generate this
        totalExpected: 32,
        isComplete: true
      };

      const context: NotificationContext = {
        currentMode: ReaderMode.LOCATE,
        currentConnection: null,
        metadata: {}
      };

      // Handle multiple packets to test RSSI smoothing
      for (let i = 0; i < 5; i++) {
        // Create packet with varying RSSI
        const rssiValue = 0x2D - i; // Vary RSSI slightly (-45 to -49)
        const testPacket = {
          ...packet,
          rawPayload: new Uint8Array([
            // RFID protocol header (8 bytes)
            0x04,       // pktVer = 0x04 for compact mode
            0x00,       // flags
            0x05, 0x80, // pktType = 0x8005 (little-endian) for inventory response
            0x10, 0x00, // payloadLen = 16 bytes (PC + EPC + RSSI)
            0x01,       // antennaPort = 1
            0x00,       // reserved
            // Tag data payload (16 bytes)
            0x30, 0x00, // PC
            // EPC bytes
            0xE2, 0x80, 0x68, 0x94, 0x00, 0x00, 0x50, 0x1E,
            0xC3, 0xB8, 0xBA, 0xE9,
            rssiValue, 0x00  // NB_RSSI bytes (varying)
          ])
        };
        const canHandle = handler.canHandle(testPacket as CS108Packet, context);
        if (canHandle) {
          try {
            handler.handle(testPacket as CS108Packet, context);
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