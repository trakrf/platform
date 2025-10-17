/**
 * Tests for CS108 packet parsing
 */

import { describe, it, expect, beforeEach } from 'vitest';
import { PacketHandler } from './packet.js';
import { parsePacket } from './protocol.js';
import { RFID_POWER_OFF, RFID_POWER_ON, TRIGGER_PRESSED_NOTIFICATION } from './event.js';

describe('parsePacket', () => {
  it('should parse a valid command response packet', () => {
    // RFID_POWER_OFF response packet
    const data = new Uint8Array([
      0xA7, // Prefix
      0xB3, // Transport (BT)
      0x03, // Length (3 bytes after header)
      0xC2, // Module (RFID)
      0x82, // Reserve
      0x9E, // Direction (uplink)
      0x00, 0x00, // CRC (simplified)
      0x80, 0x01, // Event code 0x8001 (big-endian)
      0x00  // Success byte
    ]);
    
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
    // Only header, no event code
    const data = new Uint8Array([
      0xA7, 0xB3, 0x03, 0xC2, 0x82, 0x9E, 0x00, 0x00
    ]);
    
    const packet = parsePacket(data);
    expect(packet).toBeNull();
  });
  
  it('should return null for invalid prefix', () => {
    const data = new Uint8Array([
      0xFF, // Wrong prefix
      0xB3, 0x03, 0xC2, 0x82, 0x9E, 0x00, 0x00,
      0x80, 0x01, 0x00
    ]);
    
    const packet = parsePacket(data);
    expect(packet).toBeNull();
  });
  
  it('should return null for invalid reserve byte', () => {
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
    const data = new Uint8Array([
      0xA7, 0xB3, 0x03, 0xC2, 0x82, 0x9E, 0x00, 0x00,
      0xFF, 0xFF, // Unknown event code 0xFFFF
      0x00
    ]);
    
    expect(() => parsePacket(data)).toThrow('Unknown CS108 event code: 0xffff');
  });
  
  it('should handle USB transport byte', () => {
    const data = new Uint8Array([
      0xA7,
      0xE6, // USB transport
      0x03, 0xC2, 0x82, 0x9E, 0x00, 0x00,
      0x80, 0x00, // Event code 0x8000 (RFID_POWER_ON)
      0x00
    ]);
    
    const packet = parsePacket(data);
    
    expect(packet).not.toBeNull();
    expect(packet?.transport).toBe(0xE6);
    expect(packet?.event).toBe(RFID_POWER_ON);
  });
  
  it('should handle downlink direction', () => {
    const data = new Uint8Array([
      0xA7, 0xB3, 0x03, 0xC2, 0x82,
      0x37, // Downlink direction
      0x00, 0x00,
      0x80, 0x00, // Event code 0x8000
      0x00
    ]);
    
    const packet = parsePacket(data);
    
    expect(packet).not.toBeNull();
    expect(packet?.direction).toBe(0x37);
  });
  
  it('should correctly identify command vs notification events', () => {
    const commandPacket = new Uint8Array([
      0xA7, 0xB3, 0x03, 0xC2, 0x82, 0x9E, 0x00, 0x00,
      0x80, 0x01, // RFID_POWER_OFF (command)
      0x00
    ]);
    
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

  it('should reassemble fragmented LOCATE mode inventory packet from real bridge logs', () => {
    // Actual fragmented packet captured from bridge logs
    // This is a LOCATE mode inventory report (81 00 03 signature)
    // Total packet length: 0x26 (38 bytes) split across 3 BLE transmissions

    // Fragment 1: First 20 bytes including header
    const fragment1 = new Uint8Array([
      0xA7, 0xB3, 0x26, 0xC2, 0xCA, 0x9E, 0xF2, 0x2B,  // Header (8 bytes)
      0x81, 0x00, 0x03, 0x12, 0x05, 0x80, 0x07, 0x00,  // Start of payload
      0x00, 0x00, 0x93, 0x1A
    ]);

    // Fragment 2: Next 20 bytes (continuation)
    const fragment2 = new Uint8Array([
      0x00, 0x00, 0x80, 0x5E, 0x1F, 0x0F, 0x00, 0x00,
      0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00,
      0x00, 0x00, 0x00, 0x00
    ]);

    // Fragment 3: Final 6 bytes
    const fragment3 = new Uint8Array([
      0x00, 0x01, 0x00, 0x21, 0x0E, 0xDE
    ]);

    // Process fragments in sequence
    const result1 = handler.processIncomingData(fragment1);
    expect(result1).toEqual([]); // Should buffer, not complete yet

    const result2 = handler.processIncomingData(fragment2);
    expect(result2).toEqual([]); // Still buffering

    const result3 = handler.processIncomingData(fragment3);
    expect(result3.length).toBe(1); // Now we should have complete packet

    // Verify the reassembled packet
    const packet = result3[0];
    expect(packet.prefix).toBe(0xA7B3);
    expect(packet.length).toBe(0x26); // 38 bytes (includes event code + payload)
    expect(packet.eventCode).toBe(0x8100); // Inventory notification
    expect(packet.rawPayload).toBeDefined();
    expect(packet.rawPayload?.length).toBe(36); // 38 - 2 (event code) = 36 bytes actual payload

    // Verify LOCATE mode signature (rawPayload starts AFTER event code)
    // The bytes 81 00 are the event code, not part of rawPayload
    // rawPayload starts with 03 (LOCATE mode indicator)
    expect(packet.rawPayload?.[0]).toBe(0x03); // LOCATE mode
    expect(packet.rawPayload?.[1]).toBe(0x12); // Next byte from fragment
    expect(packet.rawPayload?.[2]).toBe(0x05); // Continuation
  });

  it('should handle maximum size packet fragmentation (128 bytes = 7 fragments)', () => {
    // Create a maximum size packet: 8 byte header + 120 byte payload = 128 bytes
    // This will fragment into 7 BLE packets (6x20 + 1x8)

    const maxPacket = new Uint8Array(128);
    // Header
    maxPacket[0] = 0xA7; // Prefix byte 1
    maxPacket[1] = 0xB3; // Prefix byte 2
    maxPacket[2] = 0x78; // Length (120 bytes = 0x78)
    maxPacket[3] = 0xC2; // Header byte
    maxPacket[4] = 0xCA; // Header byte
    maxPacket[5] = 0x9E; // Header byte
    maxPacket[6] = 0xF2; // Header byte
    maxPacket[7] = 0x2B; // Header byte

    // Add event code (bytes 8-9)
    maxPacket[8] = 0x81;  // Event code high byte (INVENTORY_TAG)
    maxPacket[9] = 0x00;  // Event code low byte

    // Fill rest with test data (starting from byte 10)
    for (let i = 10; i < 128; i++) {
      maxPacket[i] = i & 0xFF;
    }

    // Fragment into 20-byte chunks
    const fragments: Uint8Array[] = [];
    for (let offset = 0; offset < 128; offset += 20) {
      const size = Math.min(20, 128 - offset);
      fragments.push(maxPacket.slice(offset, offset + size));
    }

    expect(fragments.length).toBe(7); // Should be 7 fragments

    // Process all fragments
    let result: any[] = [];
    for (let i = 0; i < fragments.length - 1; i++) {
      result = handler.processIncomingData(fragments[i]);
      expect(result).toEqual([]); // Should buffer until complete
    }

    // Last fragment should complete the packet
    result = handler.processIncomingData(fragments[fragments.length - 1]);
    expect(result.length).toBe(1);

    const packet = result[0];
    expect(packet.prefix).toBe(0xA7B3); // Combined prefix value
    expect(packet.length).toBe(0x78); // 120 bytes (includes event code)
    expect(packet.eventCode).toBe(0x8100); // INVENTORY_TAG
    expect(packet.rawPayload).toBeDefined();
    expect(packet.rawPayload.length).toBe(118); // 120 - 2 (event code) = 118 bytes actual payload

    // Verify first few bytes of payload (should be our test pattern starting at 0x0A)
    expect(packet.rawPayload[0]).toBe(0x0A); // First byte after event code
    expect(packet.rawPayload[1]).toBe(0x0B);
    expect(packet.rawPayload[2]).toBe(0x0C);
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
    
    // Check that custom CRC was injected
    expect(packet[6]).toBe(0x34);  // CRC low byte
    expect(packet[7]).toBe(0x12);  // CRC high byte
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