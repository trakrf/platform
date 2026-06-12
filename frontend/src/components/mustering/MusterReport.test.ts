// Unit tests for MusterReport CSV export helpers (TRA-978 Phase 4).
// Tests are purely over the exported pure functions — no React rendering needed.

import { describe, it, expect } from 'vitest';
import { csvCell, buildCsvRows, buildReportCsv } from './MusterReport';
import type { MusterEvent, MusterEntry } from '@/types/mustering';

// ---- fixtures ----

function makeEntry(over: Partial<MusterEntry> = {}): MusterEntry {
  return {
    id: 1,
    org_id: 1,
    muster_event_id: 1,
    asset_id: 10,
    label: 'Operator 001',
    status: 'verified',
    created_at: '2026-06-11T10:00:00Z',
    updated_at: '2026-06-11T10:05:00Z',
    ...over,
  };
}

function makeEvent(over: Partial<MusterEvent> = {}): MusterEvent {
  return {
    id: 42,
    org_id: 1,
    status: 'completed',
    started_at: '2026-06-11T10:00:00Z',
    ended_at: '2026-06-11T10:10:00Z',
    window_minutes: 15,
    counts: { expected: 2, missing: 0, at_muster: 0, verified: 2, safe_manual: 0 },
    report: {
      total_seconds: 600,
      counts: { expected: 2, missing: 0, at_muster: 0, verified: 2, safe_manual: 0 },
      zones: [{ location_id: 100, name: 'Production Floor', expected: 1, accounted: 1, cleared_at: '2026-06-11T10:05:00Z' }],
      muster_points: [{ location_id: 200, name: 'Muster Point A', arrivals: 2 }],
    },
    entries: [
      makeEntry({ id: 1, asset_id: 10, label: 'Operator 001', status: 'verified', verified_at: '2026-06-11T10:05:00Z' }),
      makeEntry({ id: 2, asset_id: 11, label: 'Operator 002', status: 'missing' }),
    ],
    created_at: '2026-06-11T10:00:00Z',
    updated_at: '2026-06-11T10:10:00Z',
    ...over,
  };
}

// ---- csvCell ----

describe('csvCell', () => {
  it('returns plain strings as-is when no escaping needed', () => {
    expect(csvCell('hello')).toBe('hello');
    expect(csvCell(42)).toBe('42');
    expect(csvCell('')).toBe('');
    expect(csvCell(null)).toBe('');
    expect(csvCell(undefined)).toBe('');
  });

  it('wraps strings containing a comma in double-quotes', () => {
    expect(csvCell('a,b')).toBe('"a,b"');
  });

  it('wraps strings containing a double-quote and escapes the inner quote', () => {
    expect(csvCell('say "hello"')).toBe('"say ""hello"""');
  });

  it('wraps strings containing a newline', () => {
    expect(csvCell('line1\nline2')).toBe('"line1\nline2"');
  });
});

// ---- buildCsvRows ----

describe('buildCsvRows', () => {
  const zoneMap = new Map<number, string>([
    [100, 'Production Floor'],
    [200, 'Muster Point A'],
  ]);

  it('maps entry fields correctly', () => {
    const entries = [
      makeEntry({
        label: 'Alice',
        status: 'verified',
        expected_location_id: 100,
        muster_location_id: 200,
        first_muster_seen_at: '2026-06-11T10:03:00Z',
        verified_at: '2026-06-11T10:05:00Z',
        marked_safe_note: '',
      }),
    ];
    const rows = buildCsvRows(entries, zoneMap);
    expect(rows).toHaveLength(1);
    const row = rows[0];
    expect(row.person).toBe('Alice');
    expect(row.status).toBe('verified');
    expect(row.expected_zone).toBe('Production Floor');
    expect(row.muster_location).toBe('Muster Point A');
    expect(row.first_muster_seen_at).toBe('2026-06-11T10:03:00Z');
    expect(row.verified_at).toBe('2026-06-11T10:05:00Z');
    expect(row.note).toBe('');
  });

  it('uses location_id as fallback when name not in map', () => {
    const entries = [makeEntry({ expected_location_id: 999, muster_location_id: 888 })];
    const rows = buildCsvRows(entries, new Map());
    expect(rows[0].expected_zone).toBe('999');
    expect(rows[0].muster_location).toBe('888');
  });

  it('leaves zone/muster fields empty when ids are absent', () => {
    const entries = [makeEntry({ expected_location_id: undefined, muster_location_id: undefined })];
    const rows = buildCsvRows(entries, zoneMap);
    expect(rows[0].expected_zone).toBe('');
    expect(rows[0].muster_location).toBe('');
  });
});

// ---- buildReportCsv ----

describe('buildReportCsv', () => {
  it('includes event id in the summary header', () => {
    const ev = makeEvent();
    const csv = buildReportCsv(ev, new Map([[100, 'Production Floor'], [200, 'Muster Point A']]));
    expect(csv).toContain('# Muster Report — Event #42');
  });

  it('includes a per-person data row for each entry', () => {
    const ev = makeEvent();
    const csv = buildReportCsv(ev, new Map());
    expect(csv).toContain('Operator 001');
    expect(csv).toContain('Operator 002');
  });

  it('includes zone breakdown rows', () => {
    const ev = makeEvent();
    const csv = buildReportCsv(ev, new Map([[100, 'Production Floor'], [200, 'Muster Point A']]));
    expect(csv).toContain('Production Floor');
    expect(csv).toContain('# Zone Breakdown');
  });

  it('includes muster-point arrivals', () => {
    const ev = makeEvent();
    const csv = buildReportCsv(ev, new Map([[200, 'Muster Point A']]));
    expect(csv).toContain('Muster Point A');
    expect(csv).toContain('# Muster Point Arrivals');
  });

  it('includes the person-column header row', () => {
    const ev = makeEvent();
    const csv = buildReportCsv(ev, new Map());
    expect(csv).toContain('Person,Status,Expected Zone,Muster Location');
  });

  it('produces valid CRLF-delimited content', () => {
    const ev = makeEvent();
    const csv = buildReportCsv(ev, new Map());
    expect(csv).toMatch(/\r\n/);
  });

  it('handles events without a report gracefully', () => {
    const ev = makeEvent({ report: undefined, entries: [makeEntry()] });
    const csv = buildReportCsv(ev, new Map());
    // Should not throw and must contain at least the summary line.
    expect(csv).toContain('# Muster Report — Event #42');
    expect(csv).toContain('Operator 001');
  });
});
