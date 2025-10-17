/**
 * System-specific types
 */

/**
 * Battery status data
 */
export interface BatteryData {
  voltage: number;      // millivolts
  percentage: number;   // 0-100
  timestamp: number;
}

/**
 * Error notification data
 */
export interface ErrorData {
  code: number;
  message: string;
  module?: number;
  timestamp: number;
}