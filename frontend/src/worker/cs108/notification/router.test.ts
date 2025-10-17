/**
 * Tests for NotificationRouter
 */

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { NotificationRouter, type RouterConfig } from './router';
import type {
  NotificationHandler,
  NotificationContext,
} from './types';
import type { CS108Packet, CS108Event } from '../type';
import { ReaderMode, ReaderState } from '../../types/reader';

describe('NotificationRouter', () => {
  let router: NotificationRouter;
  let emitNotificationEvent: ReturnType<typeof vi.fn>;
  let config: RouterConfig;
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;
  let consoleWarnSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    emitNotificationEvent = vi.fn();
    config = {
      debug: false,
      getCurrentMode: () => ReaderMode.IDLE,
      getReaderState: () => ReaderState.CONNECTED,
      onError: vi.fn(),
    };
    router = new NotificationRouter(emitNotificationEvent, config);

    consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    consoleWarnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
    consoleWarnSpy.mockRestore();
  });

  describe('handler registration', () => {
    it('should register a handler for an event code', () => {
      const handler: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
      };

      router.register(0xA000, handler);
      expect(router.hasHandler(0xA000)).toBe(true);
    });

    it('should support multiple handlers for the same event code', () => {
      const handler1: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
      };
      const handler2: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
      };

      router.register(0xA000, handler1);
      router.register(0xA000, handler2);

      // Should NOT warn since multiple handlers are now supported
      expect(consoleWarnSpy).not.toHaveBeenCalled();

      // Both handlers should be registered
      expect(router.hasHandler(0xA000)).toBe(true);
    });

    it('should unregister a handler', () => {
      const handler: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
        cleanup: vi.fn(),
      };

      router.register(0xA000, handler);
      router.unregister(0xA000);

      expect(router.hasHandler(0xA000)).toBe(false);
      expect(handler.cleanup).toHaveBeenCalled();
    });

    it('should handle cleanup errors gracefully', () => {
      const handler: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
        cleanup: vi.fn(() => {
          throw new Error('Cleanup failed');
        }),
      };

      router.register(0xA000, handler);
      router.unregister(0xA000);

      expect(consoleErrorSpy).toHaveBeenCalledWith(
        '[Worker] ERROR:',
        expect.stringContaining('Error during handler cleanup'),
        expect.any(Error)
      );
      expect(router.hasHandler(0xA000)).toBe(false);
    });
  });

  describe('notification handling', () => {
    it('should route packet to registered handler', () => {
      const handler: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
      };

      const packet: CS108Packet = {
        event: {
          name: 'TEST_EVENT',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: { test: 'data' },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.register(0xA000, handler);
      router.handleNotification(packet);

      expect(handler.canHandle).toHaveBeenCalledWith(packet, expect.objectContaining({
        currentMode: ReaderMode.IDLE,
        readerState: ReaderState.CONNECTED,
        emitNotificationEvent,
      }));
      expect(handler.handle).toHaveBeenCalledWith(packet, expect.any(Object));
    });

    it('should not call handle if canHandle returns false', () => {
      const handler: NotificationHandler = {
        canHandle: vi.fn(() => false),
        handle: vi.fn(),
      };

      const packet: CS108Packet = {
        event: {
          name: 'TEST_EVENT',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: { test: 'data' },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.register(0xA000, handler);
      router.handleNotification(packet);

      expect(handler.canHandle).toHaveBeenCalled();
      expect(handler.handle).not.toHaveBeenCalled();
    });

    it('should handle errors in canHandle gracefully', () => {
      const handler: NotificationHandler = {
        canHandle: vi.fn(() => {
          throw new Error('canHandle failed');
        }),
        handle: vi.fn(),
      };

      const packet: CS108Packet = {
        event: {
          name: 'TEST_EVENT',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: { test: 'data' },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.register(0xA000, handler);
      router.handleNotification(packet);

      expect(consoleErrorSpy).toHaveBeenCalledWith(
        '[Worker] ERROR:',
        expect.stringContaining('Error in canHandle'),
        expect.any(Error)
      );
      expect(handler.handle).not.toHaveBeenCalled();
    });

    it('should handle errors in handle with error boundary', () => {
      const handler: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn(() => {
          throw new Error('Handler failed');
        }),
      };

      const packet: CS108Packet = {
        event: {
          name: 'TEST_EVENT',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: { test: 'data' },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.register(0xA000, handler);
      router.handleNotification(packet);

      expect(consoleErrorSpy).toHaveBeenCalledWith(
        '[Worker] ERROR:',
        expect.stringContaining('Error handling event'),
        expect.any(Error)
      );
      expect(config.onError).toHaveBeenCalledWith(expect.any(Error), packet);
    });

    it('should ignore packets with no registered handler', () => {
      const packet: CS108Packet = {
        event: {
          name: 'UNKNOWN_EVENT',
          eventCode: 0xFFFF,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: { test: 'data' },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.handleNotification(packet);

      // Should not throw, just silently ignore
      expect(emitNotificationEvent).not.toHaveBeenCalled();
    });
  });

  describe('batch operations', () => {
    it('should register multiple handlers at once', () => {
      const handlers = new Map<number, NotificationHandler>([
        [0xA000, {
          canHandle: vi.fn(() => true),
          handle: vi.fn(),
        }],
        [0xA001, {
          canHandle: vi.fn(() => true),
          handle: vi.fn(),
        }],
        [0xA002, {
          canHandle: vi.fn(() => true),
          handle: vi.fn(),
        }],
      ]);

      router.registerAll(handlers);

      expect(router.hasHandler(0xA000)).toBe(true);
      expect(router.hasHandler(0xA001)).toBe(true);
      expect(router.hasHandler(0xA002)).toBe(true);
    });

    it('should clear all handlers', () => {
      const handler1: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
        cleanup: vi.fn(),
      };
      const handler2: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
        cleanup: vi.fn(),
      };

      router.register(0xA000, handler1);
      router.register(0xA001, handler2);

      router.clear();

      expect(router.hasHandler(0xA000)).toBe(false);
      expect(router.hasHandler(0xA001)).toBe(false);
      expect(handler1.cleanup).toHaveBeenCalled();
      expect(handler2.cleanup).toHaveBeenCalled();
    });
  });

  describe('query operations', () => {
    it('should return list of registered event codes', () => {
      router.register(0xA000, {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
      });
      router.register(0xA001, {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
      });

      const events = router.getRegisteredEvents();
      expect(events).toContain(0xA000);
      expect(events).toContain(0xA001);
      expect(events).toHaveLength(2);
    });

    it('should return handler for event code', () => {
      const handler: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn(),
      };

      router.register(0xA000, handler);
      expect(router.getHandler(0xA000)).toBe(handler);
      expect(router.getHandler(0xA001)).toBeUndefined();
    });
  });

  describe('context building', () => {
    it('should provide correct context to handlers', () => {
      let capturedContext: NotificationContext | undefined;
      const handler: NotificationHandler = {
        canHandle: vi.fn(() => true),
        handle: vi.fn((packet, context) => {
          capturedContext = context;
        }),
      };

      config.getCurrentMode = () => ReaderMode.INVENTORY;
      config.getReaderState = () => ReaderState.SCANNING;

      const packet: CS108Packet = {
        event: {
          name: 'TEST_EVENT',
          eventCode: 0xA000,
          module: 0xD9,
          isCommand: false,
          isNotification: true,
        } as CS108Event,
        payload: { test: 'data' },
        rawData: new Uint8Array([]),
        timestamp: Date.now(),
      };

      router.register(0xA000, handler);
      router.handleNotification(packet);

      expect(capturedContext).toBeDefined();
      expect(capturedContext?.currentMode).toBe(ReaderMode.INVENTORY);
      expect(capturedContext?.readerState).toBe(ReaderState.SCANNING);
      expect(capturedContext?.emitNotificationEvent).toBe(emitNotificationEvent);
      expect(capturedContext?.metadata).toEqual({});
    });
  });
});