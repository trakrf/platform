import fs from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const data = JSON.parse(fs.readFileSync(join(__dirname, 'full-packet-cap.json'), 'utf8'));

// Get all 0x8002 packets (firmware commands)
// These can be both commands (type:"command") and responses (type:"notification")
const firmwarePackets = data
  .filter(p => {
    const bytes = p.data.split(' ');
    // Check event code at position 8-9
    return bytes[8] === '80' && bytes[9] === '02';
  })
  .map(p => {
    const bytes = p.data.split(' ');
    // Skip the 10-byte header to get just the payload
    const payload = bytes.slice(10).map(h => '0x' + h);
    // Get the full packet for reference
    const fullPacket = bytes.join(' ');

    // Determine direction based on prefix
    const prefix = bytes[0] + bytes[1];
    const direction = prefix === 'a7b3' ? 'command' : 'response';

    // Parse some info from the payload
    const eventCode = bytes[8] + bytes[9]; // Should be 80 02
    const payloadLength = bytes.length - 10;

    return {
      fullPacket,
      direction,
      type: p.type,
      eventCode,
      payloadLength,
      payload: '[' + payload.join(', ') + ']'
    };
  });

// Deduplicate by payload
const uniquePayloads = {};
firmwarePackets.forEach(p => {
  if (!uniquePayloads[p.payload]) {
    uniquePayloads[p.payload] = p;
  }
});

const unique = Object.values(uniquePayloads);
const commands = unique.filter(p => p.direction === 'command');
const responses = unique.filter(p => p.direction === 'response');

// Create the JSON5 file
const json5Content = `// CS108 Firmware Commands and Responses (0x8002)
// Extracted from real CS108 capture
{
  description: "Real CS108 firmware command (0x8002) packets captured during operation",
  stats: {
    total: ${firmwarePackets.length},
    unique: ${unique.length},
    commands: ${commands.length},
    responses: ${responses.length}
  },

  // Commands sent to device (prefix 0xA7B3)
  commands: [
${commands.map(p => '    ' + p.payload).join(',\n')}
  ],

  // Responses from device (prefix 0xB3A7)
  responses: [
${responses.map(p => '    ' + p.payload).join(',\n')}
  ],

  // Full packets with headers for reference
  fullPackets: {
    commands: [
${commands.map(p => `      // ${p.fullPacket}\n      ${p.payload}`).join(',\n')}
    ],
    responses: [
${responses.map(p => `      // ${p.fullPacket}\n      ${p.payload}`).join(',\n')}
    ]
  }
}`;

fs.writeFileSync(join(__dirname, 'firmware-commands.json5'), json5Content);

console.log(`Created firmware-commands.json5 with ${unique.length} unique firmware packets`);
console.log(`  Commands sent: ${commands.length}`);
console.log(`  Responses received: ${responses.length}`);

if (commands.length > 0) {
  console.log('\nSample commands we sent to reader:');
  commands.slice(0, 3).forEach(p => {
    console.log(`  → ${p.fullPacket}`);
  });
}

if (responses.length > 0) {
  console.log('\nSample responses from reader:');
  responses.slice(0, 3).forEach(p => {
    console.log(`  ← ${p.fullPacket}`);
  });
}