/**
 * CS108 Event Definitions - Unified Model
 * 
 * All CS108 commands and notifications using unified event model.
 * Extracted from protocol specifications and working implementation.
 * Uses named constants (Option B) to avoid magic number antipatterns.
 */

import type { CS108Event } from './type.js';
import { CS108_MODULES } from './type.js';
import type { ScalarPayload } from './payload-types.js';
import { parseUint8, parseBatteryPercentage } from './system/parser.js';
// parseInventoryTag removed - parsing handled by InventoryHandler
import { parseBarcodeData } from './barcode/parser.js';
// import { RFID_REGISTERS } from './rfid/constant.js'; // TODO: Uncomment when implementing register writes

// =============================================================================
// POWER MANAGEMENT COMMANDS
// =============================================================================

export const RFID_POWER_OFF: CS108Event = {
  name: 'RFID_POWER_OFF',
  eventCode: 0x8001,
  module: CS108_MODULES.RFID,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  timeout: 3000, // Power commands need extra time
  settlingDelay: 200, // Allow hardware to settle after power off
  description: 'Power off RFID module'
};

export const RFID_POWER_ON: CS108Event = {
  name: 'RFID_POWER_ON',
  eventCode: 0x8000,
  module: CS108_MODULES.RFID,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  timeout: 3000,
  settlingDelay: 500, // Allow hardware to fully initialize after power on
  description: 'Power on RFID module'
};

export const BARCODE_POWER_OFF: CS108Event = {
  name: 'BARCODE_POWER_OFF',
  eventCode: 0x9001,
  module: CS108_MODULES.BARCODE,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  timeout: 3000,
  settlingDelay: 200, // Allow hardware to settle after power off
  description: 'Power off barcode module'
};

export const BARCODE_POWER_ON: CS108Event = {
  name: 'BARCODE_POWER_ON',
  eventCode: 0x9000,
  module: CS108_MODULES.BARCODE,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  timeout: 3000,
  settlingDelay: 200, // Allow hardware to initialize after power on
  description: 'Power on barcode module'
};

// =============================================================================
// SYSTEM COMMANDS
// =============================================================================

export const GET_BATTERY_VOLTAGE: CS108Event<ScalarPayload> = {
  name: 'GET_BATTERY_VOLTAGE',
  eventCode: 0xA000,  // CSL overloaded this for autonomous notifications too
  module: CS108_MODULES.NOTIFICATION,
  isCommand: true,      // Primary use: command to get battery voltage
  isNotification: true,  // Also used for auto-notifications from START_BATTERY_REPORTING
  payloadLength: 0,
  responseLength: 2,
  parser: parseBatteryPercentage,
  description: 'Get battery percentage (0-100) from voltage reading'
};

export const GET_TRIGGER_STATE: CS108Event<ScalarPayload> = {
  name: 'GET_TRIGGER_STATE',
  eventCode: 0xA001,
  module: CS108_MODULES.NOTIFICATION,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: 1,
  parser: parseUint8,
  description: 'Get trigger state (0=released, 1=pressed)'
};

export const START_BATTERY_REPORTING: CS108Event = {
  name: 'START_BATTERY_REPORTING',
  eventCode: 0xA002,
  module: CS108_MODULES.NOTIFICATION,
  isCommand: true,
  isNotification: false,
  payloadLength: 1,
  payload: new Uint8Array([0x01]), // Enable reporting
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  description: 'Enable battery auto-reporting every 5 seconds'
};

export const START_TRIGGER_REPORTING: CS108Event = {
  name: 'START_TRIGGER_REPORTING', 
  eventCode: 0xA008, // From protocol.ts.old
  module: CS108_MODULES.NOTIFICATION,
  isCommand: true,
  isNotification: false,
  payloadLength: 1,
  payload: new Uint8Array([0x01]), // Enable reporting
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  description: 'Enable trigger state auto-reporting'
};

export const STOP_BATTERY_REPORTING: CS108Event = {
  name: 'STOP_BATTERY_REPORTING',
  eventCode: 0xA003,
  module: CS108_MODULES.NOTIFICATION,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  description: 'Stop battery auto-reporting'
};

export const STOP_TRIGGER_REPORTING: CS108Event = {
  name: 'STOP_TRIGGER_REPORTING',
  eventCode: 0xA009,
  module: CS108_MODULES.NOTIFICATION,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  description: 'Stop trigger state auto-reporting'
};

