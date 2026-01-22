/**
 * CS108 Protocol Constants and Utilities
 *
 * Low-level protocol definitions for CS108 RFID reader:
 * - Packet structure constants
 * - CRC calculation
 * - Basic packet parsing
 *
 * CS108 Packet Structure:
 * [0]: Prefix (0xA7)
 * [1]: Transport (0xB3=BT, 0xE6=USB)
 * [2]: Data length (size after 8-byte header)
 * [3]: Module (0xC2=RFID, 0x6A=Barcode, 0xD9=System)
 * [4]: Reserve byte (0x82)
 * [5]: Direction (0x37=downlink, 0x9E=uplink)
 * [6-7]: CRC (big-endian per vendor spec: high byte at [6], low byte at [7])
 * [8-9]: Event code (little-endian)
 * [10+]: Payload data
 */

import type { CS108Packet } from './type.js';
import { logger } from '../utils/logger.js';
import type { CS108PayloadType } from './payload-types.js';
import { CS108_EVENT_MAP } from './event.js';

// CS108 Protocol Constants
export const PACKET_CONSTANTS = {
  // Fixed header byte values
  PREFIX_BYTE: 0xA7,         // Byte 0: Always 0xA7
  RESERVE_BYTE: 0x82,        // Byte 4: Always 0x82
  DOWNLINK_DIRECTION: 0x37,  // Byte 5: Commands (downlink)
  UPLINK_DIRECTION: 0x9E,    // Byte 5: Responses/notifications (uplink)

  // Transport byte values (variable)
  TRANSPORT_USB: 0xE6,       // USB connection
  TRANSPORT_BLUETOOTH: 0xB3, // Bluetooth connection

  // Structure
  MIN_LENGTH: 10,            // 8 header + 2 event code minimum
  MAX_DATA_SIZE: 120,        // Maximum data size after header
  MAX_PACKET_SIZE: 128,      // Maximum total packet size (header + data)
  HEADER_SIZE: 8,            // Header size before event code

  // Offsets
  PREFIX_OFFSET: 0,
  TRANSPORT_OFFSET: 1,
  LENGTH_OFFSET: 2,          // Single byte
  MODULE_OFFSET: 3,
  RESERVE_OFFSET: 4,
  DIRECTION_OFFSET: 5,
  CRC_OFFSET: 6,             // CRC at bytes 6-7
  EVENT_CODE_OFFSET: 8,      // Event code at bytes 8-9
  PAYLOAD_OFFSET: 10         // Payload starts at byte 10
};

/**
 * Parse a CS108 packet from raw bytes
 * Returns CS108Packet or null if incomplete
 * Throws on unknown event codes (fail-fast)
 */
export function parsePacket(data: Uint8Array): CS108Packet | null {
  // Need at least header + event code
  if (data.length < 10) {
    return null;
  }

  // Parse header fields
  const prefix = data[0];
  const transport = data[1];
  const length = data[2];
  const module = data[3];
  const reserve = data[4];
  const direction = data[5];
  const crc = (data[6] << 8) | data[7]; // Big-endian per vendor spec

  // Validate fixed bytes
  if (prefix !== PACKET_CONSTANTS.PREFIX_BYTE) {
    return null; // Invalid packet
  }

  if (reserve !== PACKET_CONSTANTS.RESERVE_BYTE) {
    return null; // Invalid packet
  }

  // Validate transport byte
  if (transport !== PACKET_CONSTANTS.TRANSPORT_BLUETOOTH &&
      transport !== PACKET_CONSTANTS.TRANSPORT_USB) {
    return null; // Invalid transport
  }

  // Validate direction
  if (direction !== PACKET_CONSTANTS.DOWNLINK_DIRECTION &&
      direction !== PACKET_CONSTANTS.UPLINK_DIRECTION) {
    return null; // Invalid direction
  }

  // Validate length
  if (length > PACKET_CONSTANTS.MAX_DATA_SIZE) {
    return null; // Invalid length
  }

  // Calculate expected total size
  const totalExpected = PACKET_CONSTANTS.HEADER_SIZE + length;
  const isComplete = data.length >= totalExpected;

  // If not complete, return null (need more data)
  if (!isComplete) {
    return null;
  }

  // Parse event code (big-endian)
  const eventCode = (data[8] << 8) | data[9];

  // Look up event - MUST exist (fail-fast)
  const event = CS108_EVENT_MAP.get(eventCode);
  if (!event) {
    logger.error(`[PacketHandler] Unknown event code: 0x${eventCode.toString(16).padStart(4, '0')}`);
    throw new Error(`Unknown CS108 event code: 0x${eventCode.toString(16).padStart(4, '0')}`);
  }
  logger.debug(`[PacketHandler] Parsed packet for event: ${event.name} (0x${eventCode.toString(16).padStart(4, '0')})`)

  // Extract raw payload (everything after event code)
  const rawPayload = data.length > 10 ? data.slice(10, totalExpected) : new Uint8Array(0);

  // Parse payload if event has a parser
  let payload: CS108PayloadType = undefined;
  if (event.parser && rawPayload.length > 0) {
    try {
      payload = event.parser(rawPayload);
    } catch (error) {
      logger.warn(`[parsePacket] Parser failed for ${event.name}:`, error);
      // Don't fail the packet parse, just leave payload undefined
    }
  }

  return {
    // Header fields
    prefix,
    transport,
    length,
    module,
    reserve,
    direction,
    crc,

    // Event identification
    eventCode,
    event,

    // Payload
    rawPayload,
    payload,

    // Computed fields
    totalExpected,
    isComplete: true
  };
}

