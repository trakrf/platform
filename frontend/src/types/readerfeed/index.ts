// Types for the reader live-feed / coverage diagnostic.
//
// As of TRA-924 reads arrive already parsed and org-filtered from the backend
// SSE proxy (each SSE `data:` frame is one ParsedRead). The CS463 wire payload
// is parsed server-side now (internal/ingest/parser_cs463.go), so the browser
// no longer sees raw broker payloads.

/**
 * A single parsed read, device-agnostic. `rssi` is a number (0 when unknown).
 * `readerTimestampMs` is the reader's own clock (informational — display only).
 * `readerKey` is the `{key}` segment of the source topic.
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

export type ReaderFeedStatus = 'connecting' | 'connected' | 'error' | 'closed';
