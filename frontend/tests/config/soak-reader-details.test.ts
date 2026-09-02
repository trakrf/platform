import { describe, it, expect } from 'vitest';
import { writeFileSync, mkdtempSync } from 'fs';
import { tmpdir } from 'os';
import path from 'path';
// @ts-expect-error — .mjs instrument module, no types by design
import { readReaderDetails, READER_DETAILS_PREFIX } from '../../scripts/suite-run-signals.mjs';

/**
 * TRA-1232. Every capture we hold is unattributed.
 *
 * The 2026-09-01 campaign produced four transport captures of a device-side
 * defect, and not one of them can say what firmware it was observed on. That
 * had to be reconstructed from notes afterwards — the exact "quoted rather than
 * measured" failure the campaign spent itself correcting — and flashing the
 * reader destroys the attribution permanently.
 *
 * This is an EXTRACTED value rather than a counted needle, so it lives beside
 * the table rather than in it, and `null` has to stay distinct from a value for
 * the same reason as in `readReadCycles`: a rep that recorded nothing must not
 * be indistinguishable from one that recorded a reader with no firmware.
 */
const withLog = (text: string) => {
  const dir = mkdtempSync(path.join(tmpdir(), 'readerdetails-'));
  const file = path.join(dir, 'out.log');
  writeFileSync(file, text);
  return file;
};

describe('readReaderDetails', () => {
  it('extracts what the reader said it was', () => {
    const log = withLog(
      'some other line\n' +
        `${READER_DETAILS_PREFIX}{"siliconLabsFirmware":"1.0.17","bluetoothFirmware":"1.0.20","serialNumber":"CS108ABC12345"}\n`
    );

    expect(readReaderDetails(log)).toEqual({
      siliconLabsFirmware: '1.0.17',
      bluetoothFirmware: '1.0.20',
      serialNumber: 'CS108ABC12345',
    });
  });

  /**
   * The worker emits this line each time a value lands, not once at the end —
   * three at connect, two more once the radio is powered. The LAST line is the
   * complete picture, and taking the first would attribute the rep to a partial
   * read that happens to be missing the most valuable of the three versions.
   */
  it('takes the last line, which is the one that has everything', () => {
    const log = withLog(
      `${READER_DETAILS_PREFIX}{"bluetoothFirmware":"1.0.20"}\n` +
        `${READER_DETAILS_PREFIX}{"bluetoothFirmware":"1.0.20","rfidFirmware":"2.6.46"}\n`
    );

    expect(readReaderDetails(log)).toEqual({
      bluetoothFirmware: '1.0.20',
      rfidFirmware: '2.6.46',
    });
  });

  /**
   * A rep whose reader never answered measured nothing, and `{}` would say the
   * opposite — that we asked and the reader has no firmware versions. Same
   * null-versus-empty discipline the rest of this module runs on.
   */
  it('is null when the reader never said', () => {
    expect(readReaderDetails(withLog('nothing to see here\n'))).toBeNull();
  });

  it('is null when there is no log at all', () => {
    expect(readReaderDetails('/no/such/log')).toBeNull();
    expect(readReaderDetails(undefined)).toBeNull();
  });

  /**
   * A truncated log is the ordinary way this line gets mangled: the capture is
   * cut mid-write and the JSON never closes. That is a rep that recorded
   * nothing usable, and it must read as nothing rather than crash the summary
   * that every other rep in the arm depends on.
   */
  it('is null rather than throwing on a half-written line', () => {
    expect(readReaderDetails(withLog(`${READER_DETAILS_PREFIX}{"bluetoothFirm`))).toBeNull();
  });
});
