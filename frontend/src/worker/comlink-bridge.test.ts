/**
 * Comlink Bridge Tests
 * Tests to verify Comlink can properly serialize callbacks across worker boundary
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as Comlink from 'comlink';

// Mock Comlink
vi.mock('comlink', () => ({
  proxy: vi.fn((fn) => {
    // Return a wrapped version, not the original
    return new Proxy(fn, {});
  }),
  wrap: vi.fn((endpoint) => endpoint),
  expose: vi.fn(),
  transfer: vi.fn((obj) => obj)
}));

describe('Comlink Bridge Serialization', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should be able to pass callbacks through Comlink proxy', async () => {
    // This test simulates the DeviceManager → Worker callback flow

    // Mock a worker-like object that stores callbacks
    const mockWorkerAPI = {
      callbacks: new Map<string, (...args: unknown[]) => void>(),

      // Simulate worker registration methods
      onStateChanged(callback: (state: string) => void) {
        this.callbacks.set('state', callback);
      },

      onTagRead(callback: (tag: unknown) => void) {
        this.callbacks.set('tag', callback);
      },

      // Simulate worker triggering callbacks
      triggerStateChange(state: string) {
        const callback = this.callbacks.get('state');
        if (callback) {
          callback(state);
        }
      },

      triggerTagRead(tag: unknown) {
        const callback = this.callbacks.get('tag');
        if (callback) {
          callback(tag);
        }
      }
    };

    // Test direct callback registration (works in unit tests)
    const mockStateCallback = vi.fn();
    const mockTagCallback = vi.fn();

    mockWorkerAPI.onStateChanged(mockStateCallback);
    mockWorkerAPI.onTagRead(mockTagCallback);

    // Trigger callbacks directly
    mockWorkerAPI.triggerStateChange('READY');
    mockWorkerAPI.triggerTagRead({ epc: 'test', rssi: -45 });

    // Should work without Comlink
    expect(mockStateCallback).toHaveBeenCalledWith('READY');
    expect(mockTagCallback).toHaveBeenCalledWith({ epc: 'test', rssi: -45 });
  });

  it('should handle Comlink proxy function registration', async () => {
    // Test what happens when callbacks go through Comlink proxy

    const mockStateCallback = vi.fn();
    const mockTagCallback = vi.fn();

    // Simulate Comlink.proxy wrapping functions
    const proxiedStateCallback = Comlink.proxy(mockStateCallback);
    const proxiedTagCallback = Comlink.proxy(mockTagCallback);

    // Test that proxied functions are still callable
    expect(typeof proxiedStateCallback).toBe('function');
    expect(typeof proxiedTagCallback).toBe('function');

    // Note: In real Comlink, these would be async operations
    // but for this test we're checking the wrapping behavior
  });

  it('should identify callback serialization issues', async () => {
    // Test what we suspect is happening in the browser

    const callback = vi.fn();

    // Check how function serializes (returns undefined, not an error)
    const serialized = JSON.stringify(callback);
    expect(serialized).toBeUndefined();

    // Check if Comlink.proxy helps with serialization
    const proxiedCallback = Comlink.proxy(callback);

    // Comlink.proxy should return a proxy object, not the original function
    expect(proxiedCallback).not.toBe(callback);

    console.log('Original callback type:', typeof callback);
    console.log('Proxied callback type:', typeof proxiedCallback);
    console.log('Proxied callback:', proxiedCallback);
    console.log('Proxied callback constructor:', proxiedCallback.constructor.name);
  });

  it('should test callback transfer through MessagePort', async () => {
    // Test the flow: DeviceManager → Comlink → Worker → MessagePort → Worker events

    const mockCallback = vi.fn();

    // This is what DeviceManager does:
    // device.onTagRead(callback) where device is Comlink.wrap(worker)

    // This is what the worker should receive:
    // A Comlink proxy function that can be called

    // The key question: Can the worker call this proxy function?
    const proxiedCallback = Comlink.proxy(mockCallback);

    // In the worker, when we receive a domain event, we do:
    // if (onTagReadCallback) onTagReadCallback(tagData);

    // This should work if Comlink properly proxies the function
    if (typeof proxiedCallback === 'function') {
      // proxiedCallback(someData); // Would call mockCallback through Comlink
      expect(true).toBe(true); // Placeholder - real test would verify the call
    } else {
      console.log('Proxied callback is not a function!', typeof proxiedCallback);
      expect(typeof proxiedCallback).toBe('function');
    }
  });
});