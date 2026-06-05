// Pure parsing of CS463 reader MQTT payloads into device-agnostic reads.
// Mirrors the backend parser (internal/ingest/parser_cs463.go): rssi is a
// quoted string parsed leniently, timeStampOfRead is microseconds since epoch.
// Tolerant of bad input — never throws; malformed payloads yield [].

import type { CS463Payload, ParsedRead } from '@/types/readerfeed';

/** Extract the `{key}` from `trakrf.id/{key}/reads`; fall back to the topic. */
export function readerKeyFromTopic(topic: string): string {
  const m = /^trakrf\.id\/([^/]+)\/reads$/.exec(topic);
  return m ? m[1] : topic;
}

function toRssi(raw: string): number {
  const f = Number.parseFloat(raw);
  return Number.isFinite(f) ? Math.round(f) : 0;
}

/**
 * Parse one MQTT publish into zero or more reads. `raw` may be a string or the
 * Uint8Array/Buffer that mqtt.js delivers. Tags without an EPC are skipped.
 */
export function parseReaderPayload(topic: string, raw: string | Uint8Array): ParsedRead[] {
  const text = typeof raw === 'string' ? raw : new TextDecoder().decode(raw);
  let payload: CS463Payload;
  try {
    payload = JSON.parse(text) as CS463Payload;
  } catch {
    return [];
  }
  if (!payload || !Array.isArray(payload.tags)) return [];

  const readerKey = readerKeyFromTopic(topic);
  const reads: ParsedRead[] = [];
  for (const t of payload.tags) {
    if (!t || typeof t.epc !== 'string' || t.epc === '') continue;
    reads.push({
      epc: t.epc,
      readerKey,
      capturePointName: typeof t.capturePointName === 'string' ? t.capturePointName : '',
      antennaPort: Number.isFinite(t.antennaPort) ? t.antennaPort : 0,
      rssi: toRssi(typeof t.rssi === 'string' ? t.rssi : String(t.rssi ?? '')),
      readerTimestampMs: Number.isFinite(t.timeStampOfRead) ? Math.round(t.timeStampOfRead / 1000) : 0,
    });
  }
  return reads;
}