/**
 * Calculate CRC-16 for CS108 packet
 * Implementation from backup/pre-consolidation branch lib/rfid/cs108/utils.ts
 */
export function calculateCRC(data: Uint8Array): number {
  // CRC-16 lookup table from CS108 specification
  const crcLookupTable: number[] = [
    0x0000, 0x1189, 0x2312, 0x329b, 0x4624, 0x57ad, 0x6536, 0x74bf,
    0x8c48, 0x9dc1, 0xaf5a, 0xbed3, 0xca6c, 0xdbe5, 0xe97e, 0xf8f7,
    0x1081, 0x0108, 0x3393, 0x221a, 0x56a5, 0x472c, 0x75b7, 0x643e,
    0x9cc9, 0x8d40, 0xbfdb, 0xae52, 0xdaed, 0xcb64, 0xf9ff, 0xe876,
    0x2102, 0x308b, 0x0210, 0x1399, 0x6726, 0x76af, 0x4434, 0x55bd,
    0xad4a, 0xbcc3, 0x8e58, 0x9fd1, 0xeb6e, 0xfae7, 0xc87c, 0xd9f5,
    0x3183, 0x200a, 0x1291, 0x0318, 0x77a7, 0x662e, 0x54b5, 0x453c,
    0xbdcb, 0xac42, 0x9ed9, 0x8f50, 0xfbef, 0xea66, 0xd8fd, 0xc974,
    0x4204, 0x538d, 0x6116, 0x709f, 0x0420, 0x15a9, 0x2732, 0x36bb,
    0xce4c, 0xdfc5, 0xed5e, 0xfcd7, 0x8868, 0x99e1, 0xab7a, 0xbaf3,
    0x5285, 0x430c, 0x7197, 0x601e, 0x14a1, 0x0528, 0x37b3, 0x263a,
    0xdecd, 0xcf44, 0xfddf, 0xec56, 0x98e9, 0x8960, 0xbbfb, 0xaa72,
    0x6306, 0x728f, 0x4014, 0x519d, 0x2522, 0x34ab, 0x0630, 0x17b9,
    0xef4e, 0xfec7, 0xcc5c, 0xddd5, 0xa96a, 0xb8e3, 0x8a78, 0x9bf1,
    0x7387, 0x620e, 0x5095, 0x411c, 0x35a3, 0x242a, 0x16b1, 0x0738,
    0xffcf, 0xee46, 0xdcdd, 0xcd54, 0xb9eb, 0xa862, 0x9af9, 0x8b70,
    0x8408, 0x9581, 0xa71a, 0xb693, 0xc22c, 0xd3a5, 0xe13e, 0xf0b7,
    0x0840, 0x19c9, 0x2b52, 0x3adb, 0x4e64, 0x5fed, 0x6d76, 0x7cff,
    0x9489, 0x8500, 0xb79b, 0xa612, 0xd2ad, 0xc324, 0xf1bf, 0xe036,
    0x18c1, 0x0948, 0x3bd3, 0x2a5a, 0x5ee5, 0x4f6c, 0x7df7, 0x6c7e,
    0xa50a, 0xb483, 0x8618, 0x9791, 0xe32e, 0xf2a7, 0xc03c, 0xd1b5,
    0x2942, 0x38cb, 0x0a50, 0x1bd9, 0x6f66, 0x7eef, 0x4c74, 0x5dfd,
    0xb58b, 0xa402, 0x9699, 0x8710, 0xf3af, 0xe226, 0xd0bd, 0xc134,
    0x39c3, 0x284a, 0x1ad1, 0x0b58, 0x7fe7, 0x6e6e, 0x5cf5, 0x4d7c,
    0xc60c, 0xd785, 0xe51e, 0xf497, 0x8028, 0x91a1, 0xa33a, 0xb2b3,
    0x4a44, 0x5bcd, 0x6956, 0x78df, 0x0c60, 0x1de9, 0x2f72, 0x3efb,
    0xd68d, 0xc704, 0xf59f, 0xe416, 0x90a9, 0x8120, 0xb3bb, 0xa232,
    0x5ac5, 0x4b4c, 0x79d7, 0x685e, 0x1ce1, 0x0d68, 0x3ff3, 0x2e7a,
    0xe70e, 0xf687, 0xc41c, 0xd595, 0xa12a, 0xb0a3, 0x8238, 0x93b1,
    0x6b46, 0x7acf, 0x4854, 0x59dd, 0x2d62, 0x3ceb, 0x0e70, 0x1ff9,
    0xf78f, 0xe606, 0xd49d, 0xc514, 0xb1ab, 0xa022, 0x92b9, 0x8330,
    0x7bc7, 0x6a4e, 0x58d5, 0x495c, 0x3de3, 0x2c6a, 0x1ef1, 0x0f78
  ];

  let crc: number = 0;

  for (let i = 0; i < data.length; i++) {
    const index: number = (crc ^ data[i]) & 0xff;
    const tableValue: number = crcLookupTable[index];
    crc = ((crc >> 8) ^ tableValue) & 0xffff; // Ensure 16-bit result
  }

  return crc;
}

