/**
 * Common Parser Functions
 * Type-safe parsers for basic data types and common responses
 */

import type { ScalarParser } from '../payload-types.js';

/**
 * Parse single byte as unsigned integer
 */
export const parseUint8 = (data: Uint8Array): number => data[0] || 0;

/**
 * Parse 2 bytes as little-endian unsigned integer
 */
export const parseUint16LE = (data: Uint8Array): number => (data[0] || 0) | ((data[1] || 0) << 8);

/**
 * Parse 2 bytes as big-endian unsigned integer
 */
export const parseUint16BE = (data: Uint8Array): number => ((data[0] || 0) << 8) | (data[1] || 0);

/**
 * Parse 4 bytes as little-endian unsigned integer
 */
export const parseUint32LE = (data: Uint8Array): number =>
  (data[0] || 0) | ((data[1] || 0) << 8) | ((data[2] || 0) << 16) | ((data[3] || 0) << 24);

/**
 * Battery voltage range constants for percentage calculation
 */
const BATTERY_VOLTAGE = {
  MIN: 3000,  // 3.0V - 0%
  MAX: 4200,  // 4.2V - 100%
} as const;

/**
 * Battery percentage parser
 * CS108 reports voltage in millivolts, big-endian
 * We convert to percentage (0-100) for the UI
 */
export const parseBatteryPercentage: ScalarParser = (data: Uint8Array): number => {
  const voltage = parseUint16BE(data);

  // Convert to percentage using defined constants
  const range = BATTERY_VOLTAGE.MAX - BATTERY_VOLTAGE.MIN;
  const offset = voltage - BATTERY_VOLTAGE.MIN;
  const percentage = (offset / range) * 100;

  // Clamp to 0-100 range and round to integer
  const clampedPercentage = Math.round(Math.max(0, Math.min(100, percentage)));

  return clampedPercentage;
};