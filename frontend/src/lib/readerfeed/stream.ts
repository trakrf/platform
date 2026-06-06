// SSE transport for the tag-presence feed (TRA-936, originally TRA-924). The
// backend streams already-parsed, org-filtered presence deltas over an
// authenticated SSE endpoint, so the browser holds no broker creds and only ever
// sees its own org's tags.
//
// We consume the stream with fetch() + ReadableStream rather than the native
// EventSource because EventSource can't set the Authorization header — and the
// endpoint rides the same JWT bearer as the REST API.

import type { LeavePayload, PresenceEvent, SnapshotPayload, TagState } from '@/types/readerfeed';

/** Path appended to the API base URL (e.g. `/api/v1`) for the read stream. */
export const READ_STREAM_PATH = '/reads/stream';

export interface SSEParseState {
  buffer: string;
}

/** Validate + shape a raw (eventType, data) pair into a typed PresenceEvent, or
 *  null if the type is unknown or the payload is malformed. */
function toEvent(type: string, data: unknown): PresenceEvent | null {
  const d = data as Record<string, unknown>;
  switch (type) {
    case 'snapshot':
      if (Array.isArray((d as { tags?: unknown }).tags)) {
        return { type: 'snapshot', data: data as SnapshotPayload };
      }
      return null;
    case 'enter':
    case 'update':
      if (typeof d.epc === 'string' && d.epc !== '' && typeof d.readerKey === 'string') {
        return { type, data: data as TagState };
      }
      return null;
    case 'leave':
      if (typeof d.epc === 'string' && typeof d.readerKey === 'string') {
        return { type: 'leave', data: data as LeavePayload };
      }
      return null;
    default:
      return null;
  }
}

/**
 * Feed raw SSE text; returns any complete frames parsed into presence events.
 * A frame's `event:` line names the type and its `data:` line carries JSON.
 * Comment lines (`:` heartbeats) are ignored, an incomplete trailing frame stays
 * buffered for the next chunk, and malformed/unknown frames are dropped. Never
 * throws.
 */
export function parseSSEChunk(state: SSEParseState, chunk: string): PresenceEvent[] {
  state.buffer += chunk;
  const events: PresenceEvent[] = [];
  let sep: number;
  while ((sep = state.buffer.indexOf('\n\n')) !== -1) {
    const frame = state.buffer.slice(0, sep);
    state.buffer = state.buffer.slice(sep + 2);

    let type = '';
    let dataLine = '';
    for (const line of frame.split('\n')) {
      if (line.startsWith('event:')) type = line.slice(6).trim();
      else if (line.startsWith('data:')) dataLine = line.slice(5).trim();
    }
    if (!type || !dataLine) continue; // heartbeat/comment or partial

    try {
      const ev = toEvent(type, JSON.parse(dataLine));
      if (ev) events.push(ev);
    } catch {
      // malformed frame; skip
    }
  }
  return events;
}

export interface ReadStreamCallbacks {
  onEvents: (events: PresenceEvent[]) => void;
  onOpen: () => void;
  onError: (err: unknown) => void;
}

export interface ReadStreamHandle {
  close: () => void;
}

const INITIAL_BACKOFF_MS = 1000;
const MAX_BACKOFF_MS = 15_000;

/**
 * Open the org-scoped presence SSE stream over fetch (carries the JWT bearer).
 * Auto-reconnects with exponential backoff. On a 401 it calls onUnauthorized
 * (which should refresh the token) and retries immediately if that succeeds.
 */
export function openReadStream(opts: {
  baseURL: string;
  getToken: () => string | null;
  onUnauthorized: () => Promise<boolean>;
  callbacks: ReadStreamCallbacks;
}): ReadStreamHandle {
  let closed = false;
  let controller: AbortController | null = null;
  let backoff = INITIAL_BACKOFF_MS;

  const run = async () => {
    while (!closed) {
      controller = new AbortController();
      const state: SSEParseState = { buffer: '' };
      try {
        const token = opts.getToken();
        const resp = await fetch(opts.baseURL + READ_STREAM_PATH, {
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
          const events = parseSSEChunk(state, decoder.decode(value, { stream: true }));
          if (events.length > 0) opts.callbacks.onEvents(events);
        }
      } catch (err) {
        if (closed) return;
        opts.callbacks.onError(err);
      }

      if (closed) return;
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
