/**
 * RSSI to Frequency Conversion for Web Audio
 *
 * Maps RSSI signal strength to audio frequency.
 * Range: 300Hz (weak) to 1500Hz (strong)
 * Optimized for older users and noisy environments.
 */

// Frequency range constants
export const MIN_FREQUENCY = 300; // Hz at -100 dBm
export const MAX_FREQUENCY = 1500; // Hz at -30 dBm
export const NO_SIGNAL_FREQUENCY = 200; // Hz for "no signal" tick (lower, distinct)

// RSSI thresholds (match rssiToInterval.ts)
const MIN_RSSI = -100;
const MAX_RSSI = -30;

/**
 * Convert RSSI to frequency using linear mapping.
 * Linear feels more natural for pitch than exponential.
 */
export function rssiToFrequency(rssi: number): number {
  if (rssi >= MAX_RSSI) return MAX_FREQUENCY;
  if (rssi < MIN_RSSI) return MIN_FREQUENCY;

  // Linear interpolation
  const normalized = (rssi - MIN_RSSI) / (MAX_RSSI - MIN_RSSI);
  return MIN_FREQUENCY + (MAX_FREQUENCY - MIN_FREQUENCY) * normalized;
}
