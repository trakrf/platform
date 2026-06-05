import { describe, it, expect } from 'vitest';
import { parseSSEChunk, type SSEParseState } from './stream';
import type { ParsedRead } from '@/types/readerfeed';

const ev: ParsedRead = {
  epc: 'E1',
  readerKey: 'dock-9',
  capturePointName: 'cp',
  antennaPort: 1,
  rssi: -50,
  readerTimestampMs: 10,
};

describe('parseSSEChunk', () => {
  it('parses a complete data frame into a read', () => {
    const st: SSEParseState = { buffer: '' };
    const reads = parseSSEChunk(st, `data: ${JSON.stringify(ev)}\n\n`);
    expect(reads).toHaveLength(1);
    expect(reads[0]).toMatchObject(ev);
  });

  it('ignores comment/heartbeat frames', () => {
    const st: SSEParseState = { buffer: '' };
    expect(parseSSEChunk(st, ': ping\n\n')).toHaveLength(0);
    expect(parseSSEChunk(st, ': connected\n\n')).toHaveLength(0);
  });

  it('buffers a frame split across chunks', () => {
    const st: SSEParseState = { buffer: '' };
    const full = `data: ${JSON.stringify(ev)}\n\n`;
    const mid = Math.floor(full.length / 2);
    expect(parseSSEChunk(st, full.slice(0, mid))).toHaveLength(0);
    const reads = parseSSEChunk(st, full.slice(mid));
    expect(reads).toHaveLength(1);
    expect(reads[0].epc).toBe('E1');
  });

  it('parses multiple frames in one chunk', () => {
    const st: SSEParseState = { buffer: '' };
    const reads = parseSSEChunk(
      st,
      `data: ${JSON.stringify({ ...ev, epc: 'A' })}\n\ndata: ${JSON.stringify({ ...ev, epc: 'B' })}\n\n`,
    );
    expect(reads.map((r) => r.epc)).toEqual(['A', 'B']);
  });

  it('drops malformed JSON without throwing', () => {
    const st: SSEParseState = { buffer: '' };
    expect(parseSSEChunk(st, 'data: not-json\n\n')).toHaveLength(0);
  });

  it('drops frames without an epc string', () => {
    const st: SSEParseState = { buffer: '' };
    expect(parseSSEChunk(st, `data: ${JSON.stringify({ readerKey: 'x' })}\n\n`)).toHaveLength(0);
  });
});
