/**
 * Worker Event Types
 *
 * Type-safe event definitions for worker to main thread communication.
 * Uses discriminated unions for proper TypeScript type narrowing.
 */

import type { ReaderStateType, ReaderModeType, ReaderSettings } from './reader';

/**
 * Event type discriminators
 */
export enum WorkerEventType {
  // State events
  READER_STATE_CHANGED = 'READER_STATE_CHANGED',
  READER_MODE_CHANGED = 'READER_MODE_CHANGED',
  SETTINGS_UPDATED = 'SETTINGS_UPDATED',

  // RFID events
  TAG_BATCH = 'TAG_BATCH',
  TAG_READ = 'TAG_READ',
  LOCATE_UPDATE = 'LOCATE_UPDATE',

  // Barcode events
  BARCODE_READ = 'BARCODE_READ',
  BARCODE_GOOD_READ = 'BARCODE_GOOD_READ',
  BARCODE_AUTO_STOP_REQUEST = 'BARCODE_AUTO_STOP_REQUEST',

  // System events
  BATTERY_UPDATE = 'BATTERY_UPDATE',
  DEVICE_ERROR = 'DEVICE_ERROR',
  TRIGGER_STATE_CHANGED = 'TRIGGER_STATE_CHANGED',
  COMMAND_RESPONSE = 'COMMAND_RESPONSE',
  TRANSPORT_DISCONNECTED = 'TRANSPORT_DISCONNECTED',

  // Buffer monitoring events
  BUFFER_WARNING = 'BUFFER_WARNING',
  BUFFER_METRICS = 'BUFFER_METRICS',
  PARSE_ERROR = 'PARSE_ERROR',

  // Error event
  ERROR = 'ERROR',

  // Debug event
  DEBUG_LOG = 'DEBUG_LOG'
}

/**
 * Base event structure
 */
interface WorkerEventBase {
  type: WorkerEventType;
  timestamp: number;
}

/**
 * Reader state change event
 */
interface ReaderStateChangedEvent extends WorkerEventBase {
  type: WorkerEventType.READER_STATE_CHANGED;
  payload: {
    readerState: ReaderStateType;
  };
}

/**
 * Reader mode change event
 */
interface ReaderModeChangedEvent extends WorkerEventBase {
  type: WorkerEventType.READER_MODE_CHANGED;
  payload: {
    mode: ReaderModeType | null;
  };
}

/**
 * Settings update event
 */
interface SettingsUpdatedEvent extends WorkerEventBase {
  type: WorkerEventType.SETTINGS_UPDATED;
  payload: {
    settings: ReaderSettings;
  };
}

/**
 * Tag batch event for inventory
 */
interface TagBatchEvent extends WorkerEventBase {
  type: WorkerEventType.TAG_BATCH;
  payload: {
    tags: Array<{
      epc: string;
      rssi: number;
      phaseAngle?: number;
      doppler?: number;
      channelIndex?: number;
      antennaPort?: number;
      pc?: number | string;  // Can be either number or string
      crc?: string;
      timestamp: number;
      phase?: number;        // Phase from TagData
      antenna?: number;      // Antenna from TagData
    }>;
  };
}

/**
 * Tag read event for streaming tags from a single parse cycle
 * Tags are kept as an array to reduce message passing overhead
 */
interface TagReadEvent extends WorkerEventBase {
  type: WorkerEventType.TAG_READ;
  payload: {
    tags: Array<{
      epc: string;
      rssi: number;
      pc: number;
      antennaPort?: number;
      timestamp: number;
      mode?: 'compact' | 'normal';
      phase?: number;
    }>;
    timestamp: number;
  };
}

/**
 * Locate update event
 */
interface LocateUpdateEvent extends WorkerEventBase {
  type: WorkerEventType.LOCATE_UPDATE;
  payload: {
    epc: string;
    rssi: number;
    wbRssi?: number;  // Wideband RSSI for better locate accuracy
    smoothedRssi?: number;
    averageRssi?: number;
    timestamp: number;
    channelIndex?: number;
    antennaPort?: number;
  };
}

/**
 * Barcode read event
 */
interface BarcodeReadEvent extends WorkerEventBase {
  type: WorkerEventType.BARCODE_READ;
  payload: {
    barcode: string;
    symbology?: string;  // Human-readable symbology name
    rawData?: string;     // Hex string representation of raw data
    timestamp: number;
  };
}

/**
 * Barcode good read confirmation event
 */
interface BarcodeGoodReadEvent extends WorkerEventBase {
  type: WorkerEventType.BARCODE_GOOD_READ;
  payload: {
    confirmationNumber: number;
  };
}

