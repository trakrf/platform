// Types for the reader live-feed / coverage diagnostic (TRA-902).
//
// The CS463 publishes to `trakrf.id/{key}/reads`. Payload shape mirrors the
// backend parser (`internal/ingest/parser_cs463.go`), verified against live
// preview traffic 2026-06-04: rssi is a quoted string, timeStampOfRead is
// microseconds since epoch.

/** Raw CS463 MQTT payload (one publish = one or more tag reads). */
export interface CS463Payload {
  tags?: CS463Tag[];
}

export interface CS463Tag {
  epc: string;
  timeStampOfRead: number; // microseconds since epoch
  antennaPort: number;
  capturePointName: string;
  rssi: string; // quoted, e.g. "-56"
}

/**
 * A single parsed read, device-agnostic. `rssi` is coerced to a number (0 on
 * unparseable). `readerTimestampMs` is the reader's own clock (informational —
 * display only). `readerKey` is the `{key}` segment of the source topic.
 */
export interface ParsedRead {
  epc: string;
  readerKey: string;
  capturePointName: string;
  antennaPort: number;
  rssi: number;
  readerTimestampMs: number;
}

/**
 * A read as held in the live store: the parsed read plus the browser receive
 * time. Age and expiry are derived from `receivedAt` (not the reader clock) so
 * the 15s aging primitive is immune to reader clock skew.
 */
export interface LiveRead extends ParsedRead {
  /** EPC — table row key. */
  id: string;
  /** Browser clock (ms) when this read was last observed. */
  receivedAt: number;
}

export type ReaderFeedStatus =
  | 'disabled' // no broker URL configured
  | 'connecting'
  | 'connected'
  | 'error'
  | 'closed';
