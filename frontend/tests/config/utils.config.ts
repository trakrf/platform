/**
 * General Test Utilities
 * 
 * Shared utility functions for testing that aren't specific to any particular device or connection method.
 */

/**
 * Convert byte array to hex string for logging
 * General utility for debugging BLE/serial communications
 * @param bytes Byte array to convert
 * @returns Hex string representation (e.g., "0xA7 0xB3 0x02")
 */
export function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map(b => '0x' + b.toString(16).padStart(2, '0').toUpperCase())
    .join(' ');
}

/**
 * Convert hex string to byte array
 * Accepts formats: "A7B3", "A7 B3", "0xA7 0xB3", "0xA7,0xB3"
 * @param hex Hex string to convert
 * @returns Uint8Array of bytes
 */
export function hexToBytes(hex: string): Uint8Array {
  // Remove 0x prefix, spaces, commas
  const cleaned = hex.replace(/0x/gi, '').replace(/[\s,]/g, '');
  
  // Ensure even number of characters
  if (cleaned.length % 2 !== 0) {
    throw new Error(`Invalid hex string: ${hex} (must have even number of characters)`);
  }
  
  const bytes = new Uint8Array(cleaned.length / 2);
  for (let i = 0; i < cleaned.length; i += 2) {
    bytes[i / 2] = parseInt(cleaned.substr(i, 2), 16);
  }
  
  return bytes;
}

/**
 * Compare two byte arrays for equality
 * @param a First byte array
 * @param b Second byte array
 * @returns True if arrays are equal
 */
export function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false;
  }
  return true;
}

/**
 * Create a delay/sleep promise
 * @param ms Milliseconds to wait
 * @returns Promise that resolves after delay
 */
export function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}