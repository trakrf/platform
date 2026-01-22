/**
 * Inventory Parser for CS108 RFID Reader
 *
 * Optimized implementation that:
 * - Uses 64KB ring buffer (vs legacy 16MB) for worker thread efficiency
 * - Handles tags spanning multiple CS108 packets seamlessly
 * - Supports compact mode (0x04/0x8005) inventory responses
 * - Processes 1000+ tags/second with minimal memory usage
 *
 * Packet processing flow:
 * 1. Receives raw payload from 0x8100 notifications (RFID firmware response data)
 * 2. Accumulates in ring buffer to handle fragmented protocol packets
 * 3. Parses complete RFID protocol packets (0x8005 for inventory in compact mode)
 * 4. Extracts tag data: PC(2 bytes, big-endian) + EPC(variable) + NB_RSSI(1 byte)
 *
 * Based on CS108 spec Appendix A - Inventory-Response Packet (Compact mode)
 * PC word encoding: Big-endian in protocol packets (verified with real hardware data)
 */

import { RingBuffer, type BufferMetrics } from './ring-buffer';
import { logger } from '../../utils/logger.js';

export interface ParsedTag {
  epc: string;              // Hex string EPC
  rssi: number;             // RSSI value in dBm (narrowband)
  pc: number;               // PC word value
  antennaPort?: number;     // Antenna port number
  timestamp: number;        // When tag was read
  mode?: 'compact' | 'normal'; // Which mode it came from
  phase?: number;           // Phase angle (0-180 degrees) for location smoothing
  wbRssi?: number;          // Wideband RSSI (raw value) for normal mode
}

export interface ParserState {
  buffer: RingBuffer;
  sequenceNumber: number;
  mode: 'compact' | 'normal';
  packetsProcessed: number;
  tagsExtracted: number;
  lastPacketTime: number;
  parseErrors: number;
}

export type InventoryMode = 'compact' | 'normal';

export class InventoryParser {
  private buffer: RingBuffer;
  private sequenceNumber = 0;
  private expectedMode: InventoryMode = 'compact';
  private packetsProcessed = 0;
  private tagsExtracted = 0;
  private lastPacketTime = 0;
  private parseErrors = 0;
  private debug = false;

  constructor(mode: InventoryMode = 'compact', debug = false) {
    // 64KB buffer for efficiency
    this.buffer = new RingBuffer(64 * 1024);
    this.expectedMode = mode;
    this.debug = debug;

    logger.debug(`[InventoryParser] Initialized with 64KB buffer, mode: ${mode}`);
  }

  /**
   * Process inventory payload data from CS108 0x8100 notification
   * Payload contains RFID protocol packets that may span multiple notifications
   *
   * @param payload Raw payload from 0x8100 notification (contains protocol packets)
   * @param sequence Optional sequence number for debugging
   * @returns Array of parsed tags
   */
  processInventoryPayload(payload: Uint8Array, sequence?: number): ParsedTag[] {
    // Track sequence if provided (for debugging multi-packet tags)
    if (sequence !== undefined) {
      if (this.sequenceNumber !== 0 && sequence !== (this.sequenceNumber + 1) % 256) {
        if (this.debug) {
          logger.debug(`[InventoryParser] Sequence jump: ${this.sequenceNumber} → ${sequence}`);
        }
      }
      this.sequenceNumber = sequence;
    }

    // Add payload to buffer - it contains RFID protocol packets
    if (payload.length > 0) {
      if (!this.buffer.dataIn(payload)) {
        logger.error('[InventoryParser] Buffer overflow - clearing');
        this.buffer.clear();
        return [];
      }


      if (this.debug) {
        logger.debug(`[InventoryParser] Added ${payload.length} bytes${sequence !== undefined ? ` (seq: ${sequence})` : ''}`);
      }
    }

    this.packetsProcessed++;
    this.lastPacketTime = Date.now();

    // Parse complete protocol packets from buffer
    return this.parseProtocolPackets();
  }

