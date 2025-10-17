/**
 * Tests for CS108 packet builder
 */

import { describe, it, expect } from 'vitest';
import {
  buildNotification,
  buildResponse,
  TestPackets,
  formatPacket,
  packetsEqual
} from './cs108-packet-builder';
import { parsePacket } from '@/worker/cs108/protocol';
import {
  TRIGGER_PRESSED_NOTIFICATION,
  TRIGGER_RELEASED_NOTIFICATION,
  RFID_POWER_OFF,
  GET_BATTERY_VOLTAGE
} from '@/worker/cs108/event';

describe('CS108 Packet Builder', () => {
  describe('buildNotification', () => {
    it('should build trigger pressed notification correctly', () => {
      const packet = buildNotification(TRIGGER_PRESSED_NOTIFICATION);
      
      // Parse the packet we built
      const parsed = parsePacket(packet);
      
      expect(parsed).not.toBeNull();
      expect(parsed?.event).toBe(TRIGGER_PRESSED_NOTIFICATION);
      expect(parsed?.direction).toBe(0x9E); // Uplink
      expect(parsed?.rawPayload.length).toBe(0); // No payload
    });
    
    it('should build trigger released notification correctly', () => {
      const packet = buildNotification(TRIGGER_RELEASED_NOTIFICATION);
      
      const parsed = parsePacket(packet);
      
      expect(parsed).not.toBeNull();
      expect(parsed?.event).toBe(TRIGGER_RELEASED_NOTIFICATION);
      expect(parsed?.direction).toBe(0x9E);
      expect(parsed?.rawPayload.length).toBe(0);
    });
    
    it('should build battery notification with payload', () => {
      const millivolts = 3700; // 3.7V
      const payload = new Uint8Array([
        millivolts & 0xFF,
        (millivolts >> 8) & 0xFF
      ]);

      const packet = buildNotification(GET_BATTERY_VOLTAGE, payload);
      const parsed = parsePacket(packet);

      expect(parsed).not.toBeNull();
      expect(parsed?.event).toBe(GET_BATTERY_VOLTAGE);
      // parseBatteryPercentage returns just a percentage number
      expect(parsed?.payload).toBeGreaterThan(0);
      expect(parsed?.payload).toBeLessThanOrEqual(100);
    });
  });
  
  describe('buildResponse', () => {
    it('should build successful command response', () => {
      const packet = buildResponse(RFID_POWER_OFF, { success: true });
      const parsed = parsePacket(packet);
      
      expect(parsed).not.toBeNull();
      expect(parsed?.event).toBe(RFID_POWER_OFF);
      expect(parsed?.direction).toBe(0x9E); // Uplink
      expect(parsed?.payload).toBe(0x00); // parseUint8 returns the success byte
    });
    
    it('should build failed command response', () => {
      const packet = buildResponse(RFID_POWER_OFF, { success: false });
      const parsed = parsePacket(packet);
      
      expect(parsed).not.toBeNull();
      expect(parsed?.payload).toBe(0xFF); // parseUint8 returns the failure byte
    });
  });
  
  describe('TestPackets helpers', () => {
    it('should build trigger packets using helper methods', () => {
      const press = TestPackets.triggerPress();
      const release = TestPackets.triggerRelease();
      
      const parsedPress = parsePacket(press);
      const parsedRelease = parsePacket(release);
      
      expect(parsedPress?.event).toBe(TRIGGER_PRESSED_NOTIFICATION);
      expect(parsedRelease?.event).toBe(TRIGGER_RELEASED_NOTIFICATION);
    });
    
    it('should build battery voltage packet', () => {
      const packet = TestPackets.batteryVoltage(4200); // 4.2V
      const parsed = parsePacket(packet);

      expect(parsed).not.toBeNull();
      expect(parsed?.event).toBe(GET_BATTERY_VOLTAGE);

      // parseBatteryPercentage returns just a percentage number
      expect(parsed?.payload).toBeDefined();
      expect(parsed?.payload).toBeGreaterThan(90); // 4200mV should be nearly 100%
      expect(parsed?.payload).toBeLessThanOrEqual(100);
    });
    
    it('should build inventory tag packet', () => {
      const packet = TestPackets.inventoryTag('E28011700000020E5C02C7D9', -45);
      const parsed = parsePacket(packet);
      
      expect(parsed).not.toBeNull();
      expect(parsed?.event.eventCode).toBe(0x8100); // INVENTORY_TAG_NOTIFICATION
      expect(parsed?.rawPayload.length).toBeGreaterThan(0); // Has EPC payload
    });
  });
  
  describe('formatPacket helper', () => {
    it('should format packet as readable hex string', () => {
      const packet = new Uint8Array([0xA7, 0xB3, 0x02]);
      const formatted = formatPacket(packet);
      
      expect(formatted).toBe('0xA7 0xB3 0x02');
    });
  });
  
  describe('packetsEqual helper', () => {
    it('should correctly compare packets', () => {
      const a = new Uint8Array([0xA7, 0xB3]);
      const b = new Uint8Array([0xA7, 0xB3]);
      const c = new Uint8Array([0xA7, 0xB4]);
      
      expect(packetsEqual(a, b)).toBe(true);
      expect(packetsEqual(a, c)).toBe(false);
    });
  });
});