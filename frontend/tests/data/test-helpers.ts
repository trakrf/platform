/**
 * Test helpers for consistent RFID tag testing
 * Uses real packet captures from CS108 hardware
 */

// `resolveJsonModule` is on, so this import typechecks and the
// `@ts-expect-error` that used to sit here suppressed nothing — it only became
// visible as an unused directive once `tests/**` entered the typecheck.
import testTagsData from './test-tags.json' assert { type: 'json' };

export const TEST_TAGS = testTagsData.tags;

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
  packet[2] = 0x20; // length placeholder
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

// Export sample packets from our captures
export const REAL_PACKETS = {
  compact: testTagsData.compactModePackets,
  normal: testTagsData.normalModePackets
};