  /**
   * Parse RFID protocol packets from the ring buffer
   * Handles compact mode (0x04/0x8005) inventory response packets
   */
  private parseProtocolPackets(): ParsedTag[] {
    const tags: ParsedTag[] = [];

    // Process RFID protocol packets
    while (this.buffer.length >= 8) {
      // Peek at packet header (8 bytes minimum)
      const header = this.buffer.dataPreOut(8);
      if (!header) break;

      const pktVer = header[0];
      const pktType = header[2] | (header[3] << 8);

      // Handle inventory response packets
      if (pktVer === 0x04 && pktType === 0x8005) {
        // Compact mode Inventory-Response packet
        // pkt_len at bytes 4-5 is payload length in BYTES (not words!)
        const payloadLen = header[4] | (header[5] << 8);
        const totalPacketLen = 8 + payloadLen; // Header + payload

        if (this.buffer.length < totalPacketLen) {
          // Wait for complete packet
          break;
        }

        // Extract complete packet
        const fullPacket = this.buffer.dataOut(totalPacketLen);
        if (!fullPacket) break;

        // Parse compact mode inventory response
        // Byte 6: Antenna Port #
        // Byte 7: Reserved
        // Bytes 8+: Tag data (PC + EPC + NB_RSSI)
        const antennaPort = fullPacket[6];
        const tagData = fullPacket.subarray(8);

        // Parse tags from the payload
        const packetTags = this.parseCompactTagData(tagData, antennaPort);
        tags.push(...packetTags);

        // Log compact mode tags for debugging
        if (packetTags.length > 0) {
          logger.debug(`[InventoryParser] Compact mode: ${packetTags.length} tags, first RSSI=${packetTags[0].rssi}dBm`);
        }
      } else if (pktVer === 0x03 && pktType === 0x8005) {
        // Normal mode Inventory-Response packet (used in LOCATE mode)
        // pkt_len at bytes 4-5 is length in 4-byte words
        const pktLen = (header[4] | (header[5] << 8)) * 4 + 8;

        if (this.buffer.length < pktLen) {
          // Wait for complete packet
          break;
        }

        // Extract complete packet
        const fullPacket = this.buffer.dataOut(pktLen);
        if (!fullPacket) break;

        // Parse normal mode inventory response
        const tag = this.parseNormalModeTag(fullPacket);
        if (tag) {
          tags.push(tag);
          // Always log normal mode tags for debugging LOCATE
          logger.debug(`[InventoryParser] Normal mode tag: EPC=${tag.epc}, NB_RSSI=${tag.rssi}dBm, WB_RSSI=${tag.wbRssi}dBm`);
        }
      } else if (pktVer === 0x02 || pktVer === 0x01) {
        // Command state packets (Begin: 0x8000, End: 0x8001)
        if (pktType === 0x8000 || pktType === 0x8001) {
          const pktLen = (header[4] | (header[5] << 8)) * 4 + 8; // Length in 4-byte words

          if (this.buffer.length < pktLen) {
            break; // Wait for complete packet
          }

          // Skip command packet
          this.buffer.dataOut(pktLen);

          if (this.debug) {
            const type = pktType === 0x8000 ? 'Command-Begin' : 'Command-End';
            logger.debug(`[InventoryParser] ${type} packet`);
          }
        } else {
          // Unknown packet type, skip one byte
          this.buffer.dataDel(1);
          this.parseErrors++;
        }
      } else {
        // Unknown packet version or corrupted data, skip one byte
        this.buffer.dataDel(1);
        this.parseErrors++;

        if (this.debug) {
          logger.debug(`[InventoryParser] Unknown packet: ver=0x${pktVer.toString(16)}, type=0x${pktType.toString(16).padStart(4, '0')}`);
        }
      }
    }

    return tags;
  }

