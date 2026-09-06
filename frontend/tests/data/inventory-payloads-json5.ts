/**
 * Real CS108 inventory payloads from JSON5
 * Import this for strongly-typed payload data
 */
import payloadData from './inventory-payloads.json5';

export const INVENTORY_PAYLOADS = payloadData.payloads.map(
  (payload: number[]) => new Uint8Array(payload)
);

export const PAYLOAD_METADATA = payloadData.metadata;
export const COMPACT_STRUCTURE = payloadData.compactStructure;

export function getPayloads(count?: number): Uint8Array[] {
  return count ? INVENTORY_PAYLOADS.slice(0, count) : INVENTORY_PAYLOADS;
}

export function getCompactPayloads(): Uint8Array[] {
  return INVENTORY_PAYLOADS.filter(p => p[0] === 0x04);
}

export function getStatusPayloads(): Uint8Array[] {
  return INVENTORY_PAYLOADS.filter(p => p[0] === 0x02);
}

export function getKeepalivePayloads(): Uint8Array[] {
  return INVENTORY_PAYLOADS.filter(p => p[0] === 0x70);
}