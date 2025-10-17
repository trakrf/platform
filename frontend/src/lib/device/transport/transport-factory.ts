/**
 * Transport Factory
 * Creates appropriate transport based on environment and configuration
 */

import type { Transport } from './Transport';
import { CS108BLETransport, type CS108BLETransportConfig } from './cs108-ble-transport';
import { BridgeTransport, type BridgeTransportConfig } from './bridge-transport';
import { MockTransport, type MockTransportConfig } from './mock-transport';

export type TransportMode = 'auto' | 'ble' | 'bridge' | 'mock';

export interface TransportFactoryConfig {
  mode?: TransportMode;
  ble?: CS108BLETransportConfig;
  bridge?: BridgeTransportConfig;
  mock?: MockTransportConfig;
}

export class TransportFactory {
  /**
   * Create a transport instance based on configuration
   */
  static create(config: TransportFactoryConfig = {}): Transport {
    const mode = config.mode || this.detectMode();
    
    // Creating transport in selected mode
    
    switch (mode) {
      case 'ble':
        return new CS108BLETransport(config.ble);
        
      case 'bridge':
        return new BridgeTransport(config.bridge);
        
      case 'mock':
        return new MockTransport(config.mock);
        
      case 'auto':
      default:
        return this.createAutoTransport(config);
    }
  }
  
  /**
   * Detect the best transport mode based on environment
   */
  private static detectMode(): TransportMode {
    // Check environment variables
    if (typeof process !== 'undefined' && process.env) {
      if (process.env.TRANSPORT_MODE) {
        return process.env.TRANSPORT_MODE as TransportMode;
      }
      
      if (process.env.NODE_ENV === 'test') {
        return 'mock';
      }
    }
    
    // Check if running in test environment
    if (typeof window !== 'undefined' && (window as unknown as { __TEST_MODE__?: boolean }).__TEST_MODE__) {
      return 'mock';
    }
    
    // Check if bridge server is configured
    if (typeof window !== 'undefined' && (window as unknown as { __BRIDGE_URL__?: string }).__BRIDGE_URL__) {
      return 'bridge';
    }
    
    // Check for Web Bluetooth support
    if (typeof navigator !== 'undefined' && 'bluetooth' in navigator) {
      return 'ble';
    }
    
    // Default to mock if no real transport available
    console.warn('No suitable transport detected, falling back to mock');
    return 'mock';
  }
  
  /**
   * Create transport with automatic fallback
   */
  private static createAutoTransport(config: TransportFactoryConfig): Transport {
    // Try BLE first if supported
    if (typeof navigator !== 'undefined' && 'bluetooth' in navigator) {
      // Web Bluetooth detected, using CS108 BLE transport
      return new CS108BLETransport(config.ble);
    }
    
    // Try bridge if configured
    if (config.bridge?.url) {
      // Bridge URL configured, using bridge transport
      return new BridgeTransport(config.bridge);
    }
    
    // Fall back to mock
    // Using mock transport for testing
    return new MockTransport(config.mock);
  }
  
  /**
   * Check if a specific transport mode is available
   */
  static isAvailable(mode: TransportMode): boolean {
    switch (mode) {
      case 'ble':
        return typeof navigator !== 'undefined' && 'bluetooth' in navigator;
        
      case 'bridge':
        return true; // Bridge can always be attempted
        
      case 'mock':
        return true; // Mock is always available
        
      case 'auto':
        return true;
        
      default:
        return false;
    }
  }
}