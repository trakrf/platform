/**
 * Tests for CS108 packet parsing
 */

import { describe, it, expect, beforeEach } from 'vitest';
import { PacketHandler } from './packet.js';
import { parsePacket, calculatePacketCRC, validatePacketCRC, validatePacketLength } from './protocol.js';
import { RFID_POWER_OFF, RFID_POWER_ON, TRIGGER_PRESSED_NOTIFICATION, INVENTORY_TAG_NOTIFICATION } from './event.js';

/**
 * Split a packet into BLE MTU-sized fragments (20 bytes each)
 */
function fragmentPacket(packet: Uint8Array, mtuSize = 20): Uint8Array[] {
  const fragments: Uint8Array[] = [];
  for (let offset = 0; offset < packet.length; offset += mtuSize) {
    const size = Math.min(mtuSize, packet.length - offset);
    fragments.push(packet.slice(offset, offset + size));
  }
  return fragments;
}

describe('parsePacket', () => {
  it('should parse a valid command response packet', () => {
    const handler = new PacketHandler();
    // Build a valid RFID_POWER_OFF response using packet builder
    const data = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));

    const packet = parsePacket(data);

    expect(packet).not.toBeNull();
    expect(packet?.prefix).toBe(0xA7);
    expect(packet?.transport).toBe(0xB3);
    expect(packet?.module).toBe(0xC2);
    expect(packet?.direction).toBe(0x9E);
    expect(packet?.eventCode).toBe(0x8001);
    expect(packet?.event).toBe(RFID_POWER_OFF);
    expect(packet?.rawPayload).toEqual(new Uint8Array([0x00]));
    expect(packet?.payload).toBe(0x00); // parseUint8 returns the byte value
    expect(packet?.isComplete).toBe(true);
  });

  it('should return null for incomplete packet', () => {
    // Only header, no event code - use raw bytes for invalid packet test
    const data = new Uint8Array([
      0xA7, 0xB3, 0x03, 0xC2, 0x82, 0x9E, 0x00, 0x00
    ]);

    const packet = parsePacket(data);
    expect(packet).toBeNull();
  });

  it('should return null for invalid prefix', () => {
    // Invalid prefix - must use raw bytes for invalid packet test
    const data = new Uint8Array([
      0xFF, // Wrong prefix
      0xB3, 0x03, 0xC2, 0x82, 0x9E, 0x00, 0x00,
      0x80, 0x01, 0x00
    ]);

    const packet = parsePacket(data);
    expect(packet).toBeNull();
  });

  it('should return null for invalid reserve byte', () => {
    // Invalid reserve byte - must use raw bytes for invalid packet test
    const data = new Uint8Array([
      0xA7, 0xB3, 0x03, 0xC2,
      0xFF, // Wrong reserve byte
      0x9E, 0x00, 0x00,
      0x80, 0x01, 0x00
    ]);

    const packet = parsePacket(data);
    expect(packet).toBeNull();
  });

  it('should throw for unknown event code', () => {
    // Unknown event code - must use raw bytes for invalid packet test
    const data = new Uint8Array([
      0xA7, 0xB3, 0x03, 0xC2, 0x82, 0x9E, 0x00, 0x00,
      0xFF, 0xFF, // Unknown event code 0xFFFF
      0x00
    ]);

    expect(() => parsePacket(data)).toThrow('Unknown CS108 event code: 0xffff');
  });

  it('should handle USB transport byte', () => {
    const handler = new PacketHandler();
    handler.setTransportType(true); // USB
    const data = handler.buildResponse(RFID_POWER_ON, new Uint8Array([0x00]));

    const packet = parsePacket(data);

    expect(packet).not.toBeNull();
    expect(packet?.transport).toBe(0xE6); // USB transport
    expect(packet?.event).toBe(RFID_POWER_ON);
  });

  it('should handle downlink direction', () => {
    const handler = new PacketHandler();
    const data = handler.buildCommand(RFID_POWER_ON);

    const packet = parsePacket(data);

    expect(packet).not.toBeNull();
    expect(packet?.direction).toBe(0x37); // Downlink
  });

  it('should correctly identify command vs notification events', () => {
    const handler = new PacketHandler();
    const commandPacket = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));

    const packet = parsePacket(commandPacket);

    expect(packet).not.toBeNull();
    expect(packet?.event.isCommand).toBe(true);
    expect(packet?.event.isNotification).toBe(false);
  });
});

