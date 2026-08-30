/**
 * CS108 Type Definitions - Unified Event Model
 *
 * Unified approach combining commands and notifications under a single
 * CS108Event interface. Uses named constants (Option B) to avoid magic
 * numbers and provides strong typing throughout.
 */

// Import payload types for CS108Event generic
import type {
  CS108PayloadType,
  PayloadParser
} from './payload-types.js';
import type { ReaderStateType } from '../types/reader.js';

/**
 * CS108 Event Definition
 * Unified interface for both commands and notifications
 * Generic T parameter specifies the parsed payload type
 */
export interface CS108Event<T extends CS108PayloadType = CS108PayloadType> {
  // Identity
  readonly name: string;           // Human-readable for logging
  readonly eventCode: number;      // Event code (same for command and response)
  readonly module: number;         // CS108 module (0xC2 for RFID, etc.)

  // Type flags - some events can be both (e.g., 0xA000 battery)
  readonly isCommand: boolean;     // Can be sent as a command
  readonly isNotification: boolean; // Can be received as autonomous notification

  // Request (commands only)
  readonly payloadLength?: number;     // Expected payload size
  readonly payload?: Uint8Array;       // Default payload
  readonly timeout?: number;           // Command timeout (ms)
  readonly settlingDelay?: number;     // Post-success delay (ms)

  // Response (commands only)
  readonly responseLength?: number;    // Expected response size
  readonly successByte?: number;       // Success indicator (usually 0x00)

  // Parser (both commands and notifications)
  readonly parser?: PayloadParser<T>;  // Type-safe parser function

  // Metadata
  readonly description?: string;
}

/**
 * CS108 Module Constants
 * From CS108 protocol specification
 */
export const CS108_MODULES = {
  RFID: 0xC2,
  BARCODE: 0x6A,
  NOTIFICATION: 0xD9,
  BLUETOOTH: 0x5F,
  SILICON_LAB: 0xE8
} as const;

/**
 * CS108 Packet Type
 * Complete parsed packet with header, event, and payload
 */
export interface CS108Packet {
  // Header fields (bytes 0-7)
  prefix: number;      // Byte 0: Always 0xA7
  transport: number;   // Byte 1: 0xB3 (BT) or 0xE6 (USB)
  length: number;      // Byte 2: Payload length (1-120)
  module: number;      // Byte 3: Module identifier
  reserve: number;     // Byte 4: Always 0x82
  direction: number;   // Byte 5: 0x37 (down) or 0x9E (up)
  crc: number;         // Bytes 6-7: CRC-16 (little-endian)

  // Event identification
  eventCode: number;   // Bytes 8-9: Little-endian event identifier
  event: CS108Event;   // REQUIRED: Typed event definition (fails if unknown)

  // Payload (bytes 10+)
  rawPayload: Uint8Array;      // Raw bytes from packet
  payload?: CS108PayloadType;  // Typed parsed payload (when event.parser exists)

  // Computed fields
  totalExpected: number; // 8 + length (for fragmentation)
  isComplete: boolean;   // true when all fragments received
}

/**
 * Single command in a sequence
 */
export interface SequenceCommand {
  event: CS108Event;
  payload?: Uint8Array;
  delay?: number;      // Optional delay after this command (ms)
  retryOnError?: boolean; // Whether to retry this command once if it fails (default: false)
  finalState?: ReaderStateType;  // State to transition to after successful sequence completion

  /**
   * How long the device cannot accept ANOTHER command after this one is sent
   * (ms). Held by CommandManager, measured from the send.
   *
   * Not `delay`. `delay` blocks the sequence, and therefore its caller, which
   * is the wrong shape for a constraint that is about the DEVICE's readiness
   * rather than about anyone waiting: the caller is told the command landed and
   * gets on with its life, while the next dispatch — from any caller, in any
   * sequence — is the one that pays.
   *
   * It lives on the sequence entry rather than on CS108Event because the
   * constraint belongs to a PAYLOAD: the RFID ABORT is one of many things sent
   * under RFID_FIRMWARE_COMMAND (0x8002), and the alternative is CommandManager
   * reaching into payload bytes to recognise it. See TRA-1185.
   */
  quietPeriodAfter?: number;

  /**
   * Dispatch this command even if a quiet window declared by an earlier command
   * has not expired.
   *
   * A per-command claim that THIS command is safe to issue inside THAT window —
   * not a general opt-out. The window's deadline is left intact for anything
   * else queued behind it.
   *
   * The case it exists for: a trigger cycle within one scanning mode sends
   * ABORT then START_INVENTORY, with no power cycling and no reconfiguration in
   * between. Gating the restart on the post-ABORT window costs ~2 s per cycle,
   * measured on hardware, and that stall is what makes an operator cycle the
   * trigger harder — which is how trigger edges get lost. See TRA-1197 and
   * ADR 0011.
   *
   * ⚠ Adding this to a command is a claim about the DEVICE, and the vendor note
   * says "another command" without qualification. Do not spread it to a command
   * whose behaviour inside the window has not been observed on a reader.
   */
  ignoresQuietPeriod?: boolean;
}

/**
 * A sequence of commands to execute in order
 */
export type CommandSequence = SequenceCommand[];

