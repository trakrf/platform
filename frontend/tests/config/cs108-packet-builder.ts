/**
 * CS108 Packet Builder for Tests
 * 
 * Uses PacketHandler from the worker implementation for ALL packet building.
 * No duplicate byte-twiddling - just test-specific helpers and inspection utilities.
 */

import type { CS108Event } from '@/worker/cs108/type';
import { PacketHandler } from '@/worker/cs108/packet';
import {
  TRIGGER_PRESSED_NOTIFICATION,
  TRIGGER_RELEASED_NOTIFICATION,
  GET_BATTERY_VOLTAGE,
  INVENTORY_TAG_NOTIFICATION,
  RFID_POWER_ON,
  RFID_POWER_OFF
} from '@/worker/cs108/event';

// Create a packet handler instance for building packets
const packetHandler = new PacketHandler();

/**
 * Build a command packet using the real packet builder
 */
export function buildCommand(event: CS108Event, payload?: Uint8Array): Uint8Array {
  return packetHandler.buildCommand(event, payload);
}

/**
 * Build a response packet (for testing)
 * Responses come FROM the device, so we need to simulate them
 */
export function buildResponse(event: CS108Event, options?: {
  success?: boolean;
  payload?: Uint8Array;
}): Uint8Array {
  const success = options?.success ?? true;
  const basePayload = options?.payload ?? new Uint8Array(0);
  
  // For commands with successByte, prepend it
  let payload = basePayload;
  if (event.successByte !== undefined) {
    payload = new Uint8Array(1 + basePayload.length);
    payload[0] = success ? event.successByte : 0xFF;
    if (basePayload.length > 0) {
      payload.set(basePayload, 1);
    }
  }
  
  // Use PacketHandler's new buildResponse method
  return packetHandler.buildResponse(event, payload);
}

/**
 * Build a notification packet (uplink from device)
 */
export function buildNotification(event: CS108Event, payload?: Uint8Array): Uint8Array {
  // Use PacketHandler's new buildNotification method
  return packetHandler.buildNotification(event, payload);
}

/**
 * Fragment a packet for BLE testing (20-byte MTU)
 */
export function fragmentPacket(packet: Uint8Array, mtu: number = 20): Uint8Array[] {
  const fragments: Uint8Array[] = [];
  
  for (let i = 0; i < packet.length; i += mtu) {
    const end = Math.min(i + mtu, packet.length);
    fragments.push(packet.slice(i, end));
  }
  
  return fragments;
}

/**
 * Build common test packets - USE CASE 3: Generate test events
 */
export class TestPackets {
  /**
   * Build trigger press notification
   */
  static triggerPress(): Uint8Array {
    return buildNotification(TRIGGER_PRESSED_NOTIFICATION);
  }
  
  /**
   * Build trigger release notification  
   */
  static triggerRelease(): Uint8Array {
    return buildNotification(TRIGGER_RELEASED_NOTIFICATION);
  }
  
  /**
   * Build battery voltage notification
   */
  static batteryVoltage(millivolts: number): Uint8Array {
    const payload = new Uint8Array(2);
    payload[0] = millivolts & 0xFF;
    payload[1] = (millivolts >> 8) & 0xFF;
    return buildNotification(GET_BATTERY_VOLTAGE, payload);
  }
  
  /**
   * Build inventory tag notification
   */
  static inventoryTag(epc: string, rssi: number = -45): Uint8Array {
    // Build a simplified tag payload
    // Format: [flags(2), pc(2), epc(variable), rssi(1)]
    const epcBytes = hexToBytes(epc);
    const payload = new Uint8Array(5 + epcBytes.length);
    
    // Simplified flags
    payload[0] = 0x00;
    payload[1] = 0x00;
    
    // PC (Protocol Control) - indicates EPC length
    const pc = (epcBytes.length / 2) << 11; // EPC word count in bits 15-11
    payload[2] = pc & 0xFF;
    payload[3] = (pc >> 8) & 0xFF;
    
    // EPC
    payload.set(epcBytes, 4);
    
    // RSSI (simplified as positive value)
    payload[4 + epcBytes.length] = Math.abs(rssi);
    
    return buildNotification(INVENTORY_TAG_NOTIFICATION, payload);
  }
  
  /**
   * Build power on response
   */
  static powerOnResponse(success: boolean = true): Uint8Array {
    return buildResponse(RFID_POWER_ON, { success });
  }
  
  /**
   * Build power off response
   */
  static powerOffResponse(success: boolean = true): Uint8Array {
    return buildResponse(RFID_POWER_OFF, { success });
  }
}

/**
 * Packet Inspector - USE CASE 1 & 2: Inspect commands and responses
 */
export class PacketInspector {
  /**
   * Extract event code from a packet (little-endian)
   */
  static getEventCode(packet: Uint8Array): number | null {
    if (packet.length < 10) return null;
    return packet[8] | (packet[9] << 8);
  }
  
  /**
   * Check if packet is a specific event
   */
  static isEvent(packet: Uint8Array, event: CS108Event): boolean {
    const eventCode = this.getEventCode(packet);
    return eventCode === event.eventCode;
  }
  
  /**
   * Check if packet is a command (downlink)
   */
  static isCommand(packet: Uint8Array): boolean {
    return packet.length >= 6 && packet[5] === 0x37;
  }
  
  /**
   * Check if packet is a response/notification (uplink)
   */
  static isResponse(packet: Uint8Array): boolean {
    return packet.length >= 6 && packet[5] === 0x9E;
  }
  
  /**
   * Get packet direction
   */
  static getDirection(packet: Uint8Array): 'downlink' | 'uplink' | null {
    if (packet.length < 6) return null;
    return packet[5] === 0x37 ? 'downlink' : packet[5] === 0x9E ? 'uplink' : null;
  }
  
  /**
   * Get raw payload from packet
   */
  static getRawPayload(packet: Uint8Array): Uint8Array {
    if (packet.length <= 10) return new Uint8Array(0);
    return packet.slice(10);
  }
  
  /**
   * Check if response indicates success (for commands with success byte)
   */
  static isSuccessResponse(packet: Uint8Array, event: CS108Event): boolean {
    if (event.successByte === undefined) return true;
    const rawPayload = this.getRawPayload(packet);
    return rawPayload.length > 0 && rawPayload[0] === event.successByte;
  }
  
  /**
   * Debug helper - describe packet in human-readable form
   */
  static describe(packet: Uint8Array): string {
    if (packet.length < 10) return 'Invalid packet (too short)';
    
    const eventCode = this.getEventCode(packet);
    const direction = this.getDirection(packet);
    const payloadLen = packet.length - 10;
    
    return `${direction} 0x${eventCode?.toString(16).padStart(4, '0')} (${payloadLen} byte payload)`;
  }
}

/**
 * Helper to convert hex string to bytes
 */
function hexToBytes(hex: string): Uint8Array {
  const clean = hex.replace(/\s/g, '').replace(/^0x/i, '');
  const bytes = new Uint8Array(clean.length / 2);
  
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(clean.substr(i * 2, 2), 16);
  }
  
  return bytes;
}

/**
 * Helper to compare packets (for assertions)
 */
export function packetsEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false;
  }
  
  return true;
}

/**
 * Helper to format packet as hex string (for debugging)
 */
export function formatPacket(packet: Uint8Array): string {
  return Array.from(packet)
    .map(b => '0x' + b.toString(16).toUpperCase().padStart(2, '0'))
    .join(' ');
}