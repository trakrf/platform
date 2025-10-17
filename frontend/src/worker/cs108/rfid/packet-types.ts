/**
 * RFID Packet Type Definitions
 *
 * CS108 RFID packets have a multi-layer structure:
 * 1. CS108 Transport Layer (0-9): Standard CS108 header + event code
 * 2. Inventory Packet Layer (10+): Contains packet version, type, and tag data
 * 3. Tag Data Layer: PC word + EPC + RSSI in various formats
 *
 * This file defines the intermediate types for parsing these layers.
 */

// ============================================================================
// Packet Version Constants (byte 10 in the full packet)
// ============================================================================

export enum PacketVersion {
  INVENTORY_CYCLE_END = 0x00,  // End of inventory cycle marker
  CMD_BEGIN_END_1 = 0x01,       // Command begin/end format 1
  CMD_BEGIN_END_2 = 0x02,       // Command begin/end format 2
  NORMAL = 0x03,                // Normal mode format (full metadata)
  COMPACT = 0x04,               // Compact format (minimal metadata)
  ABORT_RESPONSE = 0x40,        // Abort command response
  REG_ACCESS = 0x70,            // Register access response
}

// ============================================================================
// Packet Type Constants (bytes 12-13 in the full packet, little-endian)
// ============================================================================

export enum PacketType {
  CMD_BEGIN = 0x0000,       // Command begin
  CMD_BEGIN_ALT = 0x8000,   // Command begin (alt)
  CMD_END = 0x0001,         // Command end
  CMD_END_ALT = 0x8001,     // Command end (alt)
  INVENTORY = 0x0005,       // Inventory data
  INVENTORY_ALT = 0x8005,   // Inventory data (alt)
  INV_CYCLE = 0x0007,       // Inventory cycle
  INV_CYCLE_ALT = 0x8007,   // Inventory cycle (alt)
  LOCATE = 0x0580,          // Locate response
}

// ============================================================================
// Inventory Packet Structure (starts at byte 10 of CS108 packet)
// ============================================================================

/**
 * Base inventory packet header
 * This is what comes after the CS108 transport header
 */
export interface InventoryPacketHeader {
  version: PacketVersion;      // Byte 0 (offset 10 in full packet)
  flags: number;                // Byte 1 (offset 11)
  packetType: PacketType;       // Bytes 2-3 (offset 12-13, little-endian)
  packetLength: number;         // Bytes 4-5 (offset 14-15, little-endian)
}

/**
 * Compact mode inventory packet (version 0x04)
 * Minimal metadata, multiple tags can be in one packet
 */
export interface CompactInventoryPacket extends InventoryPacketHeader {
  version: PacketVersion.COMPACT;
  antenna: number;              // Byte 6 (offset 16)
  tagData: CompactTagData[];    // Starting at byte 8 (offset 18)
}

/**
 * Normal mode inventory packet (version 0x03)
 * Full metadata including phase, channel, timestamps
 */
export interface NormalInventoryPacket extends InventoryPacketHeader {
  version: PacketVersion.NORMAL;
  msCtr: number;                // Bytes 8-11 (offset 18-21, millisecond counter)
  wbRssi: number;               // Byte 12 (offset 22, wideband RSSI)
  nbRssi: number;               // Byte 13 (offset 23, narrowband RSSI)
  phase: number;                // Byte 14 (offset 24)
  chIdx: number;                // Byte 15 (offset 25, channel index)
  antenna: number;              // Byte 18 (offset 28)
  tagData: NormalTagData[];     // Starting at byte 30 (offset 40)
}

// ============================================================================
// Tag Data Structures
// ============================================================================

/**
 * PC (Protocol Control) word from EPC Gen2 standard
 * This 16-bit word contains tag configuration information
 */
export interface PCWord {
  raw: number;                  // Full 16-bit value
  epcLengthWords: number;        // Bits 11-15: EPC length in 16-bit words
  epcLengthBytes: number;        // Calculated: epcLengthWords * 2
  umi: boolean;                  // Bit 10: User memory indicator
  xi: boolean;                   // Bit 9: XPC indicator
  toggleBit: boolean;            // Bit 8: Toggle bit
  attributeBits: number;         // Bits 0-7: Application specific
}

/**
 * Compact mode tag data
 * PC + EPC + RSSI only
 */
export interface CompactTagData {
  pc: PCWord;                    // 2 bytes: Protocol Control word
  epc: Uint8Array;               // Variable length based on PC word
  epcHex: string;                // EPC as hex string
  rssi: number;                  // 1 byte: RSSI - 128 = dBm
}

