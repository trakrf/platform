/**
 * CS108 Packet Handler
 *
 * High-level packet handling for CS108 RFID reader:
 * - Stateful packet building and fragmentation
 * - Command/response/notification packet creation
 * - Fragment timeout management
 * - Transport type switching (BLE vs USB)
 */

import type { CS108Event, CS108Packet } from './type.js';
import { logger } from '../utils/logger.js';
import { CS108_EVENT_MAP } from './event.js';
import { PACKET_CONSTANTS, calculatePacketCRC, validatePacketCRC, validatePacketLength } from './protocol.js';
import * as Sentry from '@sentry/react';
import { PacketDebugBuffer } from './utils/packet-debug-buffer.js';

export class PacketHandler {
  private currentPacket: CS108Packet | null = null;
  private fragmentTimeout?: NodeJS.Timeout;
  private transportByte: number = PACKET_CONSTANTS.TRANSPORT_BLUETOOTH; // Default to Bluetooth for handheld
  private debugBuffer = new PacketDebugBuffer();

  constructor() {
    // Debug: Verify event map is properly initialized
    logger.debug(`[PacketHandler] CS108_EVENT_MAP initialized with ${CS108_EVENT_MAP.size} events`);
    if (CS108_EVENT_MAP.size === 0) {
      logger.error('[PacketHandler] WARNING: CS108_EVENT_MAP is empty!');
    }
  }

  /**
   * Build a CS108 packet for any event (command or notification)
   * @param event - The CS108 event to build a packet for
   * @param payload - Optional payload data
   * @param options - Build options:
   *   - direction: 'downlink' for commands TO device, 'uplink' for responses/notifications FROM device
   *   - crc: Optional CRC value to inject (for testing), otherwise calculated automatically
   */
  buildPacket(
    event: CS108Event, 
    payload?: Uint8Array, 
    options?: {
      direction?: 'downlink' | 'uplink';
      crc?: number;
    }
  ): Uint8Array {
    const payloadData = payload || event.payload || new Uint8Array(0);
    const dataLength = 2 + payloadData.length; // Event code (2 bytes) + payload
    const totalLength = PACKET_CONSTANTS.HEADER_SIZE + dataLength;
    
    if (dataLength > PACKET_CONSTANTS.MAX_DATA_SIZE) {
      throw new Error(`Data too large: ${dataLength} bytes (max ${PACKET_CONSTANTS.MAX_DATA_SIZE})`);
    }
    
    const packet = new Uint8Array(totalLength);
    
    // Build 8-byte header
    packet[PACKET_CONSTANTS.PREFIX_OFFSET] = PACKET_CONSTANTS.PREFIX_BYTE;            // 0xA7
    packet[PACKET_CONSTANTS.TRANSPORT_OFFSET] = this.transportByte;                   // 0xB3 (BT) or 0xE6 (USB)
    packet[PACKET_CONSTANTS.LENGTH_OFFSET] = dataLength;                              // Single byte length
    packet[PACKET_CONSTANTS.MODULE_OFFSET] = event.module;                            // Module (0xC2, etc.)
    packet[PACKET_CONSTANTS.RESERVE_OFFSET] = PACKET_CONSTANTS.RESERVE_BYTE;          // 0x82
    const direction = options?.direction ?? 'downlink';
    packet[PACKET_CONSTANTS.DIRECTION_OFFSET] = direction === 'downlink' 
      ? PACKET_CONSTANTS.DOWNLINK_DIRECTION   // 0x37 (TO device)
      : PACKET_CONSTANTS.UPLINK_DIRECTION;    // 0x9E (FROM device)
    
    // Event code bytes go directly into positions 8-9 (big-endian)
    packet[8] = (event.eventCode >> 8) & 0xFF;  // High byte first
    packet[9] = event.eventCode & 0xFF;         // Low byte second

    // Payload
    if (payloadData.length > 0) {
      packet.set(payloadData, PACKET_CONSTANTS.PAYLOAD_OFFSET);
    }

    // CRC - For commands (downlink), CS108 accepts 0x00 0x00 for CRC bytes
    // For responses/notifications (uplink), we calculate CRC using vendor algorithm
    // Note: CRC bytes must be 0 during calculation since they're excluded
    packet[PACKET_CONSTANTS.CRC_OFFSET] = 0x00;     // Temporarily zero for calculation
    packet[PACKET_CONSTANTS.CRC_OFFSET + 1] = 0x00;

    const crc = options?.crc ?? (() => {
      if (direction === 'downlink') {
        // Commands can use zero CRC per CS108 spec
        return 0x0000;
      } else {
        // Responses/notifications use vendor CRC algorithm (full packet minus CRC bytes)
        return calculatePacketCRC(packet);
      }
    })();
    packet[PACKET_CONSTANTS.CRC_OFFSET] = (crc >> 8) & 0xFF;     // High byte at position 6 (big-endian)
    packet[PACKET_CONSTANTS.CRC_OFFSET + 1] = crc & 0xFF;        // Low byte at position 7 (big-endian)
    
    return packet;
  }
  
