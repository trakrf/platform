/**
 * CS108 Test Configuration and Utilities
 * 
 * CS108-specific test commands, validation, and debug utilities.
 * Can be used for both bridge-based and direct hardware connections.
 * 
 * Command: GET_TRIGGER_STATE (0xA001)
 * - Deterministic response: 0x00 (released) or 0x01 (pressed)  
 * - Reliable for TDD integration testing
 * - Unlike battery voltage which varies by charge level
 */

import { bytesToHex } from './utils.config';
import { buildNotification } from './cs108-packet-builder';
import { TRIGGER_PRESSED_NOTIFICATION, TRIGGER_RELEASED_NOTIFICATION } from '@/worker/cs108/event';

// Bootstrap packets - kept as raw bytes for minimal dependencies during connectivity testing
// Test command - GET_TRIGGER_STATE (works with CS108, but this is just testing connectivity)
export const cs108TestCommand = new Uint8Array([0xA7, 0xB3, 0x02, 0xD9, 0x82, 0x37, 0x00, 0x00, 0xA0, 0x01]);
export const cs108TestResponse = new Uint8Array([0xA7, 0xB3, 0x03, 0xD9, 0x82, 0x9E, 0x74, 0x37, 0xA0, 0x01, 0x00]);

// CS108 trigger notification packets - using structured approach with CS108Event
export const cs108TriggerPressPacket = buildNotification(TRIGGER_PRESSED_NOTIFICATION);
export const cs108TriggerReleasePacket = buildNotification(TRIGGER_RELEASED_NOTIFICATION);

/**
 * Validate GET_TRIGGER_STATE response structure
 * @param response Response bytes from device
 * @returns True if response matches expected structure
 */
export function isValidTriggerStateResponse(response: Uint8Array): boolean {
  if (response.length !== 11) {
    console.error(`[CS108] Invalid response length: expected 11, got ${response.length}`);
    return false;
  }

  // Check command echo (bytes 8-9 should be 0xA0, 0x01)
  if (response[8] !== 0xA0 || response[9] !== 0x01) {
    console.error(`[CS108] Invalid command echo: expected [0xA0, 0x01], got [0x${response[8].toString(16)}, 0x${response[9].toString(16)}]`);
    return false;
  }

  // Check trigger state (byte 10 should be 0x00 or 0x01)
  const triggerState = response[10];
  if (triggerState !== 0x00 && triggerState !== 0x01) {
    console.error(`[CS108] Invalid trigger state: expected 0x00 or 0x01, got 0x${triggerState.toString(16)}`);
    return false;
  }

  console.log(`[CS108] Valid trigger state response: ${triggerState === 0x00 ? 'released' : 'pressed'}`);
  return true;
}

/**
 * Get human-readable trigger state from response
 * @param response Response bytes from device
 * @returns 'released', 'pressed', or 'invalid'
 */
export function getTriggerState(response: Uint8Array): 'released' | 'pressed' | 'invalid' {
  if (!isValidTriggerStateResponse(response)) {
    return 'invalid';
  }
  return response[10] === 0x00 ? 'released' : 'pressed';
}

/**
 * Log CS108 command/response pair with human-readable format
 * @param command Command bytes sent to device
 * @param response Response bytes from device
 */
export function logCommandResponse(command: Uint8Array, response: Uint8Array): void {
  console.log(`[CS108] Command:  ${bytesToHex(command)}`);
  console.log(`[CS108] Response: ${bytesToHex(response)}`);
  console.log(`[CS108] Trigger:  ${getTriggerState(response)}`);
}