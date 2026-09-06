/**
 * Real test tags from CS108 inventory captures
 * Source: inventory_2025-07-05.csv
 */

export const TEST_TAGS = [
  {
    epc: "E2801160600002084D9F34E9",
    pc: 0x3000,
    rssi: -45,
    description: "Real tag from inventory scan"
  },
  {
    epc: "E2801160600002084D9F3439",
    pc: 0x3000,
    rssi: -48,
    description: "Real tag from inventory scan"
  },
  {
    epc: "E2801160600002084D9EF4BA",
    pc: 0x3000,
    rssi: -65,
    description: "Real tag from inventory scan"
  },
  {
    epc: "E2801160600002084D9F4A9A",
    pc: 0x3000,
    rssi: -72,
    description: "Real tag from inventory scan"
  },
  {
    epc: "000000000000000000000009",
    pc: 0x3000,
    rssi: -28,
    description: "Test tag 9"
  },
  {
    epc: "E2006B060000000000000000",
    pc: 0x3000,
    rssi: -67,
    description: "EPC with different manufacturer prefix"
  },
  {
    epc: "E2801160600002084D9F4AF9",
    pc: 0x3000,
    rssi: -33,
    description: "Real tag from inventory scan"
  },
  {
    epc: "E2801190A502006016440AE6",
    pc: 0x3000,
    rssi: -60,
    description: "Different tag format"
  },
  {
    epc: "000000000000000000000012",
    pc: 0x3000,
    rssi: -32,
    description: "Test tag 18 (decimal)"
  },
  {
    epc: "E280116060000208D794B184",
    pc: 0x3000,
    rssi: -59,
    description: "Real tag from inventory scan"
  }
];

/**
 * Convert hex string to Uint8Array
 */
export function hexToBytes(hex: string): Uint8Array {
  const cleanHex = hex.replace(/\s+/g, '');
  const bytes = new Uint8Array(cleanHex.length / 2);
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(cleanHex.substr(i * 2, 2), 16);
  }
  return bytes;
}

/**
 * Build a compact mode inventory packet with given tags
 */
export function buildCompactInventoryPacket(tags: Array<{ pc: number; epc: string; rssi: number }>): Uint8Array {
  // Calculate payload size
  let payloadSize = 0;
  for (const tag of tags) {
    const epcBytes = tag.epc.length / 2;
    payloadSize += 2 + epcBytes + 1; // PC (2) + EPC + RSSI (1)
  }

  // Build packet
  const packet = new Uint8Array(18 + payloadSize); // 10 CS108 + 8 protocol + payload

  // CS108 header
  packet[0] = 0xA7;
  packet[1] = 0xB3;
  packet[2] = (8 + payloadSize) & 0xFF; // length
  packet[3] = 0xC2;
  packet[4] = 0x01; // sequence
  packet[5] = 0x9E;
  packet[6] = 0x00; // CRC low
  packet[7] = 0x00; // CRC high

  // Event code
  packet[8] = 0x81;
  packet[9] = 0x00;

  // Compact mode protocol header
  packet[10] = 0x04; // version
  packet[11] = 0x00; // flags
  packet[12] = 0x05; // type low (0x8005)
  packet[13] = 0x80; // type high
  packet[14] = payloadSize & 0xFF; // payload length low
  packet[15] = (payloadSize >> 8) & 0xFF; // payload length high
  packet[16] = 0x06; // antenna port
  packet[17] = 0x00; // reserved

  // Add tags
  let offset = 18;
  for (const tag of tags) {
    // PC
    packet[offset++] = tag.pc & 0xFF;
    packet[offset++] = (tag.pc >> 8) & 0xFF;

    // EPC
    const epcHex = tag.epc.replace(/\s+/g, '');
    for (let i = 0; i < epcHex.length; i += 2) {
      packet[offset++] = parseInt(epcHex.substr(i, 2), 16);
    }

    // RSSI (convert from dBm to byte: -80 dBm = 100 * 0.8 = 80 = 0x50)
    packet[offset++] = Math.round(Math.abs(tag.rssi) / 0.8);
  }

  return packet;
}