  /**
   * Build a command packet (downlink TO device)
   */
  buildCommand(event: CS108Event, payload?: Uint8Array): Uint8Array {
    const packet = this.buildPacket(event, payload, { direction: 'downlink' });

    // Debug logging
    logger.debug(`[PacketHandler] Building command ${event.name}:`, {
      eventCode: `0x${event.eventCode.toString(16).padStart(4, '0')}`,
      module: `0x${event.module.toString(16).padStart(2, '0')}`,
      payloadLength: payload?.length || 0,
      packet: Array.from(packet).map(b => `0x${b.toString(16).padStart(2, '0')}`).join(' ')
    });

    return packet;
  }
  
  /**
   * Build a response packet (uplink FROM device)
   * Used for testing and simulation
   * @param options - Optional CRC injection for testing
   */
  buildResponse(event: CS108Event, payload?: Uint8Array, options?: { crc?: number }): Uint8Array {
    return this.buildPacket(event, payload, { direction: 'uplink', ...options });
  }
  
  /**
   * Build a notification packet (uplink FROM device)  
   * Used for testing and simulation
   * @param options - Optional CRC injection for testing
   */
  buildNotification(event: CS108Event, payload?: Uint8Array, options?: { crc?: number }): Uint8Array {
    return this.buildPacket(event, payload, { direction: 'uplink', ...options });
  }
  
  /**
   * Parse packet header to create initial packet object
   * Used for packet-centric fragmentation
   */
  private parseHeader(data: Uint8Array): CS108Packet {
    // Need at least 8 bytes for header
    if (data.length < 8) {
      throw new Error(`Insufficient data for header: ${data.length} bytes`);
    }

    // Validate prefix byte (byte 0) - this is always 0xA7
    if (data[0] !== PACKET_CONSTANTS.PREFIX_BYTE) {
      throw new Error(`Invalid prefix: 0x${data[0].toString(16)}`);
    }
    
    // Parse header
    const length = data[2];
    const totalExpected = PACKET_CONSTANTS.HEADER_SIZE + length;
    
    // If we have event code, parse it
    let eventCode = 0;
    let event: CS108Event | undefined;
    if (data.length >= 10) {
      eventCode = (data[8] << 8) | data[9];  // big-endian
      event = CS108_EVENT_MAP.get(eventCode);
      logger.debug(`[PacketHandler] Parsed event code: 0x${eventCode.toString(16).padStart(4, '0')}, found event: ${event?.name || 'NOT FOUND'}`);
      if (!event) {
        logger.error(`[PacketHandler] Event map has ${CS108_EVENT_MAP.size} entries`);
        logger.error(`[PacketHandler] Looking for 0x${eventCode.toString(16)}, available: ${Array.from(CS108_EVENT_MAP.keys()).map(k => '0x' + k.toString(16)).join(', ')}`);
        throw new Error(`Unknown event code: 0x${eventCode.toString(16).padStart(4, '0')}`);
      }
    }
    
    // Create initial packet object
    return {
      // Header fields
      prefix: (data[0] << 8) | data[1], // Combined 0xA7B3
      transport: data[1],
      length: data[2],
      module: data[3],
      reserve: data[4],
      direction: data[5],
      crc: (data[6] << 8) | data[7], // Big-endian per vendor spec

      // Event (if we have it)
      eventCode,
      event: event!,  // Will be set when we have full event code

      // Payload - just what we have so far
      rawPayload: data.length > 10 ? data.slice(10) : new Uint8Array(0),
      payload: undefined,

      // Computed
      totalExpected,
      isComplete: data.length >= totalExpected
    };
  }
  
  /**
   * Buffer to accumulate raw bytes when building fragmented packets
   */
  private rawDataBuffer: Uint8Array = new Uint8Array(0);

