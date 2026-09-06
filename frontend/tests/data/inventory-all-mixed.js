import fs from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const data = JSON.parse(fs.readFileSync(join(__dirname, 'full-packet-cap.json'), 'utf8'));

// Get ALL inventory payloads in the exact order they appear
const allPayloads = data
  .filter(p => p.type === 'notification' && p.data.includes('81 00'))
  .map(p => {
    const bytes = p.data.split(' ');
    const modeByteHex = bytes[10];
    const payload = bytes.slice(10).map(h => '0x' + h);
    return {
      mode: modeByteHex,
      payload: '[' + payload.join(', ') + ']'
    };
  });

// Count by type for stats
const stats = {
  '03': 0, // normal
  '04': 0, // compact
  '02': 0, // status
  '70': 0  // keepalive
};

allPayloads.forEach(p => {
  if (stats[p.mode] !== undefined) stats[p.mode]++;
});

// Create the mixed chaos JSON5
const mixedJson5 = `// CS108 Mixed Inventory Payloads - The Ultimate Parser Test!
// All 474 packets in their original order - normal, compact, status, keepalive all mixed
// A real parser should handle this chaos gracefully
{
  description: "Real CS108 capture with all packet types intermixed",
  stats: {
    normal: ${stats['03']},     // 0x03 - Normal mode with 2-byte RSSI
    compact: ${stats['04']},    // 0x04 - Compact mode with 1-byte RSSI
    status: ${stats['02']},     // 0x02 - Status/control packets
    keepalive: ${stats['70']},  // 0x70 - Keepalive packets
    total: ${allPayloads.length}
  },

  // The gauntlet - your parser must survive this!
  payloads: [
${allPayloads.map(p => '    ' + p.payload).join(',\n')}
  ]
}`;

fs.writeFileSync(join(__dirname, 'inventory-all-mixed.json5'), mixedJson5);

console.log('Created inventory-all-mixed.json5 - The Ultimate Parser Test!');
console.log(`${allPayloads.length} packets of pure chaos:`);
console.log(`  - Normal (0x03): ${stats['03']} packets`);
console.log(`  - Compact (0x04): ${stats['04']} packets`);
console.log(`  - Status (0x02): ${stats['02']} packets`);
console.log(`  - Keepalive (0x70): ${stats['70']} packets`);
console.log('\nMay the parser gods have mercy on your code! 🙏');