/**
 * Reading the reader's own identity.
 *
 * The pure half — which register means what, what the log line says — lives
 * here. The wiring that puts these on the wire is exercised in `reader.test.ts`,
 * because that is where the decisions about WHEN to read them live.
 *
 * Refs: TRA-1232.
 */

import { describe, it, expect } from 'vitest';
import {
  IDENTITY_SEQUENCE,
  RFID_IDENTITY_SEQUENCE,
  applyRegisterResponse,
  applyIdentityPacket,
  formatReaderDetails,
  READER_DETAILS_LOG_PREFIX,
} from './identity';
import { RFID_REGISTERS } from '../rfid/constant';
import { RFID_FIRMWARE_COMMAND, INVENTORY_TAG_NOTIFICATION } from '../event';
import { GET_SILICON_LAB_VERSION, GET_BLUETOOTH_VERSION, GET_SERIAL_NUMBER } from './device-info';

describe('IDENTITY_SEQUENCE', () => {
  it('reads the three values the Bluetooth board holds', () => {
    expect(IDENTITY_SEQUENCE.map((c) => c.event)).toEqual([
      GET_SILICON_LAB_VERSION,
      GET_BLUETOOTH_VERSION,
      GET_SERIAL_NUMBER,
    ]);
  });

  /**
   * A retry here would buy a version string at the price of delaying the moment
   * the reader becomes usable, and nobody is waiting on a version string.
   *
   * `toleratesFailure` is deliberately absent too. These do not run through
   * `runSequence`, so nothing would read the flag — and a flag nothing reads is
   * worse than no flag, because it reads as a guarantee. The tolerance is
   * asserted where it lives, in `reader.test.ts`.
   */
  it('does not retry, and does not claim a tolerance it cannot enforce', () => {
    for (const cmd of IDENTITY_SEQUENCE) {
      expect(cmd.retryDelays).toBeUndefined();
      expect(cmd.toleratesFailure).toBeUndefined();
    }
  });
});

describe('RFID_IDENTITY_SEQUENCE', () => {
  /**
   * Both are register READS on `0x8002`, which is a path nothing in this
   * codebase has ever taken: `createFirmwareCommand` has had a READ_REGISTER
   * branch since it was written and no caller. The payload is pinned here
   * because a read and a write differ by one byte — access `0x00` versus
   * `0x01` — and a read built as a write would silently overwrite the register
   * it meant to inspect. On `MAC_ERROR` that would destroy the very value we
   * are asking for.
   */
  it('asks to read, never to write', () => {
    for (const cmd of RFID_IDENTITY_SEQUENCE) {
      expect(cmd.event).toBe(RFID_FIRMWARE_COMMAND);
      expect(cmd.payload?.[0]).toBe(0x70);  // low-level API
      expect(cmd.payload?.[1]).toBe(0x00);  // READ access
    }
  });

  it('reads the firmware version and the MAC error, LSB-first', () => {
    const addresses = RFID_IDENTITY_SEQUENCE.map(
      (c) => (c.payload![2] | (c.payload![3] << 8))
    );
    expect(addresses).toEqual([RFID_REGISTERS.FIRMWARE_VER, RFID_REGISTERS.MAC_ERROR]);
  });

  /**
   * These ride an RFID mode sequence, and a mode change failing is how the
   * reader ends a session in ERROR. TRA-1217 cost 63 of 200 soak reps to
   * exactly that shape — one op code going quiet inside a sequence that
   * prefixes every mode.
   */
  it('never fails the mode change it rides on', () => {
    for (const cmd of RFID_IDENTITY_SEQUENCE) {
      expect(cmd.toleratesFailure).toBe(true);
    }
  });
});

describe('applyRegisterResponse', () => {
  it('decodes the RFID processor firmware from FIRMWARE_VER', () => {
    // major 2, minor 6, patch 46 — the published RFID image, V2.6.46.
    const raw = (2 << 24) | (6 << 12) | 46;
    expect(applyRegisterResponse({}, { register: RFID_REGISTERS.FIRMWARE_VER, value: raw }))
      .toEqual({ rfidFirmware: '2.6.46' });
  });

  it('records MAC_ERROR as a number, including a healthy zero', () => {
    expect(applyRegisterResponse({}, { register: RFID_REGISTERS.MAC_ERROR, value: 0 }))
      .toEqual({ macError: 0 });
    expect(applyRegisterResponse({}, { register: RFID_REGISTERS.MAC_ERROR, value: 0x0309 }))
      .toEqual({ macError: 0x0309 });
  });

  it('keeps what it was already told', () => {
    const before = { serialNumber: 'CS108ABC12345' };
    expect(applyRegisterResponse(before, { register: RFID_REGISTERS.MAC_ERROR, value: 0 }))
      .toEqual({ serialNumber: 'CS108ABC12345', macError: 0 });
  });

  /**
   * TRA-1154 is the defect where any firmware-command reply could settle any
   * firmware command, because at the op-code level they are indistinguishable.
   * The echoed `reg_addr` is the one exception the protocol offers — so an
   * answer about `ANT_PORT_POWER` must not be filed as a firmware version just
   * because it arrived while we were asking about one.
   */
  it('ignores a response about a register it did not ask for', () => {
    expect(applyRegisterResponse({}, { register: RFID_REGISTERS.ANT_PORT_POWER, value: 300 }))
      .toBeNull();
  });
});

