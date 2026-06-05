import { describe, it, expect } from 'vitest';
import { parseReaderPayload, readerKeyFromTopic } from './parse';

describe('readerKeyFromTopic', () => {
  it('extracts the key segment from trakrf.id/{key}/reads', () => {
    expect(readerKeyFromTopic('trakrf.id/dock-01/reads')).toBe('dock-01');
  });

  it('falls back to the whole topic when it does not match the pattern', () => {
    expect(readerKeyFromTopic('Tagdata')).toBe('Tagdata');
  });

  it('returns empty string for empty topic', () => {
    expect(readerKeyFromTopic('')).toBe('');
  });
});

describe('parseReaderPayload', () => {
  const topic = 'trakrf.id/dock-01/reads';

  it('parses a well-formed single-tag payload', () => {
    const raw = JSON.stringify({
      tags: [
        {
          epc: 'E2801170',
          timeStampOfRead: 1_717_500_000_000_000, // µs
          antennaPort: 2,
          capturePointName: 'Dock Door 1',
          rssi: '-56',
        },
      ],
    });

    const reads = parseReaderPayload(topic, raw);
    expect(reads).toHaveLength(1);
    expect(reads[0]).toEqual({
      epc: 'E2801170',
      readerKey: 'dock-01',
      capturePointName: 'Dock Door 1',
      antennaPort: 2,
      rssi: -56,
      readerTimestampMs: 1_717_500_000_000, // µs → ms
    });
  });

  it('parses multiple tags in one payload', () => {
    const raw = JSON.stringify({
      tags: [
        { epc: 'AAA', timeStampOfRead: 1_000_000, antennaPort: 1, capturePointName: 'p', rssi: '-40' },
        { epc: 'BBB', timeStampOfRead: 2_000_000, antennaPort: 1, capturePointName: 'p', rssi: '-70' },
      ],
    });
    const reads = parseReaderPayload(topic, raw);
    expect(reads.map((r) => r.epc)).toEqual(['AAA', 'BBB']);
  });

  it('coerces float and signed rssi strings, rounding to int', () => {
    const raw = JSON.stringify({
      tags: [{ epc: 'X', timeStampOfRead: 0, antennaPort: 1, capturePointName: 'p', rssi: '-56.6' }],
    });
    expect(parseReaderPayload(topic, raw)[0].rssi).toBe(-57);
  });

  it('defaults rssi to 0 when blank or unparseable', () => {
    const raw = JSON.stringify({
      tags: [{ epc: 'X', timeStampOfRead: 0, antennaPort: 1, capturePointName: 'p', rssi: '' }],
    });
    expect(parseReaderPayload(topic, raw)[0].rssi).toBe(0);
  });

  it('returns [] for malformed JSON without throwing', () => {
    expect(parseReaderPayload(topic, 'not json{')).toEqual([]);
  });

  it('returns [] when tags is missing', () => {
    expect(parseReaderPayload(topic, JSON.stringify({ foo: 1 }))).toEqual([]);
  });

  it('skips tags missing an epc', () => {
    const raw = JSON.stringify({
      tags: [
        { timeStampOfRead: 0, antennaPort: 1, capturePointName: 'p', rssi: '-40' },
        { epc: 'OK', timeStampOfRead: 0, antennaPort: 1, capturePointName: 'p', rssi: '-40' },
      ],
    });
    expect(parseReaderPayload(topic, raw).map((r) => r.epc)).toEqual(['OK']);
  });

  it('accepts a Uint8Array payload (mqtt.js delivers Buffer/Uint8Array)', () => {
    const raw = JSON.stringify({
      tags: [{ epc: 'X', timeStampOfRead: 1_000_000, antennaPort: 1, capturePointName: 'p', rssi: '-40' }],
    });
    const bytes = new TextEncoder().encode(raw);
    expect(parseReaderPayload(topic, bytes)[0].epc).toBe('X');
  });
});