/**
 * Barcode auto-stop request event
 */
interface BarcodeAutoStopRequestEvent extends WorkerEventBase {
  type: WorkerEventType.BARCODE_AUTO_STOP_REQUEST;
  payload: {
    barcode: string;
    reason: string;
  };
}

/**
 * Battery update event
 */
interface BatteryUpdateEvent extends WorkerEventBase {
  type: WorkerEventType.BATTERY_UPDATE;
  payload: {
    percentage: number;
  };
}

/**
 * Device error event
 */
interface DeviceErrorEvent extends WorkerEventBase {
  type: WorkerEventType.DEVICE_ERROR;
  payload: {
    severity: 'warning' | 'error' | 'critical';
    message: string;
    code?: string;
    details?: Record<string, unknown>;
  };
}

/**
 * Trigger state change event
 */
interface TriggerStateChangedEvent extends WorkerEventBase {
  type: WorkerEventType.TRIGGER_STATE_CHANGED;
  payload: {
    pressed: boolean;
  };
}

/**
 * Command response event
 */
interface CommandResponseEvent extends WorkerEventBase {
  type: WorkerEventType.COMMAND_RESPONSE;
  payload: {
    command: string;
    response: unknown;
  };
}

/**
 * Transport disconnection event
 */
interface TransportDisconnectedEvent extends WorkerEventBase {
  type: WorkerEventType.TRANSPORT_DISCONNECTED;
  payload: {
    reason?: string;
  };
}

/**
 * Buffer warning event
 */
interface BufferWarningEvent extends WorkerEventBase {
  type: WorkerEventType.BUFFER_WARNING;
  payload: {
    utilizationPercent: number;
    used: number;
    size: number;
  };
}

/**
 * Buffer metrics event
 */
interface BufferMetricsEvent extends WorkerEventBase {
  type: WorkerEventType.BUFFER_METRICS;
  payload: {
    used: number;
    size: number;
    utilizationPercent: number;
    maxSize: number;
    growthCount: number;
    overflowCount: number;
  };
}

/**
 * Parse error event
 */
interface ParseErrorEvent extends WorkerEventBase {
  type: WorkerEventType.PARSE_ERROR;
  payload: {
    error: string;
    packet: {
      eventCode: number;
      payloadLength: number;
      reserve?: number;
    };
  };
}

/**
 * Generic error event
 */
interface ErrorEvent extends WorkerEventBase {
  type: WorkerEventType.ERROR;
  payload: {
    message: string;
    context?: unknown;
  };
}

/**
 * Debug log event for worker diagnostics
 */
interface DebugLogEvent extends WorkerEventBase {
  type: WorkerEventType.DEBUG_LOG;
  payload: {
    level: 'trace' | 'debug' | 'info' | 'warn' | 'error';
    message: string;
    context?: string;  // e.g., 'setMode', 'executeCommand', 'connect'
    details?: Record<string, unknown>;
  };
}

/**
 * Union type of all worker events
 */
export type WorkerEvent =
  | ReaderStateChangedEvent
  | ReaderModeChangedEvent
  | SettingsUpdatedEvent
  | TagBatchEvent
  | TagReadEvent
  | LocateUpdateEvent
  | BarcodeReadEvent
  | BarcodeGoodReadEvent
  | BarcodeAutoStopRequestEvent
  | BatteryUpdateEvent
  | DeviceErrorEvent
  | TriggerStateChangedEvent
  | CommandResponseEvent
  | TransportDisconnectedEvent
  | BufferWarningEvent
  | BufferMetricsEvent
  | ParseErrorEvent
  | ErrorEvent
  | DebugLogEvent;

/**
 * Helper function to post worker events
 * Adds timestamp automatically and ensures type safety
 * Handles both worker and jsdom/test environments
 */
export function postWorkerEvent(event: Omit<WorkerEvent, 'timestamp'>): void {
  const message = {
    ...event,
    timestamp: Date.now()
  };

  // In a real worker, postMessage only needs the message
  // In jsdom (tests), it needs targetOrigin as second argument
  if (typeof globalThis.postMessage === 'function') {
    try {
      // Try worker context first (single argument)
      globalThis.postMessage(message);
    } catch (error) {
      // Fall back to jsdom context (requires targetOrigin)
      if (typeof window !== 'undefined' && globalThis.postMessage === window.postMessage) {
        (globalThis as unknown as Window).postMessage(message, '*');
      } else {
        throw error;
      }
    }
  }
}