describe('applyIdentityPacket', () => {
  const noPayload = { rawPayload: new Uint8Array(0) };

  it('takes the board versions from their decoded payloads', () => {
    expect(applyIdentityPacket({}, {
      eventCode: GET_SILICON_LAB_VERSION.eventCode,
      payload: { major: 1, minor: 0, patch: 17, text: '1.0.17', packed: 0x00010011 },
      ...noPayload,
    })).toEqual({ siliconLabsFirmware: '1.0.17' });

    expect(applyIdentityPacket({}, {
      eventCode: GET_BLUETOOTH_VERSION.eventCode,
      payload: { major: 1, minor: 0, patch: 20, text: '1.0.20', packed: 0x00010014 },
      ...noPayload,
    })).toEqual({ bluetoothFirmware: '1.0.20' });
  });

  it('takes the serial number', () => {
    expect(applyIdentityPacket({}, {
      eventCode: GET_SERIAL_NUMBER.eventCode,
      payload: 'CS108ABC12345',
      ...noPayload,
    })).toEqual({ serialNumber: 'CS108ABC12345' });
  });

  /**
   * `parseSerialNumber` yields `''` for an unreadable payload, matching the
   * vendor's own try/catch. An empty string is not a serial number we read; it
   * is a read that did not work, and reporting it would put a blank where the
   * UI should say Unknown.
   */
  it('does not report an empty serial as if it had been read', () => {
    expect(applyIdentityPacket({}, {
      eventCode: GET_SERIAL_NUMBER.eventCode,
      payload: '',
      ...noPayload,
    })).toBeNull();
  });

  /**
   * ⚠ On 0x8100, not on the 0x8002 the read was sent on. Measured on hardware
   * 2026-09-02 — 0x8002 answers a read with the same one-byte status a write
   * gets, and the value comes back on the RFID processor's uplink data channel.
   */
  it('decodes a register response arriving on the RFID uplink channel', () => {
    const raw = (2 << 24) | (6 << 12) | 46;
    expect(applyIdentityPacket({}, {
      eventCode: INVENTORY_TAG_NOTIFICATION.eventCode,
      rawPayload: new Uint8Array([
        0x70, 0x00,
        RFID_REGISTERS.FIRMWARE_VER & 0xFF, (RFID_REGISTERS.FIRMWARE_VER >> 8) & 0xFF,
        raw & 0xFF, (raw >> 8) & 0xFF, (raw >> 16) & 0xFF, (raw >> 24) & 0xFF,
      ]),
    })).toEqual({ rfidFirmware: '2.6.46' });
  });

  /**
   * Every register WRITE this app performs is acknowledged on the same op code
   * with a one-byte status. There are thousands of those per session and none
   * of them says anything about the reader's identity.
   */
  it('ignores the command acknowledgement, which carries no value', () => {
    expect(applyIdentityPacket({}, {
      eventCode: RFID_FIRMWARE_COMMAND.eventCode,
      rawPayload: new Uint8Array([0x00]),
    })).toBeNull();
  });

  /**
   * The uplink channel a register value arrives on is the one tag reads arrive
   * on. There are thousands of those per scanning session; filing one as a
   * firmware version would be worse than reading nothing.
   */
  it('ignores an inventory packet on that same channel', () => {
    expect(applyIdentityPacket({}, {
      eventCode: INVENTORY_TAG_NOTIFICATION.eventCode,
      rawPayload: new Uint8Array([0x04, 0x00, 0x05, 0x80, 0x0A, 0x00, 0x00, 0x00]),
    })).toBeNull();
  });

  it('ignores an op code that says nothing about identity', () => {
    expect(applyIdentityPacket({}, {
      eventCode: 0xA000,
      payload: 87,
      ...noPayload,
    })).toBeNull();
  });
});

describe('formatReaderDetails', () => {
  /**
   * The soak instrument parses this line, so its shape is a contract rather
   * than a convenience. JSON because the alternative — space-separated
   * key=value — has to decide what happens when a serial number contains a
   * space, and "it never will" is the kind of assumption that produces a parser
   * that works until it does not.
   */
  it('emits a line the instrument can parse back', () => {
    const line = formatReaderDetails({
      siliconLabsFirmware: '1.0.17',
      bluetoothFirmware: '1.0.20',
      rfidFirmware: '2.6.46',
      serialNumber: 'CS108ABC12345',
      macError: 0,
    });
    expect(line.startsWith(READER_DETAILS_LOG_PREFIX)).toBe(true);
    expect(JSON.parse(line.slice(READER_DETAILS_LOG_PREFIX.length))).toEqual({
      siliconLabsFirmware: '1.0.17',
      bluetoothFirmware: '1.0.20',
      rfidFirmware: '2.6.46',
      serialNumber: 'CS108ABC12345',
      macError: 0,
    });
  });

  it('is a single line even when most of it is unknown', () => {
    const line = formatReaderDetails({ bluetoothFirmware: '1.0.20' });
    expect(line).not.toContain('\n');
    expect(JSON.parse(line.slice(READER_DETAILS_LOG_PREFIX.length)))
      .toEqual({ bluetoothFirmware: '1.0.20' });
  });
});