/**
 * Normal mode tag data
 * Same as compact but with additional metadata from packet header
 */
export interface NormalTagData extends CompactTagData {
  // Additional fields from the packet header
  msCtr?: number;                // Timestamp from packet
  phase?: number;                // Phase angle for locate
  channelIndex?: number;         // RF channel used
}

// ============================================================================
// Command Packets
// ============================================================================

/**
 * Command begin packet (version 0x01/0x02, type 0x0000/0x8000)
 */
export interface CommandBeginPacket extends InventoryPacketHeader {
  version: PacketVersion.CMD_BEGIN_END_1 | PacketVersion.CMD_BEGIN_END_2;
  packetType: PacketType.CMD_BEGIN | PacketType.CMD_BEGIN_ALT;
  command: number;               // Command being executed
  msCtr: number;                 // Millisecond counter at start
}

/**
 * Command end packet (version 0x01/0x02, type 0x0001/0x8001)
 */
export interface CommandEndPacket extends InventoryPacketHeader {
  version: PacketVersion.CMD_BEGIN_END_1 | PacketVersion.CMD_BEGIN_END_2;
  packetType: PacketType.CMD_END | PacketType.CMD_END_ALT;
  msCtr: number;                 // Millisecond counter at end
  status: CommandEndStatus;      // Command completion status
  errorPort: number;             // Antenna port if error
}

export enum CommandEndStatus {
  SUCCESS = 0,
  INVENTORY_DONE = 1,
  TIMEOUT = 2,
  ABORTED = 3,
  NO_TAGS = 4,
  UNKNOWN = 0xFF,
}

/**
 * Inventory cycle packet (version varies, type 0x0007/0x8007)
 */
export interface InventoryCyclePacket extends InventoryPacketHeader {
  packetType: PacketType.INV_CYCLE | PacketType.INV_CYCLE_ALT;
  msCtr: number;                 // Millisecond counter
  antenna: number;               // Current antenna
  cycleData?: number;            // Optional cycle-specific data
}

// ============================================================================
// Register Access Packets
// ============================================================================

/**
 * Register access response (version 0x70)
 */
export interface RegisterAccessPacket extends InventoryPacketHeader {
  version: PacketVersion.REG_ACCESS;
  registerAddress: number;       // 16-bit register address
  registerValue: number;         // 32-bit register value
}

// ============================================================================
// Union Types
// ============================================================================

/**
 * All possible inventory packet types
 */
export type InventoryPacket =
  | CompactInventoryPacket
  | NormalInventoryPacket
  | CommandBeginPacket
  | CommandEndPacket
  | InventoryCyclePacket
  | RegisterAccessPacket;

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Parse PC word from 2 bytes
 */
export function parsePCWord(high: number, low: number): PCWord {
  const raw = (high << 8) | low;
  const epcLengthWords = (raw >> 11) & 0x1F;

  return {
    raw,
    epcLengthWords,
    epcLengthBytes: epcLengthWords * 2,
    umi: !!(raw & 0x0400),
    xi: !!(raw & 0x0200),
    toggleBit: !!(raw & 0x0100),
    attributeBits: raw & 0xFF,
  };
}

/**
 * Type guard for compact inventory packet
 */
export function isCompactInventoryPacket(packet: InventoryPacket): packet is CompactInventoryPacket {
  return packet.version === PacketVersion.COMPACT;
}

/**
 * Type guard for normal inventory packet
 */
export function isNormalInventoryPacket(packet: InventoryPacket): packet is NormalInventoryPacket {
  return packet.version === PacketVersion.NORMAL;
}

/**
 * Type guard for command begin packet
 */
export function isCommandBeginPacket(packet: InventoryPacket): packet is CommandBeginPacket {
  return (packet.version === PacketVersion.CMD_BEGIN_END_1 ||
          packet.version === PacketVersion.CMD_BEGIN_END_2) &&
         (packet.packetType === PacketType.CMD_BEGIN ||
          packet.packetType === PacketType.CMD_BEGIN_ALT);
}

/**
 * Type guard for command end packet
 */
export function isCommandEndPacket(packet: InventoryPacket): packet is CommandEndPacket {
  return (packet.version === PacketVersion.CMD_BEGIN_END_1 ||
          packet.version === PacketVersion.CMD_BEGIN_END_2) &&
         (packet.packetType === PacketType.CMD_END ||
          packet.packetType === PacketType.CMD_END_ALT);
}