  /**
   * Process incoming data using packet-centric fragmentation
   * Maintains CS108Packet object and appends raw bytes until complete
   */
  processIncomingData(data: Uint8Array): CS108Packet[] {
    const completedPackets: CS108Packet[] = [];

    // Capture raw packet for debugging
    if (data.length > 0) {
      this.debugBuffer.add(data);
    }

    // Clear any existing timeout
    if (this.fragmentTimeout) {
      clearTimeout(this.fragmentTimeout);
      this.fragmentTimeout = undefined;
    }

    // Check if incoming data looks like a new packet header (lost fragments scenario)
    // This handles the edge case where we lost continuation fragments and received a new packet
    if (this.currentPacket && data.length >= PACKET_CONSTANTS.HEADER_SIZE && data[0] === PACKET_CONSTANTS.PREFIX_BYTE) {
      // Potential new packet - try to parse as header to confirm
      try {
        this.parseHeader(data);
        // If parseHeader succeeded, this IS a new packet, not a continuation
        logger.warn(
          `[PacketHandler] Detected new packet while expecting fragment. ` +
          `Expected ${this.currentPacket.totalExpected - this.rawDataBuffer.length} more bytes for previous packet. ` +
          `Discarding incomplete packet and starting fresh with new packet.`
        );
        this.currentPacket = null;
        this.rawDataBuffer = new Uint8Array(0);
      } catch (error) {
        // Not a valid header, treat as continuation data
        logger.debug(`[PacketHandler] Data starts with 0xA7 but not a valid header, treating as continuation`);
      }
    }

    // Append new data to our buffer - this is key for fragmentation!
    // Fragments don't have headers, they're just continuation bytes
    const combined = new Uint8Array(this.rawDataBuffer.length + data.length);
    combined.set(this.rawDataBuffer);
    combined.set(data, this.rawDataBuffer.length);
    this.rawDataBuffer = combined;

    if (!this.currentPacket) {
      // No packet in progress - look for packet header
      if (this.rawDataBuffer.length < PACKET_CONSTANTS.HEADER_SIZE) {
        // Not enough data for header yet, wait for more
        this.startFragmentTimeout();
        return completedPackets;
      }

      // Look for the start of a valid packet by finding 0xA7
      let headerStart = -1;
      for (let i = 0; i <= this.rawDataBuffer.length - PACKET_CONSTANTS.HEADER_SIZE; i++) {
        if (this.rawDataBuffer[i] === PACKET_CONSTANTS.PREFIX_BYTE) {
          // Found potential header start, try to parse it
          try {
            const testBuffer = this.rawDataBuffer.slice(i);
            this.currentPacket = this.parseHeader(testBuffer);
            headerStart = i;
            break;
          } catch (error) {
            // Not a valid header, keep looking
            continue;
          }
        }
      }

      if (headerStart === -1) {
        // No valid header found in buffer
        if (this.rawDataBuffer.length > PACKET_CONSTANTS.MAX_PACKET_SIZE) {
          // Buffer is too large, something is wrong - clear it
          logger.error(
            `[PacketHandler] Buffer overflow - no valid header found in ${this.rawDataBuffer.length} bytes. ` +
            `Clearing buffer.`
          );
          this.rawDataBuffer = new Uint8Array(0);
        } else {
          // Keep the last few bytes in case they're the start of a header
          const keepBytes = Math.min(this.rawDataBuffer.length, PACKET_CONSTANTS.HEADER_SIZE - 1);
          this.rawDataBuffer = this.rawDataBuffer.slice(-keepBytes);
          this.startFragmentTimeout();
        }
        return completedPackets;
      }

      if (headerStart > 0) {
        // Discard bytes before the header
        logger.debug(`[PacketHandler] Discarding ${headerStart} bytes before valid header`);
        this.rawDataBuffer = this.rawDataBuffer.slice(headerStart);
      }

      if (this.currentPacket) {
        logger.debug(
          `[PacketHandler] Started packet, expecting ${this.currentPacket.totalExpected} bytes total, ` +
          `have ${this.rawDataBuffer.length} so far`
        );
      }
    }

    // Check if we have a complete packet
    if (this.currentPacket && this.rawDataBuffer.length >= this.currentPacket.totalExpected) {
      // We have enough data for the complete packet
      const packet = this.finalizePacket();
      if (packet) {
        completedPackets.push(packet);
      }

      // If we have leftover data, it might be the start of the next packet
      // Process it recursively
      if (this.rawDataBuffer.length > 0) {
        const additionalPackets = this.processIncomingData(new Uint8Array(0));
        completedPackets.push(...additionalPackets);
      }
    } else if (this.currentPacket) {
      // Still waiting for more fragments
      logger.debug(
        `[PacketHandler] Waiting for fragments: have ${this.rawDataBuffer.length}/${this.currentPacket.totalExpected} bytes`
      );
      this.startFragmentTimeout();
    }

    return completedPackets;
  }
  
