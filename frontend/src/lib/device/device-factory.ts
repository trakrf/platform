/**
 * Device Factory
 * Creates appropriate device worker based on device type
 */

import * as Comlink from 'comlink';
import type { IHandheldDevice } from './types';
import type { WorkerProxy } from '@/types/worker-proxy';

export type DeviceType = 'CS108' | 'Zebra' | 'Nordic';

export interface DeviceFactoryConfig {
  deviceType: DeviceType;
  workerPath?: string;
}

export interface DeviceFactoryResult {
  worker: Worker;
  device: WorkerProxy<unknown>;
}

export class DeviceFactory {
  /**
   * Create a device worker instance
   */
  static async create(config: DeviceFactoryConfig): Promise<DeviceFactoryResult> {
    const workerPath = config.workerPath || this.getWorkerPath(config.deviceType);
    
    // Creating device worker
    
    try {
      // Create worker
      const worker = new Worker(workerPath, { type: 'module' });
      
      // Wrap with Comlink for type-safe RPC
      const device = Comlink.wrap<IHandheldDevice>(worker);
      
      // Return both worker and device proxy
      return {
        worker,
        device: device as unknown as WorkerProxy<unknown>
      };
      
    } catch (error) {
      console.error(`Failed to create ${config.deviceType} worker:`, error);
      throw error;
    }
  }
  
  /**
   * Get the worker path for a device type
   */
  private static getWorkerPath(deviceType: DeviceType): string {
    // Check if we're in development or production
    const isDevelopment = import.meta.env?.DEV || process.env.NODE_ENV === 'development';
    
    switch (deviceType) {
      case 'CS108':
        // Use bundled worker from CS108 package dist
        if (isDevelopment) {
          // In development, use the dist path directly
          return '/packages/cs108/dist/cs108-worker.js';
        } else {
          // In production, use public path
          return '/workers/cs108-worker.js';
        }
        
      case 'Zebra':
        // Future implementation
        return '/workers/zebra-worker.js';
        
      case 'Nordic':
        // Future implementation
        return '/workers/nordic-worker.js';
        
      default:
        throw new Error(`Unknown device type: ${deviceType}`);
    }
  }
  
  /**
   * Check if a device type is supported
   */
  static isSupported(deviceType: DeviceType): boolean {
    // Currently only CS108 is implemented
    return deviceType === 'CS108';
  }
  
  /**
   * Get list of supported device types
   */
  static getSupportedDevices(): DeviceType[] {
    return ['CS108'];
  }
}