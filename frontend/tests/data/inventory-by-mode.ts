/**
 * Mode-specific CS108 inventory payloads
 */
import normalModeData from './inventory-normal-mode.json5';
import compactModeData from './inventory-compact-mode.json5';

export const NORMAL_MODE_PAYLOADS = normalModeData.payloads.map(
  (p: number[]) => new Uint8Array(p)
);

export const COMPACT_MODE_PAYLOADS = compactModeData.payloads.map(
  (p: number[]) => new Uint8Array(p)
);

export function getNormalModePayloads(count?: number): Uint8Array[] {
  return count ? NORMAL_MODE_PAYLOADS.slice(0, count) : NORMAL_MODE_PAYLOADS;
}

export function getCompactModePayloads(count?: number): Uint8Array[] {
  return count ? COMPACT_MODE_PAYLOADS.slice(0, count) : COMPACT_MODE_PAYLOADS;
}

// For locate mode testing - normal mode is better as it has 2-byte RSSI
export const LOCATE_TEST_PAYLOADS = NORMAL_MODE_PAYLOADS;
