import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { getNormalModePayloads, getCompactModePayloads } from '../../../../../tests/data/inventory-by-mode';

// First, mock all modules
vi.mock('../parser');
vi.mock('../../../types/events');

// Now import the modules after mocking
import { InventoryTagHandler, createInventoryHandler, type HandlerConfig } from './handler';
import type { NotificationContext } from '../../notification/types';
import type { CS108Packet } from '../../type';
import { ReaderMode } from '../../../types/reader';
import { ReaderState } from '../../../types/reader';
import { WorkerEventType, postWorkerEvent } from '../../../types/events';
import { InventoryParser } from '../parser';

// Cast mocks to get proper typing
const mockPostWorkerEvent = vi.mocked(postWorkerEvent);
const MockInventoryParser = vi.mocked(InventoryParser);

describe('InventoryTagHandler', () => {
  let handler: InventoryTagHandler;
  let context: NotificationContext;
  let mockProcessInventoryPayload: any;
  let mockGetBufferMetrics: any;

  beforeEach(() => {
    // Reset all mocks
    vi.clearAllMocks();

    // Setup default mock returns
    mockProcessInventoryPayload = vi.fn().mockReturnValue([]);
    mockGetBufferMetrics = vi.fn().mockReturnValue({
      size: 65536,
      used: 0,
      utilizationPercent: 0,
      available: 65536
    });

    // Setup InventoryParser mock
    MockInventoryParser.mockImplementation(() => ({
      processInventoryPayload: mockProcessInventoryPayload,
      getBufferMetrics: mockGetBufferMetrics,
      reset: vi.fn(),
      checkBufferHealth: vi.fn(),
      getState: vi.fn()
    }) as any);

    // Create handler
    handler = new InventoryTagHandler();

    // Default context
    context = {
      currentMode: ReaderMode.INVENTORY,
      readerState: ReaderState.CONNECTED,
      emit: vi.fn()
    };
  });

  afterEach(() => {
    handler.cleanup();
  });

  describe('canHandle', () => {
    it('accepts 0x8100 packets in INVENTORY mode', () => {
      const packet: CS108Packet = {
        eventCode: 0x8100,
        rawPayload: new Uint8Array([0x04, 0x00, 0x05, 0x80]),
        prefix: 0xA7B3,
        messageLength: 4,
        flags: 0,
        reserve: 0,
        crc16: 0
      };

      expect(handler.canHandle(packet, context)).toBe(true);
    });

    it('accepts 0x8100 packets in LOCATE mode', () => {
      context.currentMode = ReaderMode.LOCATE;
      const packet: CS108Packet = {
        eventCode: 0x8100,
        rawPayload: new Uint8Array([0x04, 0x00, 0x05, 0x80]),
        prefix: 0xA7B3,
        messageLength: 4,
        flags: 0,
        reserve: 0,
        crc16: 0
      };

      expect(handler.canHandle(packet, context)).toBe(true);
    });

    it('rejects non-0x8100 packets', () => {
      const packet: CS108Packet = {
        eventCode: 0x8002,
        rawPayload: new Uint8Array([]),
        prefix: 0xA7B3,
        messageLength: 0,
        flags: 0,
        reserve: 0,
        crc16: 0
      };

      expect(handler.canHandle(packet, context)).toBe(false);
    });

    it('rejects 0x8100 packets in IDLE mode', () => {
      context.currentMode = ReaderMode.IDLE;
      const packet: CS108Packet = {
        eventCode: 0x8100,
        rawPayload: new Uint8Array([]),
        prefix: 0xA7B3,
        messageLength: 0,
        flags: 0,
        reserve: 0,
        crc16: 0
      };

      expect(handler.canHandle(packet, context)).toBe(false);
    });
  });

  describe('handle', () => {
    it('processes inventory packets and emits TAG_READ events', async () => {
      const testTags = [
        {
          epc: 'E280116060000123456789AB',
          pc: 0x3000,
          rssi: -64,
          timestamp: Date.now(),
          mode: 'compact' as const
        }
      ];

      mockProcessInventoryPayload.mockReturnValue(testTags);

      const packet: CS108Packet = {
        eventCode: 0x8100,
        rawPayload: new Uint8Array([0x04, 0x00, 0x05, 0x80]),
        prefix: 0xA7B3,
        messageLength: 4,
        flags: 0,
        reserve: 123,
        crc16: 0
      };

      await handler.handle(packet, context);

      // Verify parser was called with payload and sequence
      expect(mockProcessInventoryPayload).toHaveBeenCalledWith(
        packet.rawPayload,
        123
      );

      // Verify TAG_READ event was emitted with array (stream-as-we-parse)
      expect(mockPostWorkerEvent).toHaveBeenCalledTimes(1);
      expect(mockPostWorkerEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          type: WorkerEventType.TAG_READ,
          payload: expect.objectContaining({
            tags: testTags,
            timestamp: expect.any(Number)
          })
        })
      );
    });

    it('emits TAG_READ event with array of tags for multiple tags', async () => {
      const testTags = [
        {
          epc: 'E280116060000123456789AB',
          pc: 0x3000,
          rssi: -45,
          timestamp: Date.now(),
          mode: 'compact' as const
        },
        {
          epc: 'E280116060000123456789AC',
          pc: 0x3000,
          rssi: -60,
          timestamp: Date.now(),
          mode: 'compact' as const
        },
        {
          epc: 'E280116060000123456789AD',
          pc: 0x3000,
          rssi: -55,
          timestamp: Date.now(),
          mode: 'compact' as const
        }
      ];

      mockProcessInventoryPayload.mockReturnValue(testTags);

      const packet: CS108Packet = {
        eventCode: 0x8100,
        rawPayload: new Uint8Array([0x04, 0x00, 0x05, 0x80]),
        prefix: 0xA7B3,
        messageLength: 4,
        flags: 0,
        reserve: 123,
        crc16: 0
      };

      await handler.handle(packet, context);

      // Should emit single TAG_READ event with array of tags
      expect(mockPostWorkerEvent).toHaveBeenCalledTimes(1);
      expect(mockPostWorkerEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          type: WorkerEventType.TAG_READ,
          payload: expect.objectContaining({
            tags: testTags,
            timestamp: expect.any(Number)
          })
        })
      );
    });

    it('emits LOCATE_UPDATE events in LOCATE mode', async () => {
      context.currentMode = ReaderMode.LOCATE;

      const testTags = [
        {
          epc: 'E280116060000123456789AB',
          pc: 0x3000,
          rssi: -45,
          timestamp: Date.now()
        },
        {
          epc: 'E280116060000123456789AC',
          pc: 0x3000,
          rssi: -60,
          timestamp: Date.now()
        }
      ];

      mockProcessInventoryPayload.mockReturnValue(testTags);

      const packet: CS108Packet = {
        eventCode: 0x8100,
        rawPayload: new Uint8Array([0x03, 0x12, 0x05, 0x80]),
        prefix: 0xA7B3,
        messageLength: 4,
        flags: 0,
        reserve: 0,
        crc16: 0
      };

      await handler.handle(packet, context);

      // Should emit LOCATE_UPDATE with strongest tag
      expect(mockPostWorkerEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          type: WorkerEventType.LOCATE_UPDATE,
          payload: expect.objectContaining({
            epc: testTags[0].epc, // Strongest RSSI tag
            rssi: testTags[0].rssi,
            timestamp: expect.any(Number)
          })
        })
      );
    });

    it('handles empty payloads gracefully', async () => {
      mockProcessInventoryPayload.mockReturnValue([]);

      const packet: CS108Packet = {
        eventCode: 0x8100,
        rawPayload: new Uint8Array([0x02, 0x00]), // Status packet
        prefix: 0xA7B3,
        messageLength: 2,
        flags: 0,
        reserve: 0,
        crc16: 0
      };

      await handler.handle(packet, context);

      // Should not emit any events for empty results
      expect(mockPostWorkerEvent).not.toHaveBeenCalled();
    });

    it('handles parser errors gracefully', async () => {
      mockProcessInventoryPayload.mockImplementation(() => {
        throw new Error('Parse error');
      });

      const packet: CS108Packet = {
        eventCode: 0x8100,
        rawPayload: new Uint8Array([0xFF, 0xFF]),
        prefix: 0xA7B3,
        messageLength: 2,
        flags: 0,
        reserve: 0,
        crc16: 0
      };

      // Should not throw
      await expect(handler.handle(packet, context)).resolves.not.toThrow();

      // Should not emit tag events on error
      expect(mockPostWorkerEvent).not.toHaveBeenCalledWith(
        expect.objectContaining({
          type: WorkerEventType.TAG_READ
        })
      );
    });

    it('emits PARSE_ERROR in debug mode on error', async () => {
      // Create handler with debug enabled
      handler = new InventoryTagHandler({ debug: true });

      mockProcessInventoryPayload.mockImplementation(() => {
        throw new Error('Test parse error');
      });

      const packet: CS108Packet = {
        eventCode: 0x8100,
        rawPayload: new Uint8Array([0xFF, 0xFF]),
        prefix: 0xA7B3,
        messageLength: 2,
        flags: 0,
        reserve: 42,
        crc16: 0
      };

      await handler.handle(packet, context);

      // Should emit PARSE_ERROR event
      expect(mockPostWorkerEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          type: WorkerEventType.PARSE_ERROR,
          payload: expect.objectContaining({
            error: 'Test parse error',
            packet: expect.objectContaining({
              eventCode: 0x8100,
              payloadLength: 2,
              reserve: 42
            })
          })
        })
      );
    });

    it('monitors buffer health periodically', async () => {
      // Process 100 packets to trigger health check
      for (let i = 0; i < 100; i++) {
        const packet: CS108Packet = {
          eventCode: 0x8100,
          rawPayload: new Uint8Array([0x70, 0x00]),
          prefix: 0xA7B3,
          messageLength: 2,
          flags: 0,
          reserve: i,
          crc16: 0
        };
        await handler.handle(packet, context);
      }

      // Should have checked buffer health
      expect(mockGetBufferMetrics).toHaveBeenCalled();
    });

    it('emits BUFFER_WARNING when utilization > 80%', async () => {
      // Mock high buffer utilization
      mockGetBufferMetrics.mockReturnValue({
        size: 65536,
        used: 52429,
        utilizationPercent: 80.5,
        available: 13107
      });

      // Process 100 packets to trigger health check
      for (let i = 0; i < 100; i++) {
        const packet: CS108Packet = {
          eventCode: 0x8100,
          rawPayload: new Uint8Array([0x70, 0x00]),
          prefix: 0xA7B3,
          messageLength: 2,
          flags: 0,
          reserve: i,
          crc16: 0
        };
        await handler.handle(packet, context);
      }

      // Should have emitted BUFFER_WARNING
      expect(mockPostWorkerEvent).toHaveBeenCalledWith(
        expect.objectContaining({
          type: WorkerEventType.BUFFER_WARNING,
          payload: expect.objectContaining({
            utilizationPercent: 80.5,
            used: 52429,
            size: 65536
          })
        })
      );
    });
  });

  describe('configuration', () => {
    it('initializes with default config', () => {
      const handler = new InventoryTagHandler();
      const stats = handler.getStats();
      expect(stats).toBeDefined();
      expect(stats.packetsProcessed).toBe(0);
    });

    it('accepts custom configuration', () => {
      const config: HandlerConfig = {
        mode: 'normal',
        bufferMonitoring: true,
        debug: true
      };

      const handler = new InventoryTagHandler(config);
      const stats = handler.getStats();
      expect(stats).toBeDefined();
    });

    it('updates configuration dynamically', () => {
      handler.updateConfig({
        mode: 'normal',
        bufferMonitoring: true
      });

      const stats = handler.getStats();
      expect(stats).toBeDefined();
    });
  });

  describe('statistics', () => {
    it('tracks handler statistics accurately', async () => {
      const testTags = [
        { epc: 'TAG1', pc: 0x3000, rssi: -50, timestamp: Date.now() },
        { epc: 'TAG2', pc: 0x3000, rssi: -55, timestamp: Date.now() }
      ];

      mockProcessInventoryPayload.mockReturnValue(testTags);

      // Process 3 packets
      for (let i = 0; i < 3; i++) {
        const packet: CS108Packet = {
          eventCode: 0x8100,
          rawPayload: new Uint8Array([0x04, 0x00]),
          prefix: 0xA7B3,
          messageLength: 2,
          flags: 0,
          reserve: i,
          crc16: 0
        };
        await handler.handle(packet, context);
      }

      const stats = handler.getStats();
      expect(stats.packetsProcessed).toBe(3);
      expect(stats.tagsExtracted).toBe(6); // 2 tags * 3 packets
    });

    it('reports buffer metrics', () => {
      const mockMetrics = {
        size: 65536,
        used: 1024,
        utilizationPercent: 1.5,
        available: 64512
      };

      mockGetBufferMetrics.mockReturnValue(mockMetrics);

      const stats = handler.getStats();
      expect(stats.bufferMetrics).toEqual(mockMetrics);
    });
  });

  describe('cleanup', () => {
    it('resets state on cleanup', () => {
      handler.cleanup();

      const stats = handler.getStats();
      expect(stats.packetsProcessed).toBe(0);
      expect(stats.tagsExtracted).toBe(0);
    });
  });

  describe('factory function', () => {
    it('creates handler with configuration', () => {
      const handler = createInventoryHandler({
        mode: 'normal',
        debug: true
      });

      expect(handler).toBeInstanceOf(InventoryTagHandler);
    });

    it('creates handler without configuration', () => {
      const handler = createInventoryHandler();
      expect(handler).toBeInstanceOf(InventoryTagHandler);
    });
  });

  describe('mixed payload handling', () => {
    it('handles mixed mode payloads from real capture', async () => {

      const totalTags = 0;
      let normalPackets = 0;
      let compactPackets = 0;

      // Get a mix of both normal and compact payloads
      const normalPayloads = getNormalModePayloads(25);
      const compactPayloads = getCompactModePayloads(25);
      const mixedPayloads = [...normalPayloads, ...compactPayloads];

      // Process the mixed payloads
      for (const payloadArray of mixedPayloads) {
        const modeType = payloadArray[0];

        // Track packet types
        if (modeType === 0x03) normalPackets++;
        if (modeType === 0x04) compactPackets++;

        // Mock appropriate response based on type
        if (modeType === 0x03 || modeType === 0x04) {
          // Tag-bearing packets
          mockProcessInventoryPayload.mockReturnValue([
            { epc: 'TEST', pc: 0x3000, rssi: -50, timestamp: Date.now() }
          ]);
        } else {
          // Status/keepalive packets
          mockProcessInventoryPayload.mockReturnValue([]);
        }

        const packet: CS108Packet = {
          eventCode: 0x8100,
          rawPayload: payloadArray,
          prefix: 0xA7B3,
          messageLength: payloadArray.length,
          flags: 0,
          reserve: 0,
          crc16: 0
        };

        await handler.handle(packet, context);
      }

      // Should have processed all packet types
      expect(normalPackets).toBeGreaterThan(0);
      expect(compactPackets).toBeGreaterThan(0);
    });
  });
});