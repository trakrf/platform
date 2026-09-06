import fs from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const data = JSON.parse(fs.readFileSync(join(__dirname, 'full-packet-cap.json'), 'utf8'));

// Get all inventory (0x8100) packets with sequence numbers
const inventoryPackets = data
  .filter(p => p.type === 'notification' && p.data.includes('81 00'))
  .map(p => {
    const bytes = p.data.split(' ');

    // Extract header fields
    const sequence = parseInt(bytes[6], 16);  // Reserve field = sequence number
    const eventCode = bytes[8] + bytes[9];

    // Extract payload (skip 10-byte header)
    const payload = bytes.slice(10).map(h => '0x' + h);

    return {
      sequence,
      eventCode,
      payload: '[' + payload.join(', ') + ']',
      fullPacket: bytes.join(' ')
    };
  });

// Group by sequence to see patterns
const bySequence = {};
inventoryPackets.forEach(p => {
  if (!bySequence[p.sequence]) {
    bySequence[p.sequence] = [];
  }
  bySequence[p.sequence].push(p);
});

// Show sequence analysis
console.log('Inventory Packet Sequence Analysis:');
console.log(`Total packets: ${inventoryPackets.length}`);
console.log(`Unique sequences: ${Object.keys(bySequence).length}`);
console.log(`Sequence range: ${Math.min(...Object.keys(bySequence).map(Number))} - ${Math.max(...Object.keys(bySequence).map(Number))}`);

// Check for gaps in sequence
const sequences = Object.keys(bySequence).map(Number).sort((a, b) => a - b);
const gaps = [];
for (let i = 1; i < sequences.length; i++) {
  const gap = sequences[i] - sequences[i-1];
  if (gap > 1) {
    gaps.push({ from: sequences[i-1], to: sequences[i], gap });
  }
}

if (gaps.length > 0) {
  console.log('\nSequence gaps found:');
  gaps.slice(0, 5).forEach(g => {
    console.log(`  Gap: ${g.from} -> ${g.to} (missing ${g.gap - 1} sequences)`);
  });
}

// Create enhanced JSON5 with sequence info
const json5Content = `// CS108 Inventory Packets with Sequence Numbers
// Extracted from real CS108 capture with packet ordering preserved
{
  description: "Real CS108 inventory packets with sequence tracking",
  stats: {
    total: ${inventoryPackets.length},
    uniqueSequences: ${Object.keys(bySequence).length},
    minSequence: ${Math.min(...sequences)},
    maxSequence: ${Math.max(...sequences)},
    gaps: ${gaps.length}
  },

  // Packets with sequence numbers for order tracking
  packets: [
${inventoryPackets.slice(0, 100).map(p =>
    `    { seq: ${p.sequence}, payload: ${p.payload} }`
  ).join(',\n')}
  ]
}`;

fs.writeFileSync(join(__dirname, 'inventory-with-sequence.json5'), json5Content);

console.log('\nCreated inventory-with-sequence.json5');
console.log('First 5 packets with sequences:');
inventoryPackets.slice(0, 5).forEach(p => {
  console.log(`  Seq ${p.sequence}: ${p.payload.substring(0, 30)}...`);
});