// =============================================================================
// BARCODE COMMANDS
// =============================================================================

export const BARCODE_TRIGGER_SCAN: CS108Event = {
  name: 'BARCODE_TRIGGER_SCAN',
  eventCode: 0x9002,
  module: CS108_MODULES.BARCODE,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  description: 'Trigger barcode scan'
};

export const BARCODE_SEND_COMMAND: CS108Event = {
  name: 'BARCODE_SEND_COMMAND',
  eventCode: 0x9003,
  module: CS108_MODULES.BARCODE,
  isCommand: true,
  isNotification: false,
  payloadLength: 50, // Max length 50 bytes (actual length is variable)
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  description: 'Send ESC command to Newland barcode module'
};

// Barcode ESC command constants
// NOTE: CS108 appears to have these inverted compared to Newland documentation
// Newland docs say 0x31 = trigger, 0x30 = stop
// But CS108 hardware responds to 0x30 = trigger, 0x31 = stop
export const BARCODE_ESC_TRIGGER = new Uint8Array([0x1b, 0x30]); // ESC + "0" - Trigger scan (CS108)
export const BARCODE_ESC_STOP = new Uint8Array([0x1b, 0x31]);    // ESC + "1" - Stop scan (CS108)
export const BARCODE_ESC_CONTINUOUS = new Uint8Array([0x1b, 0x33]); // ESC + "3" - Continuous reading

export const VIBRATOR_ON: CS108Event = {
  name: 'VIBRATOR_ON',
  eventCode: 0x9004,
  module: CS108_MODULES.BARCODE,
  isCommand: true,
  isNotification: false,
  payloadLength: 3,
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  description: 'Turn vibrator on with mode and duration'
};

export const VIBRATOR_OFF: CS108Event = {
  name: 'VIBRATOR_OFF',
  eventCode: 0x9005,
  module: CS108_MODULES.BARCODE,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  description: 'Turn vibrator off'
};

// =============================================================================
// IMPORTANT: Event Code 0x8002 Consolidation
// =============================================================================
//
// The CS108 hardware uses a single event code (0x8002) for ALL RFID firmware
// operations. Previous implementations incorrectly created separate event
// definitions for each operation (SET_RF_POWER, START_INVENTORY, etc.).
//
// Per CS108 API Specification:
// - Chapter 8.1: "0x8002 - RFID firmware command data. See Appendix A"
// - Appendix A: Documents the payload formats for different operations
//
// The payload determines the operation, not the event code:
// - Register writes: [0x70, access, register, value]
// - HST_CMD operations: Write specific values to register 0xF000
// - ABORT: Special format [0x40, 0x03, ...]
//
// Phase 2 will implement createFirmwareCommand() to build these payloads.
// =============================================================================

// =============================================================================
// RFID FIRMWARE COMMAND (All RFID operations use this single event)
// =============================================================================

/**
 * RFID Firmware Command - ALL RFID firmware operations use this event
 *
 * Per CS108 API Spec Chapter 8.1: "0x8002 - RFID firmware command data. See Appendix A"
 *
 * The payload determines the actual operation:
 * - Register writes: [0x70, access, reg_lsb, reg_msb, value[4]]
 * - ABORT command: [0x40, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]
 * - Other firmware commands per Appendix A
 *
 * Phase 2 will create a firmware command builder to generate these payloads
 */
export const RFID_FIRMWARE_COMMAND: CS108Event = {
  name: 'RFID_FIRMWARE_COMMAND',
  eventCode: 0x8002,
  module: CS108_MODULES.RFID,
  isCommand: true,
  isNotification: false,
  payloadLength: -1,  // Variable length depending on command
  responseLength: 1,   // Status byte: 0x00 = success, 0xFF = failure
  successByte: 0x00,
  parser: parseUint8,
  timeout: 5000,       // May need adjustment based on command type
  settlingDelay: 100,
  description: 'RFID firmware command data (register ops, inventory control, etc.)'
};

// =============================================================================
// NOTIFICATIONS (Autonomous events from hardware)
// =============================================================================

// Note: BATTERY_NOTIFICATION removed - use BATTERY_VOLTAGE for both command and notification
// since they share the same event code (0xA000);

