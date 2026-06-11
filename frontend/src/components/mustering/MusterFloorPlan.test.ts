// Unit tests for the MusterFloorPlan pin-badge math (TRA-978 phase 7).
// Tests the exported pure function only — no React rendering needed (mirrors
// MusterReport.test.ts's CSV-helper approach).

import { describe, it, expect } from 'vitest';
import { pinBadge } from './MusterFloorPlan';
import type { MusterEvent, MusterEntry, ZonePresence } from '@/types/mustering';

const zones: ZonePresence[] = [
  { location_id: 100, name: 'Production Floor', muster_point: false, count: 5 },
  { location_id: 101, name: 'Warehouse', muster_point: false, count: 2 },
  { location_id: 200, name: 'Muster Point A', muster_point: true, count: 0 },
];

function entry(over: Partial<MusterEntry>): MusterEntry {
  return {
    id: 1,
    org_id: 1,
    muster_event_id: 1,
    asset_id: 10,
    label: 'Operator 001',
    status: 'missing',
    created_at: '2026-06-11T10:00:00Z',
    updated_at: '2026-06-11T10:00:00Z',
    ...over,
  };
}

function activeEvent(entries: MusterEntry[]): MusterEvent {
  return {
    id: 42,
    org_id: 1,
    status: 'active',
    started_at: '2026-06-11T10:00:00Z',
    window_minutes: 15,
    counts: { expected: entries.length, missing: 0, at_muster: 0, verified: 0, safe_manual: 0 },
    entries,
    created_at: '2026-06-11T10:00:00Z',
    updated_at: '2026-06-11T10:00:00Z',
  };
}

describe('pinBadge', () => {
  it('presence mode: zone pin shows live headcount (present)', () => {
    expect(pinBadge(100, zones, null)).toEqual({ kind: 'present', count: 5 });
  });

  it('presence mode: muster point shows its headcount too', () => {
    expect(pinBadge(200, zones, null)).toEqual({ kind: 'present', count: 0 });
  });

  it('presence mode: unknown location → present 0', () => {
    expect(pinBadge(999, zones, null)).toEqual({ kind: 'present', count: 0 });
  });

  it('completed event is treated as presence mode', () => {
    const ev = { ...activeEvent([]), status: 'completed' as const };
    expect(pinBadge(101, zones, ev)).toEqual({ kind: 'present', count: 2 });
  });

  it('drill mode: zone pin shows MISSING count by expected_location_id', () => {
    const ev = activeEvent([
      entry({ id: 1, status: 'missing', expected_location_id: 100 }),
      entry({ id: 2, status: 'missing', expected_location_id: 100 }),
      entry({ id: 3, status: 'at_muster', expected_location_id: 100, muster_location_id: 200 }),
      entry({ id: 4, status: 'missing', expected_location_id: 101 }),
    ]);
    expect(pinBadge(100, zones, ev)).toEqual({ kind: 'missing', count: 2 });
    expect(pinBadge(101, zones, ev)).toEqual({ kind: 'missing', count: 1 });
  });

  it('drill mode: muster-point pin shows arrivals (at_muster + verified) by muster_location_id', () => {
    const ev = activeEvent([
      entry({ id: 1, status: 'at_muster', muster_location_id: 200 }),
      entry({ id: 2, status: 'verified', muster_location_id: 200 }),
      entry({ id: 3, status: 'missing', expected_location_id: 100 }),
      entry({ id: 4, status: 'safe_manual', muster_location_id: 200 }), // not counted as arrival
    ]);
    expect(pinBadge(200, zones, ev)).toEqual({ kind: 'arrivals', count: 2 });
  });

  it('drill mode: muster-point arrivals scoped to the matching point', () => {
    const ev = activeEvent([
      entry({ id: 1, status: 'at_muster', muster_location_id: 999 }), // other point
    ]);
    expect(pinBadge(200, zones, ev)).toEqual({ kind: 'arrivals', count: 0 });
  });
});