  /**
   * Parse tag data from compact mode inventory payload
   * Format: PC(2 bytes) + EPC(variable) + NB_RSSI(1 byte)
   *
   * @param data Tag data bytes from inventory response packet
   * @param antennaPort Antenna port number from packet header
   */
  private parseCompactTagData(data: Uint8Array, antennaPort: number): ParsedTag[] {
    const tags: ParsedTag[] = [];
    let offset = 0;

    while (offset + 4 < data.length) { // Minimum: 2 PC + 2 EPC + 1 RSSI
      // Read PC word - The data shows this is BIG-ENDIAN in the protocol packets
      const pc = (data[offset] << 8) | data[offset + 1];

      // Extract EPC length from PC bits 15-11 (mask 0xF800)
      const epcLengthWords = (pc & 0xF800) >> 11;

      // Validate EPC length
      if (epcLengthWords === 0 || epcLengthWords > 31) {
        // Invalid PC, skip one byte and try again
        offset++;
        this.parseErrors++;
        continue;
      }

      const epcLengthBytes = epcLengthWords * 2;
      const tagLength = 2 + epcLengthBytes + 1; // PC + EPC + RSSI

      // Check if we have complete tag data
      if (offset + tagLength > data.length) {
        // Incomplete tag, save for next packet
        const remaining = data.subarray(offset);
        this.buffer.dataIn(remaining);
        break;
      }

      // Extract EPC bytes
      const epcBytes = data.subarray(offset + 2, offset + 2 + epcLengthBytes);

      // Convert EPC to hex string
      const epc = Array.from(epcBytes)
        .map(b => b.toString(16).padStart(2, '0'))
        .join('')
        .toUpperCase();

      // Extract and convert NB_RSSI (Narrowband RSSI)
      const nbRssi = data[offset + 2 + epcLengthBytes];

      // RSSI conversion formula from CS108 spec for compact mode (R2000 chip):
      // Formula: 20 * log10(2^Exponent * (1 + Mantissa / 8))
      // Where: Mantissa = bits 2:0, Exponent = bits 7:3
      // Result is in dBuV (dB microvolts), convert to dBm: dBm = dBuV - 106.98
      const mantissa = nbRssi & 0x07;  // bits 2:0
      const exponent = (nbRssi >> 3) & 0x1F;  // bits 7:3

      // Calculate RSSI in dBuV using the R2000 formula (CS108 always uses R2000)
      const rssiDbuV = 20 * Math.log10(Math.pow(2, exponent) * (1 + mantissa / 8));

      // Debug: Log raw bytes and formula results for RSSI calibration
      // Enable via: window.__enableRssiDebug(true) from browser console
      if ((globalThis as unknown as Record<string, unknown>).__RSSI_DEBUG) {
        console.log(`[RSSI-Compact] raw=0x${nbRssi.toString(16).padStart(2, '0')} (${nbRssi}) | dBuV=${rssiDbuV.toFixed(2)} dBm=${(rssiDbuV - 106.98).toFixed(2)}`);
      }

      // CS108 RSSI formula produces values in dBµV (dB microvolts), not dBm
      // Convert to dBm using vendor formula: dBm = dBuV - 106.98
      // Range: 17-97 dBuV maps to -90 to -10 dBm
      // Source: CS108 vendor app ViewModelGeiger.cs dBuV2dBm()
      const rssi = rssiDbuV - 106.98;

      tags.push({
        epc,
        rssi,
        pc,
        antennaPort,
        timestamp: Date.now(),
        mode: 'compact'
      });

      offset += tagLength;
      this.tagsExtracted++;

      if (this.debug) {
        logger.debug(`[InventoryParser] Tag: EPC=${epc}, RSSI=${rssi}dBm, PC=0x${pc.toString(16)}, Antenna=${antennaPort}`);
      }
    }

    // Keep any partial tag data for next packet
    if (offset < data.length) {
      const remaining = data.subarray(offset);
      this.buffer.dataIn(remaining);
    }

    return tags;
  }

  /**
   * Set the expected inventory mode
   */
  setMode(mode: InventoryMode): void {
    this.expectedMode = mode;
    logger.debug(`[InventoryParser] Mode set to: ${mode}`);
  }

  /**
   * Reset parser state and clear buffer
   */
  reset(): void {
    this.buffer.clear();
    this.sequenceNumber = 0;
    this.packetsProcessed = 0;
    this.tagsExtracted = 0;
    this.parseErrors = 0;
    this.lastPacketTime = 0;
    logger.debug('[InventoryParser] Reset complete');
  }

  /**
   * Get current parser state for diagnostics
   */
  getState(): ParserState {
    return {
      buffer: this.buffer,
      sequenceNumber: this.sequenceNumber,
      mode: this.expectedMode,
      packetsProcessed: this.packetsProcessed,
      tagsExtracted: this.tagsExtracted,
      lastPacketTime: this.lastPacketTime,
      parseErrors: this.parseErrors
    };
  }

  /**
   * Get buffer metrics for health monitoring
   */
  getBufferMetrics(): BufferMetrics {
    return this.buffer.getMetrics();
  }

  /**
   * Check buffer health and log warnings if needed
   */
  checkBufferHealth(): BufferMetrics {
    const metrics = this.buffer.getMetrics();
    if (metrics.utilizationPercent > 80) {
      logger.warn(`[InventoryParser] Buffer utilization high: ${metrics.utilizationPercent}%`);
    }
    return metrics;
  }