/**
 * Build a normal mode inventory packet with given tags
 */
export function buildNormalInventoryPacket(tags: Array<{ pc: number; epc: string; rssi: number }>): Uint8Array {
  // Calculate payload size in words
  const metadataWords = 3; // 12 bytes of metadata = 3 words
  let tagDataBytes = 0;
  for (const tag of tags) {
    const epcBytes = tag.epc.length / 2;
    tagDataBytes += 2 + epcBytes + 2; // PC (2) + EPC + RSSI (2 for normal mode)
  }
  const tagDataWords = Math.ceil(tagDataBytes / 4);
  const totalWords = 2 + metadataWords + tagDataWords; // 2 for protocol header

  const totalBytes = totalWords * 4;
  const packet = new Uint8Array(10 + totalBytes); // 10 CS108 + protocol packet

  // CS108 header
  packet[0] = 0xA7;
  packet[1] = 0xB3;
  packet[2] = (totalBytes + 2) & 0xFF; // length (includes event code)
  packet[3] = 0xC2;
  packet[4] = 0x01; // sequence
  packet[5] = 0x9E;
  packet[6] = 0x00; // CRC low
  packet[7] = 0x00; // CRC high

  // Event code
  packet[8] = 0x81;
  packet[9] = 0x00;

  // Normal mode protocol header
  packet[10] = 0x03; // version
  packet[11] = 0x00; // flags
  packet[12] = 0x05; // type low (0x8005)
  packet[13] = 0x80; // type high
  packet[14] = totalWords & 0xFF; // length in words low
  packet[15] = (totalWords >> 8) & 0xFF; // length in words high
  packet[16] = 0x00; // reserved
  packet[17] = 0x00; // reserved

  // Metadata
  packet[18] = 0x00; // ms_ctr
  packet[19] = 0x00;
  packet[20] = 0x00;
  packet[21] = 0x00;
  packet[22] = 0x48; // wb_rssi
  packet[23] = 0x48; // nb_rssi
  packet[24] = 0x01; // ana_ctrl low
  packet[25] = 0x00; // ana_ctrl high
  packet[26] = 0x01; // antenna port low
  packet[27] = 0x00; // antenna port high

  // Add tags
  let offset = 28;
  for (const tag of tags) {
    // PC
    packet[offset++] = tag.pc & 0xFF;
    packet[offset++] = (tag.pc >> 8) & 0xFF;

    // EPC
    const epcHex = tag.epc.replace(/\s+/g, '');
    for (let i = 0; i < epcHex.length; i += 2) {
      packet[offset++] = parseInt(epcHex.substr(i, 2), 16);
    }

    // RSSI in normal mode (16-bit signed, in tenths of dBm)
    const rssiTenths = tag.rssi * 10;
    packet[offset++] = rssiTenths & 0xFF;
    packet[offset++] = (rssiTenths >> 8) & 0xFF;
  }

  // Pad to word boundary if needed
  while (offset < packet.length) {
    packet[offset++] = 0x00;
  }

  return packet;
}

// Real packet examples from captures
export const REAL_PACKETS = {
  compact: {
    singleTag: {
      description: "Single tag in compact mode (0x04 version)",
      hex: "a7 b3 19 c2 01 9e 00 00 81 00 04 00 05 80 0f 00 06 00 30 00 E2 80 11 60 60 00 02 08 4D 9F 34 E9 39"
    }
  },
  normal: {
    singleTag: {
      description: "Single tag in normal mode (0x03 version)",
      hex: "a7 b3 2a c2 01 9e 00 00 81 00 03 00 05 80 08 00 00 00 00 00 00 00 48 48 01 00 01 00 30 00 E2 80 11 60 60 00 02 08 4D 9F 34 E9 D3 FE"
    }
  }
};