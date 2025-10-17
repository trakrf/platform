/**
 * Tests for BatteryHandler
 */

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { BatteryHandler } from './battery';
import type { NotificationContext } from '../notification/types';
import type { CS108Packet } from '../type';
import { ReaderMode, ReaderState } from '../../types/reader';
import { logger, LogLevel } from '../../utils/logger';

describe('BatteryHandler', () => {
  let handler: BatteryHandler;
  let context: NotificationContext;
  let postMessageSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    handler = new BatteryHandler();
    postMessageSpy = vi.fn();
    globalThis.postMessage = postMessageSpy;
    context = {
      currentMode: ReaderMode.IDLE,
      readerState: ReaderState.CONNECTED,
      emitNotificationEvent: vi.fn(), // Still needed for NotificationContext type
      metadata: {},
    };
  });

  describe('canHandle', () => {
    it('should handle packets with scalar payload (percentage)', () => {
      const packet: CS108Packet = {
        event: {
          name: 'BATTERY_VOLTAGE',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        },
        payload: 50, // Scalar payload - just the percentage
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      expect(handler.canHandle(packet, context)).toBe(true);
    });

    it('should not handle packets without payload', () => {
      const packet: CS108Packet = {
        event: {
          name: 'BATTERY_VOLTAGE',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        },
        payload: undefined,
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      expect(handler.canHandle(packet, context)).toBe(false);
    });

    it('should not handle packets with non-scalar payload', () => {
      const packet: CS108Packet = {
        event: {
          name: 'BATTERY_VOLTAGE',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        },
        payload: {
          something: 'else',
        },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      expect(handler.canHandle(packet, context)).toBe(false);
    });
  });

  describe('handle', () => {
    it('should emit battery update with percentage from scalar payload', () => {
      const packet: CS108Packet = {
        event: {
          name: 'BATTERY_VOLTAGE',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        },
        payload: 50, // Scalar payload - percentage
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      handler.handle(packet, context);

      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'BATTERY_UPDATE',
        payload: {
          percentage: 50,
        },
        timestamp: expect.any(Number),
      }));
    });

    it('should handle different percentage values', () => {
      const packet: CS108Packet = {
        event: {
          name: 'BATTERY_VOLTAGE',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        },
        payload: 75, // 75% battery
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      handler.handle(packet, context);

      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'BATTERY_UPDATE',
        payload: {
          percentage: 75,
        },
        timestamp: expect.any(Number),
      }));
    });

    it('should handle minimum battery (0%)', () => {
      const packet: CS108Packet = {
        event: {
          name: 'BATTERY_VOLTAGE',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        },
        payload: 0, // 0% battery
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      handler.handle(packet, context);

      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'BATTERY_UPDATE',
        payload: {
          percentage: 0,
        },
        timestamp: expect.any(Number),
      }));
    });

    it('should handle maximum battery (100%)', () => {
      const packet: CS108Packet = {
        event: {
          name: 'BATTERY_VOLTAGE',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        },
        payload: 100, // 100% battery
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      handler.handle(packet, context);

      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: 'BATTERY_UPDATE',
        payload: {
          percentage: 100,
        },
        timestamp: expect.any(Number),
      }));
    });

    it('should handle various percentage values', () => {
      const testCases = [0, 25, 50, 75, 100];

      testCases.forEach((percentage) => {
        const packet: CS108Packet = {
          event: {
            name: 'BATTERY_VOLTAGE',
            eventCode: 0xA000,
            module: 0xD9,
            isCommand: false,
            isNotification: true,
          },
          payload: percentage,
          rawData: new Uint8Array([]),
          timestamp: Date.now(),
        };

        postMessageSpy.mockClear();
        handler.handle(packet, context);

        expect(postMessageSpy).toHaveBeenCalledWith(
          expect.objectContaining({
            type: 'BATTERY_UPDATE',
            payload: expect.objectContaining({
              percentage,
            }),
            timestamp: expect.any(Number)
          })
        );
      });
    });

    it('should log in debug mode', () => {
      // Set logger to debug level for this test
      logger.setLevel(LogLevel.DEBUG);

      const consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
      context.metadata = { debug: true };

      const packet: CS108Packet = {
        event: {
          name: 'BATTERY_VOLTAGE',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        },
        payload: 50, // 50% battery
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      handler.handle(packet, context);

      expect(consoleSpy).toHaveBeenCalledWith(
        '[Worker] DEBUG:',
        '[BatteryHandler] Battery update: 50%'
      );

      consoleSpy.mockRestore();
    });
  });
});