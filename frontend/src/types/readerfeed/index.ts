// Types for the reader live-feed / coverage diagnostic.
//
// As of TRA-936 the backend owns a server-authoritative tag-presence set and
// streams ItemTest-style "Inventory" deltas over SSE: a snapshot on connect (and
// as a periodic keyframe), then enter/update/leave transitions. Read count and
// RSSI aggregates are computed server-side (the only place that sees every read),
// so the browser is a thin reducer over TagState — it no longer aggregates raw
// reads itself.

/**
 * One tag's presence record, keyed server-side by (reader, epc). This is the
 * ItemTest Inventory row. `firstSeen`/`lastSeen` are server epoch-ms (immune to
 * reader clock skew); the staleness gradient and "time since last seen" derive
 * from `lastSeen` on a local tick — no network traffic animates the fade.
 */
export interface TagState {
  readerKey: string;
  epc: string;
  /** Resolved asset name, when known. Optional — asset-name resolution is a follow-up. */
  alias?: string;
  capturePointName: string;
  /** Most recent antenna port. */
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

/** Leave payload: identifies the evicted tag. */
export interface LeavePayload {
  readerKey: string;
  epc: string;
}

/** A presence delta parsed off the SSE stream. */
export type PresenceEvent =
  | { type: 'snapshot'; data: SnapshotPayload }
  | { type: 'enter'; data: TagState }
  | { type: 'update'; data: TagState }
  | { type: 'leave'; data: LeavePayload };

export type ReaderFeedStatus = 'connecting' | 'connected' | 'error' | 'closed';
