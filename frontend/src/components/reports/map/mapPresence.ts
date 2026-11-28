/**
 * Pure helpers for the location map (TRA-1402). No React, unit-tested.
 */

import type { MapOccupant, MapPresence } from '@/types/locationMaps';
import type { Location } from '@/types/locations';

export type OccupantKind = 'all' | 'people' | 'assets';

export interface PresenceFilter {
  kind: OccupantKind;
  /** Case-insensitive substring on name or external key. Empty = no filter. */
  q: string;
}

function matches(o: MapOccupant, f: PresenceFilter, q: string): boolean {
  if (f.kind === 'people' && !o.is_person) return false;
  if (f.kind === 'assets' && o.is_person) return false;
  if (!q) return true;
  return o.name.toLowerCase().includes(q) || o.external_key.toLowerCase().includes(q);
}

/** Filter occupants everywhere on the map. Zones stay, even when emptied, so
 *  the pins don't jump around as the filter changes. */
export function filterPresence(p: MapPresence, f: PresenceFilter): MapPresence {
  const q = f.q.trim().toLowerCase();
  if (f.kind === 'all' && !q) return p;
  return {
    ...p,
    zones: p.zones.map((z) => ({ ...z, occupants: z.occupants.filter((o) => matches(o, f, q)) })),
    unplaced: p.unplaced.filter((o) => matches(o, f, q)),
  };
}

interface RectLike {
  left: number;
  top: number;
  width: number;
  height: number;
}

const clampPct = (v: number) => Math.min(100, Math.max(0, Math.round(v * 100) / 100));

/** Convert a click on the rendered image to a stored pin position. */
export function pinPercentFromClick(
  rect: RectLike,
  clientX: number,
  clientY: number,
): { x_pct: number; y_pct: number } {
  return {
    x_pct: clampPct(((clientX - rect.left) / rect.width) * 100),
    y_pct: clampPct(((clientY - rect.top) / rect.height) * 100),
  };
}

/** Every location below rootId, in list order. */
export function descendantsOf(rootId: number, all: Location[]): Location[] {
  const children = new Map<number, Location[]>();
  for (const l of all) {
    if (l.parent_id == null) continue;
    const list = children.get(l.parent_id) ?? [];
    list.push(l);
    children.set(l.parent_id, list);
  }
  const out = new Set<number>();
  const stack = [rootId];
  while (stack.length) {
    const id = stack.pop()!;
    for (const c of children.get(id) ?? []) {
      if (!out.has(c.id) && c.id !== rootId) {
        out.add(c.id);
        stack.push(c.id);
      }
    }
  }
  return all.filter((l) => out.has(l.id));
}

/** "just now", "3 min ago", "2 h ago", "4 d ago". */
export function relativeTime(iso: string, now: number = Date.now()): string {
  const secs = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000));
  if (secs < 60) return 'just now';
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins} min ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours} h ago`;
  return `${Math.floor(hours / 24)} d ago`;
}
