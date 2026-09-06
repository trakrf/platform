import fs from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const data = JSON.parse(fs.readFileSync(join(__dirname, 'full-packet-cap.json'), 'utf8'));

// Separate payloads by mode
const payloadsByMode = {
  normal: [],    // 0x03
  compact: [],   // 0x04
  status: [],    // 0x02
  keepalive: []  // 0x70
};

data
  .filter(p => p.type === 'notification' && p.data.includes('81 00'))
  .forEach(p => {
    const bytes = p.data.split(' ');
    const modeByteHex = bytes[10]; // First byte after 10-byte header
    const payload = bytes.slice(10).map(h => '0x' + h);

    switch (modeByteHex) {
      case '03':
        payloadsByMode.normal.push(payload);
        break;
      case '04':
        payloadsByMode.compact.push(payload);
        break;
      case '02':
        payloadsByMode.status.push(payload);
        break;
      case '70':
        payloadsByMode.keepalive.push(payload);
        break;
    }
  });

// Create separate JSON5 files for each mode
const normalModeJson5 = `// CS108 Normal Mode (0x03) Inventory Payloads
// ${payloadsByMode.normal.length} packets captured
// Format: PC(2 bytes) + EPC(variable) + RSSI(2 bytes signed)
{
  mode: "normal",
  totalPackets: ${payloadsByMode.normal.length},

  // First 50 normal mode payloads for testing
  payloads: [
${payloadsByMode.normal.slice(0, 50).map(p => '    [' + p.join(', ') + ']').join(',\n')}
  ]
}`;

const compactModeJson5 = `// CS108 Compact Mode (0x04) Inventory Payloads
// ${payloadsByMode.compact.length} packets captured
// Format: PC(2 bytes) + EPC(variable) + RSSI(1 byte)
{
  mode: "compact",
  totalPackets: ${payloadsByMode.compact.length},

  // All compact mode payloads for testing
  payloads: [
${payloadsByMode.compact.map(p => '    [' + p.join(', ') + ']').join(',\n')}
  ]
}`;

// Write mode-specific files
fs.writeFileSync(join(__dirname, 'inventory-normal-mode.json5'), normalModeJson5);
fs.writeFileSync(join(__dirname, 'inventory-compact-mode.json5'), compactModeJson5);

// Create combined statistics
const stats = {
  normal: payloadsByMode.normal.length,
  compact: payloadsByMode.compact.length,
  status: payloadsByMode.status.length,
  keepalive: payloadsByMode.keepalive.length,
  total: Object.values(payloadsByMode).reduce((sum, arr) => sum + arr.length, 0)
};

console.log('Inventory payload breakdown:');
console.log(`  Normal mode (0x03):    ${stats.normal} packets`);
console.log(`  Compact mode (0x04):   ${stats.compact} packets`);
console.log(`  Status (0x02):         ${stats.status} packets`);
console.log(`  Keepalive (0x70):      ${stats.keepalive} packets`);
console.log(`  Total:                 ${stats.total} packets`);

console.log('\nCreated:');
console.log('  - inventory-normal-mode.json5 (50 samples)');
console.log('  - inventory-compact-mode.json5 (all samples)');

// Also create a TypeScript module for easy imports
const tsModule = `/**
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
`;

fs.writeFileSync(join(__dirname, 'inventory-by-mode.ts'), tsModule);
console.log('  - inventory-by-mode.ts (TypeScript module)');