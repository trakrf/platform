// musterStore (TRA-978 POC).
//
// Holds presence + active-event state for the Mustering screen, fed by the
// org-scoped mustering SSE stream (GET /api/v1/mustering/stream). The SSE
// transport lives in openMusterStream() below (the TRA-924 pattern — fetch +
// ReadableStream, NOT EventSource, which can't set the Authorization header);
// the store is purely the reducer target. useMusterFeed owns the stream
// lifecycle and reconnects on org switch. REST actions wrap musteringApi.
//
// Frame contract (see backend/internal/handlers/mustering/stream.go):
//   event: snapshot  data {zones, persons_on_site, event|null}
//   event: presence  data {zones, persons_on_site, persons}  (persons only while event active)
//   event: entry     data {entry, counts}
//   event: event     data {event}           (activated/completed/cancelled)
// Heartbeats are SSE comments or empty events — parsed defensively (dropped).

import { create } from 'zustand';
import { createStoreWithTracking } from './createStore';
import { musteringApi } from '@/lib/api/mustering';
import type {
  MusterEvent,
  MusterEntry,
  MusterCounts,
  ZonePresence,
  MusterSighting,
  MusterSnapshotPayload,
  MusterPresencePayload,
  MusterEntryPayload,
  MusterEventPayload,
} from '@/types/mustering';

const STREAM_PATH = '/mustering/stream';
const INITIAL_BACKOFF_MS = 1000;
const MAX_BACKOFF_MS = 15_000;

export type MusterConnection = 'idle' | 'connecting' | 'live' | 'error';

interface MusterState {
  zones: ZonePresence[];
  personsOnSite: number;
  event: MusterEvent | null;
  connection: MusterConnection;
  // Break-glass: missing-row last-known locations are only revealed in the UI
  // after an explicit unlock(). Reset whenever the event id changes or ends.
  revealUnlocked: boolean;
  error: string | null;

  // Stream → store reducer surface (driven by useMusterFeed via openMusterStream).
  applyFrame: (frame: MusterFrame) => void;
  setConnection: (connection: MusterConnection) => void;
  setStreamError: (message: string) => void;

  refreshStatus: () => Promise<void>;

  activate: (windowMinutes?: number) => Promise<void>;
  allClear: () => Promise<MusterEvent | null>;
  cancel: () => Promise<void>;
  verify: (entryId: number) => Promise<void>;
  markSafe: (entryId: number, note?: string) => Promise<void>;
  unlock: () => Promise<void>;
  simulate: (sightings: MusterSighting[]) => Promise<void>;
  seed: () => Promise<void>;
  fetchEvents: () => Promise<MusterEvent[]>;
  fetchEvent: (id: number) => Promise<MusterEvent>;
}

// --- SSE frame parsing (mirrors lib/readerfeed/stream.ts) ---

export interface SSEParseState {
  buffer: string;
}

export type MusterFrame =
  | { type: 'snapshot'; data: MusterSnapshotPayload }
  | { type: 'presence'; data: MusterPresencePayload }
  | { type: 'entry'; data: MusterEntryPayload }
  | { type: 'event'; data: MusterEventPayload };

function toFrame(type: string, data: unknown): MusterFrame | null {
  const d = data as Record<string, unknown>;
  switch (type) {
    case 'snapshot':
      if (Array.isArray(d.zones)) return { type: 'snapshot', data: data as MusterSnapshotPayload };
      return null;
    case 'presence':
      if (Array.isArray(d.zones)) return { type: 'presence', data: data as MusterPresencePayload };
      return null;
    case 'entry':
      if (d.entry && typeof d.entry === 'object') {
        return { type: 'entry', data: data as MusterEntryPayload };
      }
      return null;
    case 'event':
      if (d.event && typeof d.event === 'object') {
        return { type: 'event', data: data as MusterEventPayload };
      }
      return null;
    default:
      return null;
  }
}

/**
 * Feed raw SSE text; returns any complete frames parsed into muster frames.
 * Comment/heartbeat lines and incomplete trailing frames stay buffered;
 * malformed/unknown frames are dropped. Never throws.
 */
export function parseSSEChunk(state: SSEParseState, chunk: string): MusterFrame[] {
  state.buffer += chunk;
  const frames: MusterFrame[] = [];
  let sep: number;
  while ((sep = state.buffer.indexOf('\n\n')) !== -1) {
    const raw = state.buffer.slice(0, sep);
    state.buffer = state.buffer.slice(sep + 2);

    let type = '';
    let dataLine = '';
    for (const line of raw.split('\n')) {
      if (line.startsWith('event:')) type = line.slice(6).trim();
      else if (line.startsWith('data:')) dataLine = line.slice(5).trim();
    }
    if (!type || !dataLine) continue; // heartbeat/comment or partial

    try {
      const f = toFrame(type, JSON.parse(dataLine));
      if (f) frames.push(f);
    } catch {
      // malformed frame; skip
    }
  }
  return frames;
}

