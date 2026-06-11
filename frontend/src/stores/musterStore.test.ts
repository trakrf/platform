import { describe, it, expect, beforeEach, vi } from 'vitest';

// Mock the REST api so refreshStatus() can be exercised against a fixed payload.
const statusMock = vi.fn();
vi.mock('@/lib/api/mustering', () => ({
  musteringApi: {
    status: () => statusMock(),
  },
}));

import {
  useMusterStore,
  parseSSEChunk,
  type MusterFrame,
  type SSEParseState,
} from './musterStore';
import type {
  MusterEvent,
  MusterEntry,
  MusterCounts,
  ZonePresence,
} from '@/types/mustering';

// --- fixtures ---

const counts = (over: Partial<MusterCounts> = {}): MusterCounts => ({
  expected: 0,
  missing: 0,
  at_muster: 0,
  verified: 0,
  safe_manual: 0,
  ...over,
});

const zone = (over: Partial<ZonePresence> = {}): ZonePresence => ({
  location_id: 10,
  name: 'Zone A',
  muster_point: false,
  count: 0,
  ...over,
});

const entry = (over: Partial<MusterEntry> = {}): MusterEntry => ({
  id: 100,
  org_id: 1,
  muster_event_id: 1,
  asset_id: 5,
  label: 'Alice',
  status: 'missing',
  created_at: '2026-06-11T00:00:00Z',
  updated_at: '2026-06-11T00:00:00Z',
  ...over,
});

const event = (over: Partial<MusterEvent> = {}): MusterEvent => ({
  id: 1,
  org_id: 1,
  status: 'active',
  started_at: '2026-06-11T00:00:00Z',
  window_minutes: 15,
  counts: counts({ expected: 1, missing: 1 }),
  entries: [entry()],
  created_at: '2026-06-11T00:00:00Z',
  updated_at: '2026-06-11T00:00:00Z',
  ...over,
});

// Build a raw SSE wire frame (event/data pair terminated by a blank line).
const wire = (type: string, data: unknown): string =>
  `event: ${type}\ndata: ${JSON.stringify(data)}\n\n`;

// Reset the store to its initial reducer-relevant state before each test.
function resetStore() {
  useMusterStore.setState({
    zones: [],
    personsOnSite: 0,
    event: null,
    connection: 'idle',
    revealUnlocked: false,
    error: null,
  });
}

describe('parseSSEChunk', () => {
  let state: SSEParseState;
  beforeEach(() => {
    state = { buffer: '' };
  });

  it('parses each frame type from the wire', () => {
    const chunk =
      wire('snapshot', { zones: [zone()], persons_on_site: 3, event: null }) +
      wire('presence', { zones: [zone({ count: 2 })], persons_on_site: 2, persons: [] }) +
      wire('entry', { entry: entry(), counts: counts() }) +
      wire('event', { event: event() });

    const frames = parseSSEChunk(state, chunk);
    expect(frames.map((f) => f.type)).toEqual(['snapshot', 'presence', 'entry', 'event']);
  });

  it('drops heartbeats (comments / data-less frames) without throwing', () => {
    const chunk = ': keep-alive\n\n' + 'event: snapshot\n\n' + wire('snapshot', {
      zones: [zone()],
      persons_on_site: 1,
      event: null,
    });
    const frames = parseSSEChunk(state, chunk);
    expect(frames).toHaveLength(1);
    expect(frames[0].type).toBe('snapshot');
  });

  it('drops malformed JSON and unknown event types', () => {
    const chunk =
      'event: snapshot\ndata: {not json}\n\n' +
      wire('bogus', { zones: [] }) +
      // snapshot missing required `zones` array → rejected by the shape guard
      wire('snapshot', { persons_on_site: 0, event: null });
    expect(parseSSEChunk(state, chunk)).toHaveLength(0);
  });

  it('buffers an incomplete trailing frame across chunks', () => {
    expect(parseSSEChunk(state, 'event: snapshot\ndata: {"zones":[],')).toHaveLength(0);
    const frames = parseSSEChunk(state, '"persons_on_site":0,"event":null}\n\n');
    expect(frames).toHaveLength(1);
    expect(frames[0].type).toBe('snapshot');
  });
});

