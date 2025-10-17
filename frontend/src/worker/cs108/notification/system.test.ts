/**
 * Integration tests for the notification system
 */

import { describe, it, expect, beforeEach, vi, afterEach, type Mock } from 'vitest';
import { NotificationManager } from './manager';
import type { CS108Packet, CS108Event } from '../type';
import { ReaderMode, ReaderState } from '../../types/reader';

describe('Notification System Integration', () => {
  let manager: NotificationManager;
  let router: ReturnType<NotificationManager['getRouter']>;
  let postMessageSpy: Mock;
  let currentMode: ReaderMode = ReaderMode.IDLE;
  let currentState: ReaderState = ReaderState.CONNECTED;

  beforeEach(() => {
    vi.useFakeTimers();
    postMessageSpy = vi.fn();
    globalThis.postMessage = postMessageSpy;
    currentMode = ReaderMode.IDLE;
    currentState = ReaderState.CONNECTED;

    manager = new NotificationManager(
      (event) => {
        // Mock callback that actually posts the message
        postMessageSpy({
          ...event,
          timestamp: Date.now()
        });
      },
      {
        debug: false,
        getCurrentMode: () => currentMode,
        getReaderState: () => currentState,
      }
    );
    router = manager.getRouter();
  });

  afterEach(() => {
    vi.useRealTimers();
    manager.cleanup();
  });

  describe('battery notifications', () => {
    it('should process battery voltage notification', () => {
      const packet: CS108Packet = {
        event: {
          name: 'BATTERY_VOLTAGE',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: 58, // Scalar payload - just the percentage
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      expect(postMessageSpy).toHaveBeenCalledTimes(1);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'BATTERY_UPDATE',
        payload: {
          percentage: 58,
        },
        timestamp: expect.any(Number)
      }));
    });
  });

  describe('trigger notifications', () => {
    it('should process trigger pressed notification', () => {
      const packet: CS108Packet = {
        event: {
          name: 'TRIGGER_PRESSED',
          eventCode: 0xA102,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: undefined,
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      expect(postMessageSpy).toHaveBeenCalledTimes(1);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'TRIGGER_STATE_CHANGED',
        payload: {
          pressed: true,
        },
        timestamp: expect.any(Number)
      }));
    });

    it('should process trigger released notification', () => {
      const packet: CS108Packet = {
        event: {
          name: 'TRIGGER_RELEASED',
          eventCode: 0xA103,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: undefined,
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      expect(postMessageSpy).toHaveBeenCalledTimes(1);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'TRIGGER_STATE_CHANGED',
        payload: {
          pressed: false,
        },
        timestamp: expect.any(Number)
      }));
    });

    it('should process trigger state query response', () => {
      const packet: CS108Packet = {
        event: {
          name: 'TRIGGER_STATE',
          eventCode: 0xA001,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: 1, // 1 = pressed
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      expect(postMessageSpy).toHaveBeenCalledTimes(2);
      expect(postMessageSpy).toHaveBeenNthCalledWith(1, expect.objectContaining({
        type: 'TRIGGER_STATE_CHANGED',
        payload: {
          pressed: true,
        },
        timestamp: expect.any(Number)
      }));
      expect(postMessageSpy).toHaveBeenNthCalledWith(2, expect.objectContaining({
        type: 'COMMAND_RESPONSE',
        payload: {
          command: 'GET_TRIGGER_STATE',
          response: { pressed: true },
        },
        timestamp: expect.any(Number)
      }));
    });
  });

  describe('error notifications', () => {
    it('should process error notification', () => {
      const packet: CS108Packet = {
        event: {
          name: 'ERROR_NOTIFICATION',
          eventCode: 0xA101,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: 0x0004, // Unknown event error
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      expect(postMessageSpy).toHaveBeenCalledTimes(1);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'DEVICE_ERROR',
        payload: {
          severity: 'warning',
          message: 'Unknown event code',
          code: '0004',
          details: { module: undefined },
        },
        timestamp: expect.any(Number)
      }));
    });
  });

  describe('RFID inventory notifications', () => {
    // TODO: Fix inventory handler integration test
    it.skip('should process inventory tags in INVENTORY mode', () => {
      currentMode = ReaderMode.INVENTORY;

      const packet: CS108Packet = {
        event: {
          name: 'INVENTORY_TAG',
          eventCode: 0x8100,
          module: 0xC2,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: {
          epc: 'E2001234567890AB',
          rssi: -50,
          pc: 0x3000,
        },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      // Add multiple tags
      router.handleNotification(packet);
      router.handleNotification({
        ...packet,
        payload: { epc: 'E2001234567890AC', rssi: -55, pc: 0x3000 },
      });
      router.handleNotification({
        ...packet,
        payload: { epc: 'E2001234567890AD', rssi: -60, pc: 0x3000 },
      });

      // Tags are batched, so we need to wait for flush
      vi.advanceTimersByTime(100);

      // Should have emitted a batch
      expect(postMessageSpy).toHaveBeenCalledTimes(1);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'TAG_BATCH',
        payload: expect.objectContaining({
          tags: expect.arrayContaining([
            expect.objectContaining({ epc: 'E2001234567890AB' }),
            expect.objectContaining({ epc: 'E2001234567890AC' }),
            expect.objectContaining({ epc: 'E2001234567890AD' })
          ])
        }),
        timestamp: expect.any(Number)
      }));
    });

    it('should not process inventory tags in IDLE mode', () => {
      currentMode = ReaderMode.IDLE;

      const packet: CS108Packet = {
        event: {
          name: 'INVENTORY_TAG',
          eventCode: 0x8100,
          module: 0xC2,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: {
          epc: 'E2001234567890AB',
          rssi: -50,
          pc: 0x3000,
        },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      // Should not emit any events
      expect(postMessageSpy).not.toHaveBeenCalled();
    });
  });

  describe('RFID locate notifications', () => {
    it.skip('should process all tags in LOCATE mode', () => {
      currentMode = ReaderMode.LOCATE;
      // Note: Target EPC filtering now happens in locateStore, not in the handler

      // Create raw payload for inventory tag
      const epcBytes = Buffer.from('E2001234567890AB', 'hex');
      const rawPayload = new Uint8Array([
        0x30, 0x00, // PC
        ...epcBytes, // EPC
        0xCE, // RSSI (-50 in signed byte)
      ]);

      const packet: CS108Packet = {
        prefix: 0xA7,
        transport: 0xB3,
        length: rawPayload.length,
        module: 0xC2,
        reserve: 0x82,
        direction: 0x9E,
        crc: 0x0000,
        eventCode: 0x8100, // Root level eventCode
        event: {
          name: 'INVENTORY_TAG',
          eventCode: 0x8100,
          module: 0xC2,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        rawPayload, // Use rawPayload instead of rawData
        totalExpected: 8 + rawPayload.length,
        isComplete: true,
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      // Locate mode emits immediately (no batching)
      expect(postMessageSpy).toHaveBeenCalledTimes(1);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'LOCATE_UPDATE',
        payload: expect.objectContaining({
          epc: 'E2001234567890AB',
          rssi: -50,
          smoothedRssi: expect.any(Number),
          averageRssi: expect.any(Number),
          timestamp: expect.any(Number)
        }),
        timestamp: expect.any(Number)
      }));
    });

    it.skip('should emit LOCATE_UPDATE for all tags in LOCATE mode', () => {
      currentMode = ReaderMode.LOCATE;
      // Note: ALL tags are emitted in locate mode now
      // The application layer (locateStore) handles target EPC filtering

      // Create raw payload for inventory tag
      const epcBytes = Buffer.from('E2001234567890AC', 'hex');
      const rawPayload = new Uint8Array([
        0x30, 0x00, // PC
        ...epcBytes, // EPC (Different EPC)
        0xCE, // RSSI (-50 in signed byte)
      ]);

      const packet: CS108Packet = {
        prefix: 0xA7,
        transport: 0xB3,
        length: rawPayload.length,
        module: 0xC2,
        reserve: 0x82,
        direction: 0x9E,
        crc: 0x0000,
        eventCode: 0x8100, // Root level eventCode
        event: {
          name: 'INVENTORY_TAG',
          eventCode: 0x8100,
          module: 0xC2,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        rawPayload, // Use rawPayload instead of rawData
        totalExpected: 8 + rawPayload.length,
        isComplete: true,
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      // Should emit LOCATE_UPDATE for all tags (filtering happens in locateStore)
      expect(postMessageSpy).toHaveBeenCalledTimes(1);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'LOCATE_UPDATE',
        payload: expect.objectContaining({
          epc: 'E2001234567890AC',
          rssi: -50,
          smoothedRssi: expect.any(Number),
          averageRssi: expect.any(Number),
          timestamp: expect.any(Number)
        }),
        timestamp: expect.any(Number)
      }));
    });
  });

  describe('barcode notifications', () => {
    it('should process barcode data in BARCODE mode', () => {
      currentMode = ReaderMode.BARCODE;

      const packet: CS108Packet = {
        event: {
          name: 'BARCODE_DATA',
          eventCode: 0x9100,
          module: 0x6A,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: {
          data: '123456789012',
          symbology: 0x08, // UPC-A
        },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      expect(postMessageSpy).toHaveBeenCalledTimes(1);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'BARCODE_READ',
        payload: expect.objectContaining({
          barcode: '123456789012',
          symbology: 'UPC-A',
          timestamp: expect.any(Number)
        }),
        timestamp: expect.any(Number)
      }));
    });

    it('should process barcode good read confirmation', () => {
      currentMode = ReaderMode.BARCODE;

      const packet: CS108Packet = {
        event: {
          name: 'BARCODE_GOOD_READ',
          eventCode: 0x9101,
          module: 0x6A,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: undefined,
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      expect(postMessageSpy).toHaveBeenCalledTimes(1);
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'BARCODE_GOOD_READ',
        payload: {
          confirmationNumber: 1,
        },
        timestamp: expect.any(Number)
      }));
    });

    it('should not process barcode data in INVENTORY mode', () => {
      currentMode = ReaderMode.INVENTORY;

      const packet: CS108Packet = {
        event: {
          name: 'BARCODE_DATA',
          eventCode: 0x9100,
          module: 0x6A,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: {
          data: '123456789012',
          symbology: 0x08,
        },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      // Should not emit any events
      expect(postMessageSpy).not.toHaveBeenCalled();
    });
  });

  describe('unknown notifications', () => {
    it('should silently ignore unknown event codes', () => {
      const packet: CS108Packet = {
        event: {
          name: 'UNKNOWN_EVENT',
          eventCode: 0xFFFF,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: { some: 'data' },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      // Should not emit any events
      expect(postMessageSpy).not.toHaveBeenCalled();
    });
  });

  describe('handler registration', () => {
    it('should report registered handlers', () => {
      expect(manager.hasHandlerFor(0xA000)).toBe(true); // Battery
      expect(manager.hasHandlerFor(0xA102)).toBe(true); // Trigger pressed
      expect(manager.hasHandlerFor(0xA103)).toBe(true); // Trigger released
      expect(manager.hasHandlerFor(0xA101)).toBe(true); // Error
      expect(manager.hasHandlerFor(0x8100)).toBe(true); // Inventory tag
      expect(manager.hasHandlerFor(0x9100)).toBe(true); // Barcode data
      expect(manager.hasHandlerFor(0x9101)).toBe(true); // Barcode good read
      expect(manager.hasHandlerFor(0xFFFF)).toBe(false); // Unknown
    });
  });
});