describe('PacketHandler BLE Fragmentation', () => {
  let handler: PacketHandler;

  beforeEach(() => {
    handler = new PacketHandler();
  });

  it('should reassemble fragmented inventory notification packet', () => {
    // Build a valid inventory notification with LOCATE mode payload using packet builder
    // Payload: mode(1) + protocol(1) + pc(2) + epc(12) + rssi(4) + phase(2) + antenna(1) + extra(5) = 28 bytes
    const locatePayload = new Uint8Array([
      0x03,                                           // LOCATE mode
      0x12,                                           // Protocol info
      0x05, 0x80,                                     // PC bytes
      0x07, 0x00, 0x00, 0x00, 0x93, 0x1A, 0x00, 0x00, // EPC part 1
      0x80, 0x5E, 0x1F, 0x0F,                         // EPC part 2
      0x00, 0x00, 0x00, 0x00,                         // RSSI bytes
      0x30, 0x00,                                     // Phase
      0x00,                                           // Antenna
      0x00, 0x00, 0x00, 0x01, 0x00                    // Extra data
    ]);

    // Build complete packet using packet builder (includes valid CRC)
    const completePacket = handler.buildNotification(INVENTORY_TAG_NOTIFICATION, locatePayload);

    // Fragment into BLE MTU chunks
    const fragments = fragmentPacket(completePacket);
    expect(fragments.length).toBeGreaterThan(1); // Should fragment

    // Process fragments with a fresh handler
    const receiver = new PacketHandler();
    let result: ReturnType<typeof receiver.processIncomingData> = [];

    for (let i = 0; i < fragments.length - 1; i++) {
      result = receiver.processIncomingData(fragments[i]);
      expect(result).toEqual([]); // Should buffer, not complete yet
    }

    // Last fragment completes the packet
    result = receiver.processIncomingData(fragments[fragments.length - 1]);
    expect(result.length).toBe(1);

    const packet = result[0];
    expect(packet.prefix).toBe(0xA7B3);
    expect(packet.eventCode).toBe(0x8100); // INVENTORY_TAG_NOTIFICATION
    expect(packet.rawPayload?.[0]).toBe(0x03); // LOCATE mode
  });

  it('should handle maximum size packet fragmentation (128 bytes = 7 fragments)', () => {
    // Build a maximum size inventory notification (120 byte payload)
    const maxPayload = new Uint8Array(118); // 120 - 2 (event code) = 118 bytes
    maxPayload[0] = 0x03; // LOCATE mode indicator
    for (let i = 1; i < maxPayload.length; i++) {
      maxPayload[i] = (i + 9) & 0xFF; // Fill with pattern starting at 0x0A
    }

    // Build complete packet using packet builder
    const completePacket = handler.buildNotification(INVENTORY_TAG_NOTIFICATION, maxPayload);
    expect(completePacket.length).toBe(128); // 8 header + 2 event + 118 payload

    // Fragment into 20-byte chunks
    const fragments = fragmentPacket(completePacket);
    expect(fragments.length).toBe(7); // Should be 7 fragments

    // Process all fragments with a fresh handler
    const receiver = new PacketHandler();
    let result: ReturnType<typeof receiver.processIncomingData> = [];

    for (let i = 0; i < fragments.length - 1; i++) {
      result = receiver.processIncomingData(fragments[i]);
      expect(result).toEqual([]); // Should buffer until complete
    }

    // Last fragment should complete the packet
    result = receiver.processIncomingData(fragments[fragments.length - 1]);
    expect(result.length).toBe(1);

    const packet = result[0];
    expect(packet.prefix).toBe(0xA7B3);
    expect(packet.length).toBe(0x78); // 120 bytes (event code + payload)
    expect(packet.eventCode).toBe(0x8100); // INVENTORY_TAG
    expect(packet.rawPayload).toBeDefined();
    expect(packet.rawPayload.length).toBe(118);

    // Verify payload pattern
    expect(packet.rawPayload[0]).toBe(0x03); // LOCATE mode
    expect(packet.rawPayload[1]).toBe(0x0A); // Pattern continues
    expect(packet.rawPayload[2]).toBe(0x0B);
  });
});

