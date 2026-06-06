import { describe, it, expect } from 'vitest';
import { parseSSEChunk, type SSEParseState } from './stream';
import type { TagState } from '@/types/readerfeed';

const tag: TagState = {
  readerKey: 'dock-9',
  epc: 'E1',
  antennaPort: 1,
  firstSeen: 10,
  lastSeen: 20,
  readCount: 3,
  lastRssi: -50,
  rssiAvg: -52,
  rssiMin: -60,
  rssiMax: -40,
};

const frame = (type: string, data: unknown) => `event: ${type}\ndata: ${JSON.stringify(data)}\n\n`;

describe('parseSSEChunk (named presence events)', () => {
  it('parses an upsert frame into a typed event', () => {
    const st: SSEParseState = { buffer: '' };
    const evs = parseSSEChunk(st, frame('upsert', tag));
    expect(evs).toHaveLength(1);
    expect(evs[0].type).toBe('upsert');
    expect(evs[0].data).toMatchObject(tag);
  });

  it('parses a snapshot frame', () => {
    const st: SSEParseState = { buffer: '' };
    const evs = parseSSEChunk(st, frame('snapshot', { tags: [tag], uniqueTags: 1, readRate: 4.5 }));
    expect(evs[0].type).toBe('snapshot');
    expect(evs[0].data).toMatchObject({ uniqueTags: 1, readRate: 4.5 });
  });

  it('parses a leave frame', () => {
    const st: SSEParseState = { buffer: '' };
    const evs = parseSSEChunk(st, frame('leave', { readerKey: 'dock-9', epc: 'E1' }));
    expect(evs[0]).toMatchObject({ type: 'leave', data: { readerKey: 'dock-9', epc: 'E1' } });
  });

  it('ignores comment/heartbeat frames', () => {
    const st: SSEParseState = { buffer: '' };
    expect(parseSSEChunk(st, ': ping\n\n')).toHaveLength(0);
    expect(parseSSEChunk(st, ': connected\n\n')).toHaveLength(0);
  });

  it('buffers a frame split across chunks', () => {
    const st: SSEParseState = { buffer: '' };
    const full = frame('upsert', tag);
    const mid = Math.floor(full.length / 2);
    expect(parseSSEChunk(st, full.slice(0, mid))).toHaveLength(0);
    const evs = parseSSEChunk(st, full.slice(mid));
    expect(evs).toHaveLength(1);
    expect((evs[0].data as TagState).epc).toBe('E1');
  });

  it('parses multiple frames in one chunk', () => {
    const st: SSEParseState = { buffer: '' };
    const evs = parseSSEChunk(
      st,
      frame('upsert', { ...tag, epc: 'A' }) + frame('upsert', { ...tag, epc: 'B' }),
    );
    expect(evs.map((e) => e.type)).toEqual(['upsert', 'upsert']);
  });

  it('drops malformed JSON without throwing', () => {
    const st: SSEParseState = { buffer: '' };
    expect(parseSSEChunk(st, 'event: upsert\ndata: not-json\n\n')).toHaveLength(0);
  });

  it('drops frames with an unknown event type', () => {
    const st: SSEParseState = { buffer: '' };
    expect(parseSSEChunk(st, frame('bogus', tag))).toHaveLength(0);
  });

  it('drops upsert frames without an epc string', () => {
    const st: SSEParseState = { buffer: '' };
    expect(parseSSEChunk(st, frame('upsert', { readerKey: 'x' }))).toHaveLength(0);
  });
});