export const TRIGGER_PRESSED_NOTIFICATION: CS108Event = {
  name: 'TRIGGER_PRESSED',
  eventCode: 0xA102,
  module: CS108_MODULES.NOTIFICATION,
  isCommand: false,
  isNotification: true,
  parser: undefined, // No payload
  description: 'Trigger button pressed notification'
};

export const TRIGGER_RELEASED_NOTIFICATION: CS108Event = {
  name: 'TRIGGER_RELEASED', 
  eventCode: 0xA103,
  module: CS108_MODULES.NOTIFICATION,
  isCommand: false,
  isNotification: true,
  parser: undefined, // No payload
  description: 'Trigger button released notification'
};

export const INVENTORY_TAG_NOTIFICATION: CS108Event = {
  name: 'INVENTORY_TAG',
  eventCode: 0x8100,
  module: CS108_MODULES.RFID,
  isCommand: false,
  isNotification: true,
  parser: undefined, // Parser is handled by InventoryHandler which properly handles multi-tag packets
  description: 'RFID tag inventory notification'
};

export const BARCODE_DATA_NOTIFICATION: CS108Event = {
  name: 'BARCODE_DATA',
  eventCode: 0x9100,
  module: CS108_MODULES.BARCODE,
  isCommand: false,
  isNotification: true,
  parser: parseBarcodeData,
  description: 'Barcode read/reply data notification'
};

export const BARCODE_GOOD_READ_NOTIFICATION: CS108Event = {
  name: 'BARCODE_GOOD_READ',
  eventCode: 0x9101,
  module: CS108_MODULES.BARCODE,
  isCommand: false,
  isNotification: true,
  parser: undefined, // No payload for good read confirmation
  description: 'Barcode good read confirmation notification'
};

export const ERROR_NOTIFICATION: CS108Event = {
  name: 'ERROR_NOTIFICATION',
  eventCode: 0xA101,
  module: CS108_MODULES.NOTIFICATION,
  isCommand: true,      // Can be a command response when error occurs
  isNotification: true,  // Also an autonomous notification
  parser: parseUint8,    // 2-byte error code
  description: 'System error notification (also command error response)'
};

// =============================================================================
// EVENT LOOKUP MAP (Single source of truth)
// =============================================================================

export const CS108_EVENT_MAP = new Map<number, CS108Event>([
  // Power Management
  [0x8001, RFID_POWER_OFF],
  [0x8000, RFID_POWER_ON],
  [0x9001, BARCODE_POWER_OFF],
  [0x9000, BARCODE_POWER_ON],
  
  // System Commands
  [0xA000, GET_BATTERY_VOLTAGE],
  [0xA001, GET_TRIGGER_STATE],
  [0xA002, START_BATTERY_REPORTING],
  [0xA003, STOP_BATTERY_REPORTING],
  [0xA008, START_TRIGGER_REPORTING],
  [0xA009, STOP_TRIGGER_REPORTING],
  
  // Barcode Commands
  [0x9002, BARCODE_TRIGGER_SCAN],
  [0x9003, BARCODE_SEND_COMMAND],
  [0x9004, VIBRATOR_ON],
  [0x9005, VIBRATOR_OFF],
  
  // RFID Configuration Commands (Note: All use 0x8002 with different payloads)
  // Individual mapping handled by command manager based on context
  
  // RFID Firmware Commands (all use 0x8002)
  [0x8002, RFID_FIRMWARE_COMMAND],
  // Note: 0x4003 is NOT an event code - it's the ABORT payload for 0x8002
  
  // Notifications
  // 0xA000 mapped to BATTERY_VOLTAGE (handles both command and notification)
  [0xA101, ERROR_NOTIFICATION],
  [0xA102, TRIGGER_PRESSED_NOTIFICATION],
  [0xA103, TRIGGER_RELEASED_NOTIFICATION],
  [0x8100, INVENTORY_TAG_NOTIFICATION],
  [0x9100, BARCODE_DATA_NOTIFICATION],
  [0x9101, BARCODE_GOOD_READ_NOTIFICATION]
]);

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

/**
 * Get event definition by event code
 */
export function getEventByCode(eventCode: number): CS108Event | undefined {
  return CS108_EVENT_MAP.get(eventCode);
}

/**
 * Check if event code represents a notification
 */
export function isNotification(eventCode: number): boolean {
  const event = CS108_EVENT_MAP.get(eventCode);
  return event?.isNotification === true;
}