// --- SSE transport ---

export interface MusterStreamCallbacks {
  onFrames: (frames: MusterFrame[]) => void;
  onOpen: () => void;
  /** Backoff retry is about to start (after the first connect failed). */
  onRetry: () => void;
  onError: (err: unknown) => void;
}

export interface MusterStreamHandle {
  close: () => void;
}

/**
 * Open the org-scoped mustering SSE stream over fetch (carries the JWT bearer).
 * Auto-reconnects with exponential backoff. On a 401 it calls onUnauthorized
 * (which should refresh the token) and retries immediately if that succeeds.
 *
 * All lifecycle state (closed flag, abort controller, backoff) is closure-scoped
 * per invocation — never module-global — so a torn-down stream can never alias a
 * freshly opened one's state under StrictMode double-mount or rapid remount.
 */
export function openMusterStream(opts: {
  baseURL: string;
  getToken: () => string | null;
  onUnauthorized: () => Promise<boolean>;
  callbacks: MusterStreamCallbacks;
}): MusterStreamHandle {
  let closed = false;
  let controller: AbortController | null = null;
  let backoff = INITIAL_BACKOFF_MS;

  const run = async () => {
    while (!closed) {
      controller = new AbortController();
      const state: SSEParseState = { buffer: '' };
      try {
        const token = opts.getToken();
        const resp = await fetch(opts.baseURL + STREAM_PATH, {
          headers: {
            Accept: 'text/event-stream',
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          signal: controller.signal,
        });

        if (resp.status === 401) {
          const refreshed = await opts.onUnauthorized();
          if (!refreshed) throw new Error('unauthorized');
          continue; // retry immediately with the refreshed token
        }
        if (!resp.ok || !resp.body) throw new Error(`stream HTTP ${resp.status}`);

        opts.callbacks.onOpen();
        backoff = INITIAL_BACKOFF_MS;

        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          const frames = parseSSEChunk(state, decoder.decode(value, { stream: true }));
          if (frames.length > 0) opts.callbacks.onFrames(frames);
        }
      } catch (err) {
        if (closed) return;
        opts.callbacks.onError(err);
      }

      if (closed) return;
      // A retry is starting — flip the indicator back to "connecting" so a
      // dropped-then-reconnecting stream isn't stuck showing the error state.
      opts.callbacks.onRetry();
      await new Promise((r) => setTimeout(r, backoff));
      backoff = Math.min(backoff * 2, MAX_BACKOFF_MS);
    }
  };
  void run();

  return {
    close: () => {
      closed = true;
      controller?.abort();
    },
  };
}

// Merge an updated entry into the active event's entries list (in place by id).
function applyEntryToEvent(
  event: MusterEvent | null,
  entry: MusterEntry,
  counts: MusterCounts,
): MusterEvent | null {
  if (!event) return event;
  const entries = event.entries ? [...event.entries] : [];
  const idx = entries.findIndex((e) => e.id === entry.id);
  if (idx === -1) entries.push(entry);
  else entries[idx] = entry;
  return { ...event, entries, counts };
}

function readableError(err: unknown): string {
  if (typeof err === 'object' && err !== null) {
    const anyErr = err as {
      response?: { status?: number; data?: { detail?: string; title?: string; message?: string } };
      message?: string;
    };
    const status = anyErr.response?.status;
    const body = anyErr.response?.data;
    if (status === 409) {
      return body?.detail || body?.title || 'A muster event is already active.';
    }
    if (body?.detail) return body.detail;
    if (body?.title) return body.title;
    if (body?.message) return body.message;
    if (anyErr.message) return anyErr.message;
  }
  return 'Mustering request failed.';
}