// CRC lookup table for calculatePacketCRC (same as above, but exported as constant)
const CRC_LOOKUP_TABLE: number[] = [
  0x0000, 0x1189, 0x2312, 0x329b, 0x4624, 0x57ad, 0x6536, 0x74bf,
  0x8c48, 0x9dc1, 0xaf5a, 0xbed3, 0xca6c, 0xdbe5, 0xe97e, 0xf8f7,
  0x1081, 0x0108, 0x3393, 0x221a, 0x56a5, 0x472c, 0x75b7, 0x643e,
  0x9cc9, 0x8d40, 0xbfdb, 0xae52, 0xdaed, 0xcb64, 0xf9ff, 0xe876,
  0x2102, 0x308b, 0x0210, 0x1399, 0x6726, 0x76af, 0x4434, 0x55bd,
  0xad4a, 0xbcc3, 0x8e58, 0x9fd1, 0xeb6e, 0xfae7, 0xc87c, 0xd9f5,
  0x3183, 0x200a, 0x1291, 0x0318, 0x77a7, 0x662e, 0x54b5, 0x453c,
  0xbdcb, 0xac42, 0x9ed9, 0x8f50, 0xfbef, 0xea66, 0xd8fd, 0xc974,
  0x4204, 0x538d, 0x6116, 0x709f, 0x0420, 0x15a9, 0x2732, 0x36bb,
  0xce4c, 0xdfc5, 0xed5e, 0xfcd7, 0x8868, 0x99e1, 0xab7a, 0xbaf3,
  0x5285, 0x430c, 0x7197, 0x601e, 0x14a1, 0x0528, 0x37b3, 0x263a,
  0xdecd, 0xcf44, 0xfddf, 0xec56, 0x98e9, 0x8960, 0xbbfb, 0xaa72,
  0x6306, 0x728f, 0x4014, 0x519d, 0x2522, 0x34ab, 0x0630, 0x17b9,
  0xef4e, 0xfec7, 0xcc5c, 0xddd5, 0xa96a, 0xb8e3, 0x8a78, 0x9bf1,
  0x7387, 0x620e, 0x5095, 0x411c, 0x35a3, 0x242a, 0x16b1, 0x0738,
  0xffcf, 0xee46, 0xdcdd, 0xcd54, 0xb9eb, 0xa862, 0x9af9, 0x8b70,
  0x8408, 0x9581, 0xa71a, 0xb693, 0xc22c, 0xd3a5, 0xe13e, 0xf0b7,
  0x0840, 0x19c9, 0x2b52, 0x3adb, 0x4e64, 0x5fed, 0x6d76, 0x7cff,
  0x9489, 0x8500, 0xb79b, 0xa612, 0xd2ad, 0xc324, 0xf1bf, 0xe036,
  0x18c1, 0x0948, 0x3bd3, 0x2a5a, 0x5ee5, 0x4f6c, 0x7df7, 0x6c7e,
  0xa50a, 0xb483, 0x8618, 0x9791, 0xe32e, 0xf2a7, 0xc03c, 0xd1b5,
  0x2942, 0x38cb, 0x0a50, 0x1bd9, 0x6f66, 0x7eef, 0x4c74, 0x5dfd,
  0xb58b, 0xa402, 0x9699, 0x8710, 0xf3af, 0xe226, 0xd0bd, 0xc134,
  0x39c3, 0x284a, 0x1ad1, 0x0b58, 0x7fe7, 0x6e6e, 0x5cf5, 0x4d7c,
  0xc60c, 0xd785, 0xe51e, 0xf497, 0x8028, 0x91a1, 0xa33a, 0xb2b3,
  0x4a44, 0x5bcd, 0x6956, 0x78df, 0x0c60, 0x1de9, 0x2f72, 0x3efb,
  0xd68d, 0xc704, 0xf59f, 0xe416, 0x90a9, 0x8120, 0xb3bb, 0xa232,
  0x5ac5, 0x4b4c, 0x79d7, 0x685e, 0x1ce1, 0x0d68, 0x3ff3, 0x2e7a,
  0xe70e, 0xf687, 0xc41c, 0xd595, 0xa12a, 0xb0a3, 0x8238, 0x93b1,
  0x6b46, 0x7acf, 0x4854, 0x59dd, 0x2d62, 0x3ceb, 0x0e70, 0x1ff9,
  0xf78f, 0xe606, 0xd49d, 0xc514, 0xb1ab, 0xa022, 0x92b9, 0x8330,
  0x7bc7, 0x6a4e, 0x58d5, 0x495c, 0x3de3, 0x2c6a, 0x1ef1, 0x0f78
];