  /**
   * Parse normal mode inventory response packet
   * Used in LOCATE mode for single tag responses with detailed RSSI info
   */
  private parseNormalModeTag(fullPacket: Uint8Array): ParsedTag | null {
    // Normal mode packet structure (from CS108 spec):
    // Byte 12: wb_rssi (Wideband RSSI)
    // Byte 13: nb_rssi (Narrowband RSSI)
    // Byte 14: phase
    // Byte 15: channel index
    // Byte 19:18: antenna port
    // Byte 20+: inv_data (PC + EPC + CRC16)

    if (fullPacket.length < 20) {
      return null; // Too short
    }

    // Extract RSSI values
    const wbRssiByte = fullPacket[12];
    const nbRssi = fullPacket[13];
    const phase = fullPacket[14];
    const port = fullPacket[18] | (fullPacket[19] << 8);

    // Calculate narrowband RSSI using R2000 formula (CS108 always uses R2000 chip)
    // Formula: 20 * log10(2^Exponent * (1 + Mantissa / 8))
    // Where: Mantissa = bits 2:0, Exponent = bits 7:3
    // Result is in dBuV, convert to dBm: dBm = dBuV - 106.98
    const nbMantissa = nbRssi & 0x07;  // bits 2:0
    const nbExponent = (nbRssi >> 3) & 0x1F;  // bits 7:3
    const nbRssiDbuV = 20 * Math.log10(Math.pow(2, nbExponent) * (1 + nbMantissa / 8));

    // Calculate wideband RSSI using R2000 formula
    // Mantissa = bits 3:0, Exponent = bits 7:4, Mantissa_Size = 4
    // Formula: 20 * log10(2^Exponent * (1 + Mantissa / 16))
    const wbMantissa = wbRssiByte & 0x0F;  // bits 3:0
    const wbExponent = (wbRssiByte >> 4) & 0x0F;  // bits 7:4
    const wbRssiDbuV = 20 * Math.log10(Math.pow(2, wbExponent) * (1 + wbMantissa / 16));

    // Debug: Log raw bytes and formula results for RSSI calibration
    // Enable via: window.__enableRssiDebug(true) from browser console
    if ((globalThis as unknown as Record<string, unknown>).__RSSI_DEBUG) {
      console.log(`[RSSI] NB raw=0x${nbRssi.toString(16).padStart(2, '0')} (${nbRssi}) | dBuV=${nbRssiDbuV.toFixed(2)} dBm=${(nbRssiDbuV - 106.98).toFixed(2)}`);
      console.log(`[RSSI] WB raw=0x${wbRssiByte.toString(16).padStart(2, '0')} (${wbRssiByte}) | dBuV=${wbRssiDbuV.toFixed(2)} dBm=${(wbRssiDbuV - 106.98).toFixed(2)}`);
    }

    // CS108 RSSI formula produces values in dBµV (dB microvolts), not dBm
    // Convert to dBm using vendor formula: dBm = dBuV - 106.98
    // Range: 17-97 dBuV maps to -90 to -10 dBm
    // Source: CS108 vendor app ViewModelGeiger.cs dBuV2dBm()
    const rssi = nbRssiDbuV - 106.98;
    const wbRssi = wbRssiDbuV - 106.98;

    // Extract inventory data (PC + EPC + CRC16)
    const invData = fullPacket.subarray(20);

    if (invData.length < 4) {
      return null; // Need at least PC + CRC16
    }

    // Extract PC (first 2 bytes)
    const pc = (invData[0] << 8) | invData[1];

    // Get EPC length from PC
    const epcLengthWords = (pc >> 11) & 0x1F;
    const epcLengthBytes = epcLengthWords * 2;

    if (invData.length < 2 + epcLengthBytes + 2) {
      return null; // Not enough data for PC + EPC + CRC16
    }

    // Extract EPC
    const epcBytes = invData.subarray(2, 2 + epcLengthBytes);
    const epc = Array.from(epcBytes)
      .map(b => b.toString(16).padStart(2, '0'))
      .join('')
      .toUpperCase();

    return {
      epc,
      rssi,
      pc,
      antennaPort: port,
      timestamp: Date.now(),
      mode: 'normal',
      phase,
      wbRssi
    };
  }
}