  /**
   * Finalize the current packet once we have all the data
   */
  private finalizePacket(): CS108Packet | null {
    if (!this.currentPacket || this.rawDataBuffer.length < this.currentPacket.totalExpected) {
      return null;
    }

    try {
      // Extract exactly the amount of data we need
      const packetData = this.rawDataBuffer.slice(0, this.currentPacket.totalExpected);

      // === Validate length ===
      const lengthResult = validatePacketLength(packetData);
      if (!lengthResult.valid) {
        logger.warn(
          `[PacketHandler] Length validation failed: expected ${lengthResult.expected}, got ${lengthResult.actual}`
        );
        Sentry.captureMessage('CS108 packet length validation failed', {
          level: 'warning',
          extra: {
            expected: lengthResult.expected,
            actual: lengthResult.actual,
            packetHex: Array.from(packetData.slice(0, 20)).map(b => b.toString(16).padStart(2, '0')).join(' ')
          }
        });
        // Discard invalid packet but preserve any remaining bytes for next packet
        this.rawDataBuffer = this.rawDataBuffer.slice(this.currentPacket.totalExpected);
        this.currentPacket = null;
        return null;
      }

      // === Validate CRC ===
      const crcResult = validatePacketCRC(packetData);
      if (!crcResult.valid) {
        logger.warn(
          `[PacketHandler] CRC validation failed: expected 0x${crcResult.expected.toString(16).padStart(4, '0')}, ` +
          `calculated 0x${crcResult.actual.toString(16).padStart(4, '0')}`
        );
        Sentry.captureMessage('CS108 packet CRC validation failed', {
          level: 'warning',
          extra: {
            expectedCRC: `0x${crcResult.expected.toString(16).padStart(4, '0')}`,
            calculatedCRC: `0x${crcResult.actual.toString(16).padStart(4, '0')}`,
            packetHex: Array.from(packetData.slice(0, 20)).map(b => b.toString(16).padStart(2, '0')).join(' ')
          }
        });
        // Discard invalid packet but preserve any remaining bytes for next packet
        this.rawDataBuffer = this.rawDataBuffer.slice(this.currentPacket.totalExpected);
        this.currentPacket = null;
        return null;
      }

      // Parse event code if we haven't already (bytes 8-9)
      if (!this.currentPacket.event && packetData.length >= 10) {
        const eventCode = (packetData[8] << 8) | packetData[9];
        const event = CS108_EVENT_MAP.get(eventCode);

        if (!event) {
          throw new Error(`Unknown event code: 0x${eventCode.toString(16).padStart(4, '0')}`);
        }

        this.currentPacket.eventCode = eventCode;
        this.currentPacket.event = event;
      }

      // Extract the actual payload (after the 2-byte event code)
      if (this.currentPacket.length > 2) {
        this.currentPacket.rawPayload = packetData.slice(10, this.currentPacket.totalExpected);
      } else {
        this.currentPacket.rawPayload = new Uint8Array(0);
      }

      // Parse payload if event has a parser
      if (this.currentPacket.event?.parser && this.currentPacket.rawPayload.length > 0) {
        try {
          this.currentPacket.payload = this.currentPacket.event.parser(this.currentPacket.rawPayload);
        } catch (error) {
          logger.warn(`[PacketHandler] Payload parse failed for ${this.currentPacket.event.name}:`, error);
        }
      }

      this.currentPacket.isComplete = true;

      logger.debug(
        `[PacketHandler] Completed packet: ${this.currentPacket.event?.name} ` +
        `(0x${this.currentPacket.eventCode.toString(16).padStart(4, '0')}), ` +
        `${this.currentPacket.length} bytes`
      );

      const packet = this.currentPacket;

      // Reset for next packet - check if we have leftover data
      const leftover = this.rawDataBuffer.slice(this.currentPacket.totalExpected);
      this.currentPacket = null;
      this.rawDataBuffer = new Uint8Array(0);

      // If we have leftover data, it might be the start of the next packet
      // Process it recursively
      if (leftover.length > 0) {
        logger.debug(`[PacketHandler] Processing ${leftover.length} leftover bytes`);
        // This will be handled in the next call
        this.rawDataBuffer = leftover;
      }

      return packet;
    } catch (error) {
      logger.error('[PacketHandler] Finalize packet failed:', error);
      this.currentPacket = null;
      this.rawDataBuffer = new Uint8Array(0);
      return null;
    }
  }

