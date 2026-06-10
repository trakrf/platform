// Types for the reader live-feed / coverage diagnostic.
//
// As of TRA-936 the backend owns a server-authoritative tag-presence set and
// streams ItemTest-style "Inventory" deltas over SSE: a snapshot on connect (and
// as a periodic keyframe), then enter/update/leave transitions. Read count and
// RSSI aggregates are computed server-side (the only place that sees every read),
// so the browser is a thin reducer over TagState — it no longer aggregates raw
// reads itself.

/**
 * One tag's presence record, keyed server-side by (reader, epc, antenna). This
 * is the ItemTest Inventory row at its finest granularity — the client
 * aggregates per-antenna rows back to one (reader, epc) "overall" row by default
 * (TRA-937). `firstSeen`/`lastSeen` are server epoch-ms (immune to reader clock
 * skew); the staleness gradient and "time since last seen" derive from
 * `lastSeen` on a local tick — no network traffic animates the fade.
 */
export interface TagState {
  readerKey: string;
  epc: string;
  /** Resolved asset name, when known. Optional — asset-name resolution is a follow-up. */
  alias?: string;
  /** Antenna port — part of the row identity server-side. */
  antennaPort: number;
  /** Server epoch-ms of first sight. */
  firstSeen: number;
  /** Server epoch-ms of most recent sight. */
  lastSeen: number;
  readCount: number;
  lastRssi: number;
  rssiAvg: number;
  rssiMin: number;
  rssiMax: number;
}

/** Snapshot payload: full presence set + footer stats (ItemTest's Unique Tags + Read Rate). */
export interface SnapshotPayload {
  tags: TagState[];
  uniqueTags: number;
  readRate: number;
}

/** Leave payload: identifies the evicted tag, antenna included so the reducer
 *  deletes the right (reader,epc,antenna) row (TRA-937). */
export interface LeavePayload {
  readerKey: string;
  epc: string;
  antennaPort: number;
}

/**
 * A presence delta parsed off the SSE stream. `upsert` covers both first sight
 * and re-sight (same full-TagState payload, same client handling); the reducer
 * tells "new" from "seen" by map membership if it ever needs to.
 */
export type PresenceEvent =
  | { type: 'snapshot'; data: SnapshotPayload }
  | { type: 'upsert'; data: TagState }
  | { type: 'leave'; data: LeavePayload };

export type ReaderFeedStatus = 'connecting' | 'connected' | 'error' | 'closed';

/**
 * A row as rendered in the inventory table. The feed reduces deltas at the
 * finest (reader,epc,antenna) granularity; `toDisplayRows` turns that map into
 * rows for the current view — either one per antenna (split) or one aggregated
 * (reader,epc) row (the default "overall" view). `rowKey` is its stable identity
 * within the current view; `antennaLabel` is the port number, or `multi` when
 * more than one antenna folded into an aggregate row.
 */
export interface DisplayRow extends TagState {
  rowKey: string;
  antennaLabel: string;
}

/** Columns the inventory table can sort by (header click). */
export type SortKey =
  | 'epc'
  | 'readerKey'
  | 'antennaPort'
  | 'readCount'
  | 'lastRssi'
  | 'rssiAvg'
  | 'rssiMin'
  | 'rssiMax'
  | 'lastSeen';

export interface SortState {
  key: SortKey;
  dir: 'asc' | 'desc';
}
