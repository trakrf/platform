// SSE transport for the reader live-feed (TRA-924). Replaces the direct-browser
// MQTT client: the backend now streams already-parsed, org-filtered reads over
// an authenticated SSE endpoint, so the browser holds no broker creds and only
// ever sees its own org's reads.
//
// We consume the stream with fetch() + ReadableStream rather than the native
// EventSource because EventSource can't set the Authorization header — and the
// endpoint rides the same JWT bearer as the REST API.

import type { ParsedRead } from '@/types/readerfeed';

/** Path appended to the API base URL (e.g. `/api/v1`) for the read stream. */
export const READ_STREAM_PATH = '/reads/stream';

export interface SSEParseState {
  buffer: string;
}

/**
 * Feed raw SSE text; returns any complete `data:` frames parsed into reads.
 * Comment lines (`:` heartbeats) are ignored, and an incomplete trailing frame
 * stays buffered in `state` for the next chunk. Never throws — malformed frames
 * are dropped.
 */
export function parseSSEChunk(state: SSEParseState, chunk: string): ParsedRead[] {
  state.buffer += chunk;
  const reads: ParsedRead[] = [];
  let sep: number;
  while ((sep = state.buffer.indexOf('\n\n')) !== -1) {
    const frame = state.buffer.slice(0, sep);
    state.buffer = state.buffer.slice(sep + 2);
    for (const line of frame.split('\n')) {
      if (!line.startsWith('data:')) continue;
      const json = line.slice(5).trim();
      if (!json) continue;
      try {
        const r = JSON.parse(json) as ParsedRead;
        if (typeof r.epc === 'string' && r.epc !== '') reads.push(r);
      } catch {
        // malformed frame; skip
      }
    }
  }
  return reads;
}

export interface ReadStreamCallbacks {
  onReads: (reads: ParsedRead[]) => void;
  onOpen: () => void;
  onError: (err: unknown) => void;
}

export interface ReadStreamHandle {
  close: () => void;
}

const INITIAL_BACKOFF_MS = 1000;
const MAX_BACKOFF_MS = 15_000;

/**
 * Open the org-scoped live-reads SSE stream over fetch (carries the JWT bearer).
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
          const reads = parseSSEChunk(state, decoder.decode(value, { stream: true }));
          if (reads.length > 0) opts.callbacks.onReads(reads);
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
