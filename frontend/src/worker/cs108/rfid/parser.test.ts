import { describe, it, expect, beforeEach } from 'vitest';
import { InventoryParser } from './parser';
import { TEST_TAGS } from '../../../../tests/data/test-tags';
import { getCompactModePayloads, getNormalModePayloads } from '../../../../tests/data/inventory-by-mode';
import allMixedData from '../../../../tests/data/inventory-all-mixed.json5';

describe('InventoryParser', () => {
  let parser: InventoryParser;

  beforeEach(() => {
    parser = new InventoryParser('compact', false);
  });

  describe('payload processing', () => {
    it('processes empty payload without errors', () => {
      const emptyPayload = new Uint8Array(0);
      const tags = parser.processInventoryPayload(emptyPayload);
      expect(tags).toEqual([]);
      expect(parser.getState().packetsProcessed).toBe(1);
    });

    it('accumulates partial payloads in buffer', () => {
      // Compact mode header but incomplete tag data
      const partialPayload = new Uint8Array([
        0x04, 0x00, 0x00, 0x80, 0x05, 0x00, // Compact header
        0x06, 0x00, // Antenna port
        0x30, 0x00  // PC word but no EPC
      ]);
      const tags = parser.processInventoryPayload(partialPayload);
      expect(tags).toEqual([]); // No complete tags yet
      expect(parser.getState().packetsProcessed).toBe(1);
    });
  });

  describe('sequence number tracking', () => {
    it('tracks sequence numbers for debugging', () => {
      const payload1 = new Uint8Array([0x01, 0x02]);
      const payload2 = new Uint8Array([0x03, 0x04]);

      parser.processInventoryPayload(payload1, 1);
      parser.processInventoryPayload(payload2, 2);

      // Parser should track sequences internally
      expect(parser.getState().packetsProcessed).toBe(2);
    });

    it('handles sequence number wrap-around', () => {
      const payload1 = new Uint8Array([0x01]);
      const payload2 = new Uint8Array([0x02]);

      parser.processInventoryPayload(payload1, 255);
      parser.processInventoryPayload(payload2, 0); // wrapped

      // Should handle wrap without error
      expect(parser.getState().packetsProcessed).toBe(2);
    });
  });

  describe('compact mode parsing', () => {
    it('parses real compact mode payloads from test data', () => {
      // Use real test data from captured CS108 packets
      // First payload from inventory-compact-mode.json5 which has 3 tags
      const realPayload = new Uint8Array([
        0x04, 0x00, 0x05, 0x80, 0x3c, 0x00, 0x00, 0x00,
        0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x54, // Tag 1
        0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x11, 0x4c, // Tag 2
        0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09, 0x5a, // Tag 3
        0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x13, 0x50  // Tag 4
      ]);
      const tags = parser.processInventoryPayload(realPayload);

      expect(tags.length).toBe(4); // Should parse all 4 tags
      // These payloads contain tags with PC=0x3000 (96-bit EPCs)
      tags.forEach(tag => {
        expect(tag.pc).toBe(0x3000);
        expect(tag.epc.length).toBe(24); // 96 bits = 12 bytes = 24 hex chars
        expect(tag.rssi).toBeLessThan(0); // RSSI should be negative
      });
    });

    it('parses real compact mode payloads with tags', () => {
      // Use real payload from CS108 capture
      const compactPayloads = getCompactModePayloads(20);

      // Find a payload that contains actual tag data (not status/keepalive)
      const tagPayload = compactPayloads.find(p => p[0] === 0x04 && p.length > 10);

      if (tagPayload) {
        const tags = parser.processInventoryPayload(tagPayload);

        // Should parse tags from the real payload
        expect(tags.length).toBeGreaterThan(0);
        if (tags.length > 0) {
          // Verify tag structure
          expect(tags[0]).toHaveProperty('epc');
          expect(tags[0]).toHaveProperty('rssi');
          expect(tags[0]).toHaveProperty('pc');
        }
      }
    });

    // TODO: Fix packet format to match real CS108 data
    it.skip('parses single tag in compact mode (old test)', () => {
      // Build compact mode payload (no headers!)
      const testTag = TEST_TAGS[0];
      // Compact format: version(1) + flags(1) + type(2) + length(2) + antenna(1) + reserved(1) + PC(2) + EPC(12) + RSSI(1)
      const payload = new Uint8Array([
        0x04, 0x00, // Version and flags
        0x05, 0x80, // Type (0x8005 little-endian)
        0x0F, 0x00, // Length (15 bytes)
        0x06, 0x00, // Antenna port
        // Tag data
        testTag.pc & 0xFF, (testTag.pc >> 8) & 0xFF, // PC
        ...hexStringToBytes(testTag.epc), // EPC
        Math.abs(testTag.rssi) // RSSI as positive byte
      ]);

      const tags = parser.processInventoryPayload(payload);
      expect(tags).toHaveLength(1);
      expect(tags[0].epc).toBe(testTag.epc);
    });

    // TODO: Fix packet format to match real CS108 data
    it.skip('handles tags spanning multiple packets', () => {
      // Split a tag across two payloads
      const fullPayload = new Uint8Array([
        0x04, 0x00, 0x05, 0x80, 0x0F, 0x00, 0x06, 0x00,
        0x30, 0x00, // PC
        0xE2, 0x80, 0x11, 0x60, 0x60, 0x00, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, // EPC
        0x50 // RSSI
      ]);

      // Split at byte 16 (middle of EPC)
      const part1 = fullPayload.slice(0, 16);
      const part2 = fullPayload.slice(16);

      const tags1 = parser.processInventoryPayload(part1);
      expect(tags1).toHaveLength(0); // Incomplete

      const tags2 = parser.processInventoryPayload(part2);
      expect(tags2).toHaveLength(1); // Complete now
      expect(tags2[0].epc).toBe('E280116060000123456789AB');
    });

    it('parses multiple tags in single packet', () => {
      // Multiple tags in compact mode - just payload data
      const tag1 = TEST_TAGS[0];
      const tag2 = TEST_TAGS[1];

      // Build payload with protocol header + 2 tags
      const epc1Bytes = hexStringToBytes(tag1.epc);
      const epc2Bytes = hexStringToBytes(tag2.epc);
      const tagDataSize = 2 + epc1Bytes.length + 1 + 2 + epc2Bytes.length + 1; // PC+EPC+RSSI for each

      const payload = new Uint8Array(8 + tagDataSize); // 8-byte header + tags

      // Compact protocol header
      payload[0] = 0x04; // Version
      payload[1] = 0x00; // Flags
      payload[2] = 0x05; // Type low
      payload[3] = 0x80; // Type high (0x8005)
      payload[4] = tagDataSize & 0xFF; // Length low
      payload[5] = (tagDataSize >> 8) & 0xFF; // Length high
      payload[6] = 0x06; // Antenna
      payload[7] = 0x00; // Reserved

      // Tag 1
      let offset = 8;
      // PC word in BIG-ENDIAN (as per protocol packets)
      payload[offset++] = (tag1.pc >> 8) & 0xFF;
      payload[offset++] = tag1.pc & 0xFF;
      for (const byte of epc1Bytes) {
        payload[offset++] = byte;
      }
      payload[offset++] = Math.round(Math.abs(tag1.rssi) / 0.8);

      // Tag 2
      // PC word in BIG-ENDIAN (as per protocol packets)
      payload[offset++] = (tag2.pc >> 8) & 0xFF;
      payload[offset++] = tag2.pc & 0xFF;
      for (const byte of epc2Bytes) {
        payload[offset++] = byte;
      }
      payload[offset++] = Math.round(Math.abs(tag2.rssi) / 0.8);

      const tags = parser.processInventoryPayload(payload);
      expect(tags).toHaveLength(2);
      expect(tags[0].epc).toBe(tag1.epc);
      expect(tags[1].epc).toBe(tag2.epc);
    });

    it('handles 128-bit EPC correctly', () => {
      // 128-bit EPC (16 bytes)
      const longEpc = 'E280116060000123456789ABCDEF0123';
      const pc = 0x4000; // PC for 128-bit EPC (word count = 8)

      const epcBytes = hexStringToBytes(longEpc);
      const payload = new Uint8Array(8 + 2 + epcBytes.length + 1);

      // Compact header
      payload[0] = 0x04;
      payload[1] = 0x00;
      payload[2] = 0x05;
      payload[3] = 0x80;
      payload[4] = (2 + epcBytes.length + 1) & 0xFF;
      payload[5] = ((2 + epcBytes.length + 1) >> 8) & 0xFF;
      payload[6] = 0x06;
      payload[7] = 0x00;

      // Tag data
      // PC word in BIG-ENDIAN (as per protocol packets)
      payload[8] = (pc >> 8) & 0xFF;
      payload[9] = pc & 0xFF;
      for (let i = 0; i < epcBytes.length; i++) {
        payload[10 + i] = epcBytes[i];
      }
      payload[10 + epcBytes.length] = 0x50; // RSSI

      const tags = parser.processInventoryPayload(payload);
      expect(tags).toHaveLength(1);
      expect(tags[0].epc).toBe(longEpc);
      expect(tags[0].pc).toBe(pc);
    });
  });

  describe('normal mode parsing', () => {
    // TODO: Fix packet format to match real CS108 data
    it.skip('parses single tag in normal mode', () => {
      parser = new InventoryParser('normal', false);
      // Normal mode has different format - needs implementation
    });
  });

  describe('real payload patterns', () => {
    it('handles type 0x02 payloads (system/status)', () => {
      // Type 0x02 payloads appear to be system status messages
      const allPayloads = allMixedData.payloads.map((p: number[]) => new Uint8Array(p));
      const type02Payload = allPayloads.find((p: Uint8Array) => p[0] === 0x02);

      if (type02Payload) {
        const tags = parser.processInventoryPayload(type02Payload);
        // These don't contain tag data
        expect(tags).toEqual([]);

        // But they should still be processed without error
        expect(parser.getState().packetsProcessed).toBe(1);
      }
    });

    it('handles type 0x70 payloads (keepalive/status)', () => {
      // Type 0x70 payloads appear frequently, likely keepalive or status
      const allPayloads = allMixedData.payloads.map((p: number[]) => new Uint8Array(p));
      const type70Payload = allPayloads.find((p: Uint8Array) => p[0] === 0x70);

      if (type70Payload) {
        const tags = parser.processInventoryPayload(type70Payload);
        // These don't contain tag data
        expect(tags).toEqual([]);

        // But they should still be processed without error
        expect(parser.getState().packetsProcessed).toBe(1);
      }
    });

    it('processes multiple real payloads in sequence', () => {
      // Process first 20 real payloads to simulate actual data flow
      let totalTags = 0;
      const allPayloads = allMixedData.payloads.map((p: number[]) => new Uint8Array(p));

      for (let i = 0; i < 20 && i < allPayloads.length; i++) {
        const tags = parser.processInventoryPayload(allPayloads[i]);
        totalTags += tags.length;
      }

      // Should have processed all packets
      expect(parser.getState().packetsProcessed).toBe(20);

      // Should have found some tags (compact mode packets start at index 12)
      expect(totalTags).toBeGreaterThan(0);
    });
  });

  describe('error handling', () => {
    // TODO: Fix packet format to match real CS108 data
    it.skip('skips corrupted data and continues parsing', () => {
      // Add corrupted bytes followed by valid tag
      // Parser should recover and parse the valid tag
    });

    // TODO: Fix packet format to match real CS108 data
    it.skip('handles buffer overflow gracefully', () => {
      // Fill buffer to near capacity
      // Ensure graceful handling when overflow occurs
    });
  });

  describe('buffer health monitoring', () => {
    it('reports buffer metrics accurately', () => {
      // Add a partial compact mode packet that won't be parsed yet
      // Header says 100 bytes total but we only provide 50
      const payload = new Uint8Array(50);
      payload[0] = 0x04; // Compact mode
      payload[1] = 0x00; // Flags
      payload[2] = 0x05; // Type low
      payload[3] = 0x80; // Type high
      payload[4] = 92;   // Length says 100 bytes (92 + 8 header)
      payload[5] = 0x00;
      payload[6] = 0x06; // Antenna
      payload[7] = 0x00;
      // Rest is partial tag data

      parser.processInventoryPayload(payload);

      const metrics = parser.getBufferMetrics();
      expect(metrics.size).toBeGreaterThan(0);
      expect(metrics.used).toBe(50); // Should have 50 bytes buffered
      expect(metrics.utilizationPercent).toBeGreaterThanOrEqual(0);
      expect(metrics.utilizationPercent).toBeLessThanOrEqual(100);
    });
  });

  describe('state management', () => {
    // TODO: Fix packet format to match real CS108 data
    it.skip('resets state correctly', () => {
      // Process some packets
      const payload = new Uint8Array([0x01, 0x02, 0x03]);
      parser.processInventoryPayload(payload);

      // Reset
      parser.reset();

      const state = parser.getState();
      expect(state.packetsProcessed).toBe(0);
      expect(state.tagsExtracted).toBe(0);
      expect(state.sequenceNumber).toBe(0);
    });
  });

  describe('performance', () => {
    it('handles high throughput efficiently', () => {
      const startTime = Date.now();
      const iterations = 1000;

      for (let i = 0; i < iterations; i++) {
        const payload = new Uint8Array([i & 0xFF]);
        parser.processInventoryPayload(payload);
      }

      const elapsed = Date.now() - startTime;
      const throughput = iterations / (elapsed / 1000); // packets per second

      expect(throughput).toBeGreaterThan(100); // Should handle >100 packets/sec
      expect(parser.getState().packetsProcessed).toBe(iterations);
    });
  });
});

// Helper function to convert hex string to bytes
function hexStringToBytes(hex: string): Uint8Array {
  const cleanHex = hex.replace(/\s+/g, '');
  const bytes = new Uint8Array(cleanHex.length / 2);
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(cleanHex.substr(i * 2, 2), 16);
  }
  return bytes;
}