/**
 * RSSI to Interval Conversion Utilities
 */

export function rssiToBeepInterval(rssi: number): number {
  // Convert RSSI to beep interval in milliseconds
  // Stronger signal (higher RSSI) = faster beeping
  if (rssi >= -30) return 100; // Very strong signal
  if (rssi >= -50) return 250; // Strong signal
  if (rssi >= -70) return 500; // Medium signal
  if (rssi >= -90) return 1000; // Weak signal
  return 2000; // Very weak signal
}

export function convertRSSIToInterval(rssi: number): number {
  return rssiToBeepInterval(rssi);
}