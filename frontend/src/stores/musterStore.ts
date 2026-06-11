// musterStore (TRA-978 POC).
//
// Holds presence + active-event state for the Mustering screen, fed by the
// org-scoped mustering SSE stream (GET /api/v1/mustering/stream) consumed via
// authenticated fetch + ReadableStream (the TRA-924 pattern — NOT EventSource,
// which can't set the Authorization header). REST actions wrap musteringApi.
//
// Frame contract (see backend/internal/handlers/mustering/stream.go):
//   event: snapshot  data {zones, persons_on_site, event|null}
//   event: presence  data {zones, persons}  (persons only while event active)
//   event: entry     data {entry, counts}
//   event: event     data {event}           (activated/completed/cancelled)
// Heartbeats are SSE comments or empty events — parsed defensively (dropped).

import { create } from 'zustand';
import { createStoreWithTracking } from './createStore';
import { useAuthStore } from './authStore';
import { API_BASE_URL } from '@/lib/api/client';
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

  connect: () => void;
  disconnect: () => void;
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

interface ParseState {
  buffer: string;
}

type MusterFrame =
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

function parseSSEChunk(state: ParseState, chunk: string): MusterFrame[] {
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

// Merge an updated entry into the active event's entries list (in place by id).
function applyEntry(event: MusterEvent | null, entry: MusterEntry, counts: MusterCounts): MusterEvent | null {
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

// Module-level stream handle so connect/disconnect survive re-renders.
let streamClosed = true;
let streamController: AbortController | null = null;

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
            set({ zones: frame.data.zones });
            break;
          case 'entry': {
            const next = applyEntry(get().event, frame.data.entry, frame.data.counts);
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

      const runStream = async () => {
        let backoff = INITIAL_BACKOFF_MS;
        while (!streamClosed) {
          streamController = new AbortController();
          const parseState: ParseState = { buffer: '' };
          try {
            const token = useAuthStore.getState().token;
            const resp = await fetch(API_BASE_URL + STREAM_PATH, {
              headers: {
                Accept: 'text/event-stream',
                ...(token ? { Authorization: `Bearer ${token}` } : {}),
              },
              signal: streamController.signal,
            });

            if (resp.status === 401) {
              const refreshed = await useAuthStore.getState().refresh();
              if (!refreshed) throw new Error('unauthorized');
              continue; // retry immediately with the refreshed token
            }
            if (!resp.ok || !resp.body) throw new Error(`stream HTTP ${resp.status}`);

            // Reconnect → re-snapshot, so we never run on stale presence after
            // a dropped connection.
            set({ connection: 'live', error: null });
            backoff = INITIAL_BACKOFF_MS;
            void get().refreshStatus();

            const reader = resp.body.getReader();
            const decoder = new TextDecoder();
            for (;;) {
              const { value, done } = await reader.read();
              if (done) break;
              const frames = parseSSEChunk(parseState, decoder.decode(value, { stream: true }));
              for (const f of frames) applyFrame(f);
            }
          } catch (err) {
            if (streamClosed) return;
            set({ connection: 'error', error: err instanceof Error ? err.message : 'stream error' });
          }

          if (streamClosed) return;
          await new Promise((r) => setTimeout(r, backoff));
          backoff = Math.min(backoff * 2, MAX_BACKOFF_MS);
        }
      };

      return {
        zones: [],
        personsOnSite: 0,
        event: null,
        connection: 'idle',
        revealUnlocked: false,
        error: null,

        connect: () => {
          if (!streamClosed) return; // already running
          streamClosed = false;
          set({ connection: 'connecting', error: null });
          void runStream();
        },

        disconnect: () => {
          streamClosed = true;
          streamController?.abort();
          streamController = null;
          set({ connection: 'idle' });
        },

        refreshStatus: async () => {
          try {
            const { data } = await musteringApi.status();
            const incoming = data.event;
            const prev = get().event;
            const reset = !incoming || (prev && incoming && prev.id !== incoming.id) || incoming.status !== 'active';
            set({
              zones: data.zones,
              personsOnSite: data.persons_on_site,
              event: incoming,
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
            set({ event: applyEntry(get().event, data.data.entry, data.data.counts), error: null });
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
            set({ event: applyEntry(get().event, data.data.entry, data.data.counts), error: null });
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