describe('musterStore applyFrame', () => {
  beforeEach(resetStore);

  const apply = (f: MusterFrame) => useMusterStore.getState().applyFrame(f);

  it('snapshot replaces zones, persons-on-site, and event', () => {
    apply({
      type: 'snapshot',
      data: { zones: [zone({ count: 4 })], persons_on_site: 9, event: event() },
    });
    const s = useMusterStore.getState();
    expect(s.zones).toHaveLength(1);
    expect(s.zones[0].count).toBe(4);
    expect(s.personsOnSite).toBe(9);
    expect(s.event?.id).toBe(1);
  });

  it('presence updates zones + persons-on-site, discarding per-person data', () => {
    useMusterStore.setState({ event: event(), personsOnSite: 5 });
    apply({
      type: 'presence',
      data: {
        zones: [zone({ count: 7 })],
        persons_on_site: 7,
        persons: [{ asset_id: 5, label: 'Alice', location_id: 10, last_seen_at: 'x' }],
      },
    });
    const s = useMusterStore.getState();
    expect(s.zones[0].count).toBe(7);
    // Person-level detail is never reduced into store state.
    expect(JSON.stringify(s)).not.toContain('last_seen_at');
    // event untouched by a presence frame; persons-on-site now tracks the frame
    // (BUG 3 — the "on site" tile must stay live between snapshots).
    expect(s.event?.id).toBe(1);
    expect(s.personsOnSite).toBe(7);
  });

  it('entry frame replaces the matching entry by id and updates counts', () => {
    useMusterStore.setState({ event: event() });
    const updated = entry({ id: 100, status: 'verified', label: 'Alice (verified)' });
    apply({ type: 'entry', data: { entry: updated, counts: counts({ expected: 1, verified: 1 }) } });

    const s = useMusterStore.getState();
    expect(s.event?.entries).toHaveLength(1); // replaced, not appended
    expect(s.event?.entries?.[0].status).toBe('verified');
    expect(s.event?.counts.verified).toBe(1);
    expect(s.event?.counts.missing).toBe(0);
  });

  it('entry frame appends a new entry when the id is unseen', () => {
    useMusterStore.setState({ event: event() });
    apply({
      type: 'entry',
      data: { entry: entry({ id: 101, label: 'Bob' }), counts: counts({ expected: 2, missing: 2 }) },
    });
    const s = useMusterStore.getState();
    expect(s.event?.entries).toHaveLength(2);
    expect(s.event?.counts.expected).toBe(2);
  });

  it('refreshStatus coerces a null zones payload to [] (fresh-org guard, BUG 1)', async () => {
    // A fresh org with no locations: backend nil slice → JSON null. The REST
    // status path must coerce so the Dashboard useMemo that iterates zones never
    // throws "not iterable" and kills the whole tab.
    statusMock.mockResolvedValueOnce({
      data: { zones: null, persons_on_site: null, event: null },
    });
    await useMusterStore.getState().refreshStatus();
    const s = useMusterStore.getState();
    expect(Array.isArray(s.zones)).toBe(true);
    expect(s.zones).toHaveLength(0);
    expect(s.personsOnSite).toBe(0);
    expect(s.error).toBeNull();
  });

  it('keeps revealUnlocked across a same-id re-snapshot (so unlock survives)', () => {
    useMusterStore.setState({ event: event({ id: 1 }), revealUnlocked: true });
    apply({ type: 'snapshot', data: { zones: [], persons_on_site: 0, event: event({ id: 1 }) } });
    expect(useMusterStore.getState().revealUnlocked).toBe(true);
  });

  it('resets revealUnlocked when the snapshot carries a different event id', () => {
    useMusterStore.setState({ event: event({ id: 1 }), revealUnlocked: true });
    apply({ type: 'snapshot', data: { zones: [], persons_on_site: 0, event: event({ id: 2 }) } });
    expect(useMusterStore.getState().revealUnlocked).toBe(false);
  });

  it('resets revealUnlocked when the snapshot event is null (drill ended)', () => {
    useMusterStore.setState({ event: event({ id: 1 }), revealUnlocked: true });
    apply({ type: 'snapshot', data: { zones: [], persons_on_site: 0, event: null } });
    expect(useMusterStore.getState().revealUnlocked).toBe(false);
  });

  it('resets revealUnlocked on an event frame whose id changed', () => {
    useMusterStore.setState({ event: event({ id: 1 }), revealUnlocked: true });
    apply({ type: 'event', data: { event: event({ id: 2 }) } });
    expect(useMusterStore.getState().revealUnlocked).toBe(false);
  });

  it('resets revealUnlocked on an event frame that ends the drill', () => {
    useMusterStore.setState({ event: event({ id: 1 }), revealUnlocked: true });
    apply({ type: 'event', data: { event: event({ id: 1, status: 'completed' }) } });
    const s = useMusterStore.getState();
    expect(s.event?.status).toBe('completed');
    expect(s.revealUnlocked).toBe(false);
  });

  it('keeps revealUnlocked on a same-id, still-active event frame', () => {
    useMusterStore.setState({ event: event({ id: 1 }), revealUnlocked: true });
    apply({ type: 'event', data: { event: event({ id: 1, status: 'active' }) } });
    expect(useMusterStore.getState().revealUnlocked).toBe(true);
  });
});

describe('musterStore connection setters', () => {
  beforeEach(resetStore);

  it('clears the error when transitioning to live or connecting', () => {
    useMusterStore.setState({ error: 'stream HTTP 502', connection: 'error' });
    useMusterStore.getState().setConnection('connecting');
    expect(useMusterStore.getState().error).toBeNull();

    useMusterStore.setState({ error: 'again' });
    useMusterStore.getState().setConnection('live');
    expect(useMusterStore.getState().error).toBeNull();
  });

  it('preserves the error when going idle', () => {
    useMusterStore.setState({ error: 'boom' });
    useMusterStore.getState().setConnection('idle');
    expect(useMusterStore.getState().error).toBe('boom');
  });

  it('setStreamError records the message and flips to error', () => {
    useMusterStore.getState().setStreamError('stream HTTP 500');
    const s = useMusterStore.getState();
    expect(s.connection).toBe('error');
    expect(s.error).toBe('stream HTTP 500');
  });
});
