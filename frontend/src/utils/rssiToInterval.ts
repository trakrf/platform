/**
 * RSSI to Interval Conversion Utilities
 *
 * Uses exponential curve for smooth metal-detector effect.
 * Transitions to full continuous tone at -30 dBm and above.
 */

// Special value indicating continuous tone (no interval)
export const CONTINUOUS_TONE = 0;

export function rssiToBeepInterval(rssi: number): number {
  // Full continuous tone at -30 dBm and above (very close to tag)
  if (rssi >= -30) return CONTINUOUS_TONE;

  // Below -100: very slow beeps (out of practical range)
  if (rssi < -100) return 1500;

  // Exponential curve from -100 to -30 dBm
  // Gives smooth metal-detector feel with accelerating beeps
  const minRssi = -100;
  const maxRssi = -30;
  const maxInterval = 1000; // ms at weakest signal (-100 dBm)
  const minInterval = 40;   // ms just before continuous (-30 dBm)

  // Normalize RSSI to 0-1 range (0 = weak, 1 = strong)
  const normalized = (rssi - minRssi) / (maxRssi - minRssi);

  // Exponential decay: beeps accelerate as signal strengthens
  // e^(-4 * normalized) gives nice curve from 1.0 to 0.018
  const expFactor = Math.exp(-4 * normalized);

  // Scale to interval range
  const interval = minInterval + (maxInterval - minInterval) * expFactor;

  return Math.round(interval);
}

export function convertRSSIToInterval(rssi: number): number {
  return rssiToBeepInterval(rssi);
}