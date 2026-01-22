import { describe, it, expect } from 'vitest';
import { rssiToFrequency, MIN_FREQUENCY, MAX_FREQUENCY } from './rssiToFrequency';

describe('rssiToFrequency', () => {
  it('returns MIN_FREQUENCY for very weak signal', () => {
    expect(rssiToFrequency(-100)).toBe(MIN_FREQUENCY);
    expect(rssiToFrequency(-120)).toBe(MIN_FREQUENCY);
  });

  it('returns MAX_FREQUENCY for strong signal', () => {
    expect(rssiToFrequency(-30)).toBe(MAX_FREQUENCY);
    expect(rssiToFrequency(-20)).toBe(MAX_FREQUENCY);
  });

  it('returns intermediate frequency for mid-range signal', () => {
    const midFreq = rssiToFrequency(-65); // Midpoint of -100 to -30
    expect(midFreq).toBeGreaterThan(MIN_FREQUENCY);
    expect(midFreq).toBeLessThan(MAX_FREQUENCY);
    // Linear: should be close to midpoint
    expect(midFreq).toBeCloseTo((MIN_FREQUENCY + MAX_FREQUENCY) / 2, -1);
  });

  it('increases frequency as RSSI increases', () => {
    const weak = rssiToFrequency(-90);
    const medium = rssiToFrequency(-60);
    const strong = rssiToFrequency(-35);

    expect(weak).toBeLessThan(medium);
    expect(medium).toBeLessThan(strong);
  });
});