export const useMusterStore = create<MusterState>()(
  createStoreWithTracking<MusterState>(
    (set, get) => {
      // Apply a parsed frame into store state.
      const applyFrame = (frame: MusterFrame) => {
        switch (frame.type) {
          case 'snapshot': {
            const incoming = frame.data.event;
            const prev = get().event;
            const reset = !incoming || (prev && incoming && prev.id !== incoming.id) || incoming.status !== 'active';
            set({
              zones: frame.data.zones,
              personsOnSite: frame.data.persons_on_site,
              event: incoming,
              ...(reset ? { revealUnlocked: false } : {}),
            });
            break;
          }
          case 'presence':
            set({ zones: frame.data.zones, personsOnSite: frame.data.persons_on_site });
            break;
          case 'entry': {
            const next = applyEntryToEvent(get().event, frame.data.entry, frame.data.counts);
            set({ event: next });
            break;
          }
          case 'event': {
            const incoming = frame.data.event;
            const prev = get().event;
            const idChanged = !prev || prev.id !== incoming.id;
            const ended = incoming.status !== 'active';
            set({
              event: incoming,
              ...(idChanged || ended ? { revealUnlocked: false } : {}),
            });
            break;
          }
        }
      };

      return {
        zones: [],
        personsOnSite: 0,
        event: null,
        connection: 'idle',
        revealUnlocked: false,
        error: null,

        applyFrame,

        setConnection: (connection) => {
          // A live/connecting transition clears any prior stream error.
          if (connection === 'live' || connection === 'connecting') {
            set({ connection, error: null });
          } else {
            set({ connection });
          }
        },

        setStreamError: (message) => set({ connection: 'error', error: message }),

        refreshStatus: async () => {
          try {
            const { data } = await musteringApi.status();
            const incoming = data.event;
            const prev = get().event;
            const reset = !incoming || (prev && incoming && prev.id !== incoming.id) || incoming.status !== 'active';
            set({
              // Defensive: a fresh org with no locations can serialize zones as
              // null (Go nil slice). Coerce to [] so the Dashboard useMemo that
              // iterates zones never throws (mirrors the SSE path's toFrame guard).
              zones: Array.isArray(data.zones) ? data.zones : [],
              personsOnSite: data.persons_on_site ?? 0,
              event: incoming,
              error: null,
              ...(reset ? { revealUnlocked: false } : {}),
            });
          } catch (err) {
            set({ error: readableError(err) });
          }
        },

        activate: async (windowMinutes?: number) => {
          try {
            const { data } = await musteringApi.activate(windowMinutes);
            set({ event: data.data, revealUnlocked: false, error: null });
          } catch (err) {
            const msg = readableError(err);
            set({ error: msg });
            throw new Error(msg);
          }
        },

        allClear: async () => {
          const ev = get().event;
          if (!ev) return null;
          try {
            const { data } = await musteringApi.allClear(ev.id);
            set({ event: data.data, revealUnlocked: false, error: null });
            return data.data;
          } catch (err) {
            const msg = readableError(err);
            set({ error: msg });
            throw new Error(msg);
          }
        },

        cancel: async () => {
          const ev = get().event;
          if (!ev) return;
          try {
            const { data } = await musteringApi.cancel(ev.id);
            set({ event: data.data, revealUnlocked: false, error: null });
          } catch (err) {
            const msg = readableError(err);
            set({ error: msg });
            throw new Error(msg);
          }
        },

        verify: async (entryId: number) => {
          const ev = get().event;
          if (!ev) return;
          try {
            const { data } = await musteringApi.updateEntry(ev.id, entryId, 'verify');
            set({ event: applyEntryToEvent(get().event, data.data.entry, data.data.counts), error: null });
          } catch (err) {
            const msg = readableError(err);
            set({ error: msg });
            throw new Error(msg);
          }
        },

        markSafe: async (entryId: number, note?: string) => {
          const ev = get().event;
          if (!ev) return;
          try {
            const { data } = await musteringApi.updateEntry(ev.id, entryId, 'mark_safe', note);
            set({ event: applyEntryToEvent(get().event, data.data.entry, data.data.counts), error: null });
          } catch (err) {
            const msg = readableError(err);
            set({ error: msg });
            throw new Error(msg);
          }
        },

        unlock: async () => {
          const ev = get().event;
          if (!ev) {
            set({ revealUnlocked: true });
            return;
          }
          try {
            // POST returns {unlocked:true}, not the event — flip the UI flag,
            // then re-snapshot so entries' last_seen_location_id populate (the
            // break-glass reveal). The active-event guard in refreshStatus
            // preserves revealUnlocked for the same event id.
            await musteringApi.unlock(ev.id);
            set({ revealUnlocked: true, error: null });
            await get().refreshStatus();
            set({ revealUnlocked: true });
          } catch (err) {
            const msg = readableError(err);
            set({ error: msg });
            throw new Error(msg);
          }
        },

        simulate: async (sightings: MusterSighting[]) => {
          try {
            await musteringApi.simulate(sightings);
            // Transitions arrive over SSE; re-snapshot as a safety net.
            await get().refreshStatus();
          } catch (err) {
            const msg = readableError(err);
            set({ error: msg });
            throw new Error(msg);
          }
        },

        seed: async () => {
          try {
            await musteringApi.seed();
            await get().refreshStatus();
          } catch (err) {
            const msg = readableError(err);
            set({ error: msg });
            throw new Error(msg);
          }
        },

        fetchEvents: async () => {
          const { data } = await musteringApi.listEvents();
          return data.data;
        },

        fetchEvent: async (id: number) => {
          const { data } = await musteringApi.getEvent(id);
          return data.data;
        },
      };
    },
    'musterStore',
  ),
);
