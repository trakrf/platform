#!/usr/bin/env node

/**
 * Validate WebSocket URL format for BLE bridge URLs
 * Node.js compatible version
 */

/**
 * Validates that a URL is a valid WebSocket URL
 * @param {string} url - The URL to validate
 * @returns {Object} Object with isValid boolean and optional error message
 */
export function validateWebSocketUrl(url) {
  if (!url) {
    return { isValid: false, error: 'URL is required' };
  }

  try {
    const parsed = new URL(url);
    
    // Check protocol
    if (parsed.protocol !== 'ws:' && parsed.protocol !== 'wss:') {
      return { 
        isValid: false, 
        error: `Invalid protocol '${parsed.protocol}'. WebSocket URLs must start with 'ws://' or 'wss://'` 
      };
    }
    
    // Check hostname
    if (!parsed.hostname) {
      return { isValid: false, error: 'URL must include a hostname' };
    }
    
    // Validate hostname format
    const hostnameRegex = /^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/;
    const ipRegex = /^(\d{1,3}\.){3}\d{1,3}$/;
    
    if (parsed.hostname !== 'localhost' && 
        !hostnameRegex.test(parsed.hostname) && 
        !ipRegex.test(parsed.hostname)) {
      return { isValid: false, error: `Invalid hostname '${parsed.hostname}'` };
    }
    
    // Check port (if specified)
    if (parsed.port) {
      const port = parseInt(parsed.port);
      if (isNaN(port) || port < 1 || port > 65535) {
        return { isValid: false, error: `Invalid port '${parsed.port}'. Port must be between 1 and 65535` };
      }
    }
    
    return { isValid: true };
  } catch (e) {
    return { isValid: false, error: `Invalid URL format: ${e.message}` };
  }
}

/**
 * Validates BLE environment variables
 * @param {Object} env - Environment variables object
 * @returns {Object} Object with isValid boolean and error messages array
 */
export function validateBleEnvironment(env) {
  const errors = [];
  
  // Validate WebSocket URL (our app var)
  const wsUrl = env.VITE_BLE_BRIDGE_URL;
  if (wsUrl) {
    const wsValidation = validateWebSocketUrl(wsUrl);
    if (!wsValidation.isValid) {
      errors.push(`VITE_BLE_BRIDGE_URL: ${wsValidation.error}`);
    }
  }
  
  // Validate UUID formats if provided (our app vars)
  const uuidRegex = /^[0-9a-fA-F]{4}$/;
  
  const serviceUuid = env.VITE_BLE_SERVICE_UUID;
  if (serviceUuid && !uuidRegex.test(serviceUuid)) {
    errors.push(`VITE_BLE_SERVICE_UUID: Invalid format '${serviceUuid}'. Must be 4 hex characters (e.g., '9800')`);
  }
  
  const writeUuid = env.VITE_BLE_WRITE_UUID;
  if (writeUuid && !uuidRegex.test(writeUuid)) {
    errors.push(`VITE_BLE_WRITE_UUID: Invalid format '${writeUuid}'. Must be 4 hex characters (e.g., '9900')`);
  }
  
  const notifyUuid = env.VITE_BLE_NOTIFY_UUID;
  if (notifyUuid && !uuidRegex.test(notifyUuid)) {
    errors.push(`VITE_BLE_NOTIFY_UUID: Invalid format '${notifyUuid}'. Must be 4 hex characters (e.g., '9901')`);
  }
  
  return {
    isValid: errors.length === 0,
    errors
  };
}