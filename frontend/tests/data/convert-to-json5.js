import fs from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const data = JSON.parse(fs.readFileSync(join(__dirname, 'full-packet-cap.json'), 'utf8'));

// Filter for 0x8100 inventory notifications and convert to hex arrays
const inventoryPayloads = data
  .filter(p => p.type === 'notification' && p.data.includes('81 00'))
  .map(p => {
    // Skip first 10 bytes (header + event code)
    const hexBytes = p.data.split(' ').slice(10).map(h => '0x' + h);
    return '  [' + hexBytes.join(', ') + ']';
  });

// Create JSON5 with hex literals
const json5Content = `// CS108 Inventory Notification (0x8100) Payloads
// Generated from full-packet-cap.json
// Already stripped of 10-byte header (8 header + 2 event code)
// Order preserved - DO NOT SORT as tags may span packets
{
  // Payload format types observed:
  // 0x02: Status/control messages (no tag data)
  // 0x70: Keepalive/status (8 bytes)
  // 0x04: Compact mode tag data (variable length with tags)

  payloads: [
${inventoryPayloads.join(',\n')}
  ],

  // Metadata about the capture
  metadata: {
    totalPayloads: ${inventoryPayloads.length},
    captureDate: "2025-07-05",
    readerMode: "compact",
    description: "CS108 test mode capture with zero EPCs"
  },

  // Compact mode structure (type 0x04 payloads)
  compactStructure: {
    header: {
      version: [0x00, 0x01],      // Bytes 0-1
      type: [0x02, 0x03],          // Bytes 2-3: 0x8005 little-endian
      length: [0x04, 0x07],        // Bytes 4-7: payload length
      metadata: [0x08, 0x0F]       // Bytes 8-15: antenna, etc
    },
    tagData: {
      startOffset: 0x14,           // Tags start at byte 20
      tagSize: 0x10,              // 16 bytes per tag (estimated)
      structure: "RSSI(1) + Antenna(1) + PC(2) + EPC(12)"
    }
  }
}`;

// Write JSON5 file
fs.writeFileSync(join(__dirname, 'inventory-payloads.json5'), json5Content);
console.log(`Created inventory-payloads.json5 with ${inventoryPayloads.length} payloads`);

// Also create a TypeScript module that imports the JSON5
const tsModule = `/**
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
}`;

fs.writeFileSync(join(__dirname, 'inventory-payloads-json5.ts'), tsModule);
console.log('Created TypeScript module for JSON5 import');