  /**
   * Parse a complete packet from raw data (used for testing)
   */
  /* Unused for now - keeping for potential future use
  private parseCompletePacket(data: Uint8Array): CS108Packet | null {
    try {
      // Validate minimum size
      if (data.length < PACKET_CONSTANTS.HEADER_SIZE + 2) {
        throw new Error(`Packet too small: ${data.length} bytes`);
      }

      // Validate prefix
      if (data[0] !== PACKET_CONSTANTS.PREFIX_BYTE) {
        throw new Error(`Invalid prefix: 0x${data[0].toString(16)}`);
      }

      // Parse header fields
      const length = data[2];
      const totalExpected = PACKET_CONSTANTS.HEADER_SIZE + length;

      // Validate we have the complete packet
      if (data.length !== totalExpected) {
        throw new Error(`Size mismatch: have ${data.length}, expected ${totalExpected}`);
      }

      // Extract event code (bytes 8-9 in big-endian)
      const eventCode = (data[8] << 8) | data[9];
      const event = CS108_EVENT_MAP.get(eventCode);

      if (!event) {
        logger.error(`[PacketHandler] Unknown event code: 0x${eventCode.toString(16).padStart(4, '0')}`);
        logger.error(`[PacketHandler] Available events: ${Array.from(CS108_EVENT_MAP.keys()).map(k => '0x' + k.toString(16)).join(', ')}`);
        throw new Error(`Unknown event code: 0x${eventCode.toString(16).padStart(4, '0')}`);
      }

      // Extract payload (everything after event code)
      const rawPayload = length > 2 ? data.slice(10, totalExpected) : new Uint8Array(0);

      // Parse payload if event has a parser
      let payload: any = undefined;
      if (event.parser && rawPayload.length > 0) {
        try {
          payload = event.parser(rawPayload);
        } catch (error) {
          logger.warn(`[PacketHandler] Payload parse failed for ${event.name}:`, error);
        }
      }

      // Create packet object
      const packet: CS108Packet = {
        prefix: (data[0] << 8) | data[1], // Combined 0xA7B3
        transport: data[1],
        length: data[2],
        module: data[3],
        reserve: data[4],
        direction: data[5],
        crc: (data[6] << 8) | data[7], // Big-endian per vendor spec
        eventCode,
        event,
        rawPayload,
        payload,
        totalExpected,
        isComplete: true
      };

      logger.debug(`[PacketHandler] Parsed complete packet: ${event.name} (0x${eventCode.toString(16).padStart(4, '0')}), ${length} bytes`);

      return packet;
    } catch (error) {
      logger.error('[PacketHandler] Failed to parse packet:', error);
      return null;
    }
  }
  */

  /**
   * Start or restart fragment timeout
   */
  private startFragmentTimeout(): void {
    if (this.fragmentTimeout) {
      clearTimeout(this.fragmentTimeout);
    }

    this.fragmentTimeout = setTimeout(() => {
      logger.error('[PacketHandler] Fragment timeout - discarding partial packet');
      this.currentPacket = null;
      this.rawDataBuffer = new Uint8Array(0); // Clear buffer on timeout
      this.fragmentTimeout = undefined;
    }, 200); // 200ms industry standard for BLE fragmentation
  }
  
  /**
   * Set transport type (for USB vs Bluetooth)
   */
  setTransportType(isUSB: boolean): void {
    this.transportByte = isUSB ? PACKET_CONSTANTS.TRANSPORT_USB : PACKET_CONSTANTS.TRANSPORT_BLUETOOTH;
  }

  /**
   * Reset fragment buffer
   */
  reset(): void {
    this.currentPacket = null;
    this.rawDataBuffer = new Uint8Array(0);
    if (this.fragmentTimeout) {
      clearTimeout(this.fragmentTimeout);
      this.fragmentTimeout = undefined;
    }
  }

  /**
   * Get debug report when packet reassembly errors occur
   */
  getDebugReport(context: string): string {
    return this.debugBuffer.getErrorReport(context);
  }

}