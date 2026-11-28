import { describe, expect, it } from 'vitest';
import type { MapOccupant, MapPresence } from '@/types/locationMaps';
import type { Location } from '@/types/locations';
import { dataUrlToBase64 } from '@/lib/api/locationMaps';
import { descendantsOf, filterPresence, pinPercentFromClick } from './mapPresence';

const occ = (asset_id: number, name: string, is_person: boolean): MapOccupant => ({
  asset_id,
  name,
  external_key: `K-${asset_id}`,
  is_person,
  location_id: 3,
  location_name: 'Room A',
  last_seen: '2026-11-27T12:00:00Z',
});

const presence: MapPresence = {
  map_location_id: 1,
  generated_at: '2026-11-27T12:00:00Z',
  zones: [
    { location_id: 3, name: 'Room A', x_pct: 10, y_pct: 10, occupants: [occ(1, 'Alice badge', true), occ(2, 'Forklift', false)] },
    { location_id: 4, name: 'Room B', x_pct: 90, y_pct: 10, occupants: [occ(3, 'Bob badge', true)] },
  ],
  unplaced: [occ(4, 'Pallet 7', false)],
};

describe('filterPresence', () => {
  it('keeps everyone for all + empty query', () => {
    expect(filterPresence(presence, { kind: 'all', q: '' })).toEqual(presence);
  });

  it('filters people and assets, in zones and unplaced', () => {
    const people = filterPresence(presence, { kind: 'people', q: '' });
    expect(people.zones[0].occupants.map((o) => o.asset_id)).toEqual([1]);
    expect(people.zones[1].occupants.map((o) => o.asset_id)).toEqual([3]);
    expect(people.unplaced).toEqual([]);

    const assets = filterPresence(presence, { kind: 'assets', q: '' });
    expect(assets.zones[0].occupants.map((o) => o.asset_id)).toEqual([2]);
    expect(assets.unplaced.map((o) => o.asset_id)).toEqual([4]);
  });

  it('matches the query case-insensitively on name or external key, keeping every zone', () => {
    const q = filterPresence(presence, { kind: 'all', q: 'BADGE' });
    expect(q.zones).toHaveLength(2);
    expect(q.zones[0].occupants.map((o) => o.asset_id)).toEqual([1]);
    expect(filterPresence(presence, { kind: 'all', q: 'k-4' }).unplaced).toHaveLength(1);
  });
});

describe('pinPercentFromClick', () => {
  const rect = { left: 100, top: 50, width: 400, height: 200 };
  it('converts to percent, rounded to 2 dp', () => {
    expect(pinPercentFromClick(rect, 200, 100)).toEqual({ x_pct: 25, y_pct: 25 });
    expect(pinPercentFromClick(rect, 101, 51)).toEqual({ x_pct: 0.25, y_pct: 0.5 });
  });
  it('clamps to 0..100', () => {
    expect(pinPercentFromClick(rect, 0, 1000)).toEqual({ x_pct: 0, y_pct: 100 });
  });
});

describe('descendantsOf', () => {
  const loc = (id: number, parent_id: number | null, name: string) =>
    ({ id, parent_id, name, external_key: name }) as unknown as Location;
  const all = [loc(1, null, 'Site'), loc(2, 1, 'Floor'), loc(3, 2, 'Room'), loc(9, null, 'Other')];
  it('returns every location below, not the root or unrelated ones', () => {
    expect(descendantsOf(1, all).map((l) => l.id)).toEqual([2, 3]);
    expect(descendantsOf(3, all)).toEqual([]);
  });
});

describe('dataUrlToBase64', () => {
  it('strips the data URL prefix', () => {
    expect(dataUrlToBase64('data:image/png;base64,iVBORw0')).toBe('iVBORw0');
    expect(dataUrlToBase64('iVBORw0')).toBe('iVBORw0');
  });
});
