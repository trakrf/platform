/**
 * CS108 Worker - Flat API for vite-plugin-comlink
 *
 * This worker exports functions directly for the plugin to auto-wrap.
 * The plugin handles all Comlink serialization automatically.
 *
 * The worker manages:
 * - CS108 reader instance
 * - BLE MessagePort communication
 * - Event callbacks to main thread
 */
import { CS108Reader } from './cs108/reader.js';
import type {
  ReaderModeType,
  ReaderSettings
} from './types/reader.js';
import { logger, LogLevel } from './utils/logger.js';

// Reader instance
let reader: CS108Reader | null = null;

// Events flow directly through postMessage - no interception needed
// postWorkerEvent() calls globalThis.postMessage() and events go straight to main thread

// Flat API methods exposed to main thread via Comlink

export async function initialize(port: MessagePort): Promise<boolean> {
  try {
    logger.debug('initialize() called with MessagePort');

    // Create reader if it doesn't exist
    if (!reader) {
      logger.debug('Creating CS108Reader instance');
      reader = new CS108Reader();
    }

    // Set the transport port
    logger.debug('Setting transport port');
    reader.setTransportPort(port);

    // Connect to the device
    logger.debug('Calling reader.connect()...');
    const success = await reader.connect();
    logger.debug(`reader.connect() returned: ${success}`);

    // No event interception needed - events flow directly via postMessage

    return success;
  } catch (error) {
    logger.error('Initialize error:', error);
    throw error;
  }
}

export async function disconnect(): Promise<void> {
  if (reader) {
    await reader.disconnect();
    reader = null;
  }
}

export async function setMode(mode: ReaderModeType, settings?: ReaderSettings): Promise<void> {
  if (!reader) {
    throw new Error('Reader not connected');
  }
  await reader.setMode(mode, settings);
}

export async function setSettings(settings: ReaderSettings): Promise<void> {
  if (!reader) {
    throw new Error('Reader not connected');
  }

  // Apply worker log level if present
  if (settings.system?.workerLogLevel) {
    setLogLevel(settings.system.workerLogLevel);
  }

  await reader.setSettings(settings);
}

export function getSettings(): ReaderSettings {
  if (!reader) {
    throw new Error('Reader not connected');
  }
  return reader.getSettings();
}

export async function startScanning(): Promise<void> {
  if (!reader) {
    throw new Error('Reader not connected');
  }
  await reader.startScanning();
}

export async function stopScanning(): Promise<void> {
  if (!reader) {
    throw new Error('Reader not connected');
  }
  await reader.stopScanning();
}

// Log level utility to map string to numeric LogLevel enum
export function setLogLevel(level: 'error' | 'warn' | 'info' | 'debug' | LogLevel): void {
  if (typeof level === 'string') {
    const logLevelMap: Record<string, LogLevel> = {
      'error': LogLevel.ERROR,
      'warn': LogLevel.WARN,
      'info': LogLevel.INFO,
      'debug': LogLevel.DEBUG
    };
    logger.setLevel(logLevelMap[level] || LogLevel.INFO);
  } else {
    logger.setLevel(level);
  }
}

// Export LogLevel enum for convenience
export { LogLevel };

// No event callbacks needed - events flow directly through postMessage
// The main thread listens to worker.onmessage to receive events