/**
 * Calculate CRC-16 for CS108 packet validation
 * Matches vendor algorithm: calculates over entire packet EXCEPT CRC bytes (6-7)
 * @param packet - Complete packet including header
 * @returns Calculated CRC value
 */
export function calculatePacketCRC(packet: Uint8Array): number {
  let crc: number = 0;
  const packetLength = packet[2] + 8; // Length from header + 8 byte header

  for (let i = 0; i < packetLength; i++) {
    // Skip CRC bytes at positions 6 and 7
    if (i !== 6 && i !== 7) {
      const index: number = (crc ^ packet[i]) & 0xff;
      crc = ((crc >> 8) ^ CRC_LOOKUP_TABLE[index]) & 0xffff;
    }
  }

  return crc;
}

/**
 * Validate packet CRC
 * @returns Object with valid flag, expected CRC, and actual calculated CRC
 */
export function validatePacketCRC(packet: Uint8Array): { valid: boolean; expected: number; actual: number } {
  const headerCRC = (packet[6] << 8) | packet[7]; // Big-endian

  // CRC of zero means skip validation (per CS108 spec)
  if (headerCRC === 0) {
    return { valid: true, expected: 0, actual: 0 };
  }

  const calculatedCRC = calculatePacketCRC(packet);
  return {
    valid: headerCRC === calculatedCRC,
    expected: headerCRC,
    actual: calculatedCRC
  };
}

/**
 * Validate packet length matches header declaration
 */
export function validatePacketLength(packet: Uint8Array): { valid: boolean; expected: number; actual: number } {
  const declaredLength = packet[2];
  const expectedTotal = declaredLength + 8; // Header is 8 bytes
  return {
    valid: packet.length === expectedTotal,
    expected: expectedTotal,
    actual: packet.length
  };
}