describe('PacketHandler uplink building', () => {
  const handler = new PacketHandler();

  it('should build uplink response packets', () => {
    const response = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));

    // Check direction byte is uplink (0x9E)
    expect(response[5]).toBe(0x9E);

    // Verify event code
    expect(response[8]).toBe(0x80);  // 0x8001 big-endian
    expect(response[9]).toBe(0x01);
  });

  it('should build uplink notification packets', () => {
    const notification = handler.buildNotification(TRIGGER_PRESSED_NOTIFICATION);

    // Check direction byte is uplink (0x9E)
    expect(notification[5]).toBe(0x9E);

    // Verify it's a notification event (0xA102)
    expect(notification[8]).toBe(0xA1);  // 0xA102 big-endian
    expect(notification[9]).toBe(0x02);
  });

  it('should allow CRC injection for testing', () => {
    const customCRC = 0x1234;
    const packet = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]), { crc: customCRC });

    // Check that custom CRC was injected (big-endian: high byte first)
    expect(packet[6]).toBe(0x12);  // CRC high byte at position 6
    expect(packet[7]).toBe(0x34);  // CRC low byte at position 7
  });

  it('should build both downlink and uplink packets', () => {
    // Build command (downlink)
    const command = handler.buildCommand(RFID_POWER_OFF);
    expect(command[5]).toBe(0x37);  // Downlink direction

    // Build response (uplink) for same event
    const response = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));
    expect(response[5]).toBe(0x9E);  // Uplink direction

    // Event codes should match
    expect(command[8]).toBe(response[8]);
    expect(command[9]).toBe(response[9]);
  });
});

describe('CS108 Packet Validation', () => {
  describe('CRC Calculation', () => {
    it('should calculate CRC that validates correctly', () => {
      const handler = new PacketHandler();
      // Build a packet with auto-calculated CRC
      const packet = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));

      // Validate the CRC
      const result = validatePacketCRC(packet);
      expect(result.valid).toBe(true);
      expect(result.expected).toBe(result.actual);
    });

    it('should parse CRC as big-endian', () => {
      const handler = new PacketHandler();
      const packet = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));

      // Extract CRC from packet (big-endian)
      const crcFromPacket = (packet[6] << 8) | packet[7];

      const result = validatePacketCRC(packet);
      expect(result.expected).toBe(crcFromPacket);
      expect(result.valid).toBe(true);
    });

    it('should skip validation when CRC is zero', () => {
      const handler = new PacketHandler();
      // Inject zero CRC (which means skip validation per spec)
      const packet = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]), { crc: 0x0000 });

      const result = validatePacketCRC(packet);
      expect(result.valid).toBe(true);
      expect(result.expected).toBe(0);
      expect(result.actual).toBe(0);
    });

    it('should detect CRC mismatch', () => {
      const handler = new PacketHandler();
      // Inject invalid CRC
      const packet = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]), { crc: 0xFFFF });

      const result = validatePacketCRC(packet);
      expect(result.valid).toBe(false);
      expect(result.expected).toBe(0xFFFF); // What's in packet
    });
  });

  describe('Length Validation', () => {
    it('should validate correct length', () => {
      const handler = new PacketHandler();
      const packet = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));

      const result = validatePacketLength(packet);
      expect(result.valid).toBe(true);
      expect(result.expected).toBe(result.actual);
    });

    it('should detect length mismatch - too short', () => {
      const handler = new PacketHandler();
      const packet = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));

      // Truncate the packet
      const truncated = packet.slice(0, packet.length - 1);

      const result = validatePacketLength(truncated);
      expect(result.valid).toBe(false);
      expect(result.actual).toBe(truncated.length);
    });

    it('should detect length mismatch - too long', () => {
      const handler = new PacketHandler();
      const packet = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));

      // Add extra byte
      const extended = new Uint8Array(packet.length + 1);
      extended.set(packet);
      extended[packet.length] = 0xFF;

      const result = validatePacketLength(extended);
      expect(result.valid).toBe(false);
      expect(result.actual).toBe(extended.length);
    });
  });

  describe('PacketHandler Integration', () => {
    let handler: PacketHandler;

    beforeEach(() => {
      handler = new PacketHandler();
    });

    it('should reject packets with invalid CRC', () => {
      // Build a valid packet then corrupt the CRC
      const validPacket = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));

      // Corrupt the CRC bytes
      validPacket[6] = 0xFF;
      validPacket[7] = 0xFF;

      const receiver = new PacketHandler();
      const packets = receiver.processIncomingData(validPacket);
      expect(packets.length).toBe(0); // Should be rejected
    });

    it('should accept packets with valid CRC', () => {
      // Build a valid response packet (includes correct CRC)
      const validPacket = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]));

      const receiver = new PacketHandler();
      const packets = receiver.processIncomingData(validPacket);
      expect(packets.length).toBe(1);
      expect(packets[0].eventCode).toBe(0x8001); // RFID_POWER_OFF
    });

    it('should accept packets with zero CRC (validation skipped)', () => {
      // Build packet with zero CRC (skip validation per spec)
      const zeroCrcPacket = handler.buildResponse(RFID_POWER_OFF, new Uint8Array([0x00]), { crc: 0x0000 });

      const receiver = new PacketHandler();
      const packets = receiver.processIncomingData(zeroCrcPacket);
      expect(packets.length).toBe(1);
      expect(packets[0].eventCode).toBe(0x8001);
    });
  });
});
