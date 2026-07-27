import type { LucideIcon } from 'lucide-react';
import { Users, Siren, SlidersHorizontal } from 'lucide-react';
import type { TabType } from '@/stores';

/**
 * Capability names, mirroring the backend's code-owned vocabulary
 * (backend/internal/capability). Names describe workflows, never customers.
 */
export const CAPABILITY_GEOFENCE = 'geofence';
export const CAPABILITY_MUSTERING = 'mustering';

/**
 * How a capability's surface presents when the org does NOT hold the grant
 * (ADR 0002 §Frontend; decisions final 2026-07-16, Mike + Tim):
 *
 * - `absent` — no nav entry, and the route resolves the way any unknown route
 *   does. No trace that the surface exists.
 * - `locked` — nav entry renders with a lock indicator; the route resolves to
 *   the upsell view. A visible teaser.
 *
 * Nothing here is a security boundary. The enforcement is RequireCap on the
 * backend (TRA-1025); this registry is UX only.
 */
export type CapabilityPresentation = 'absent' | 'locked';

export interface CapabilityNavEntry {
  /** Capability name that must be granted for the surface to be reachable. */
  capability: string;
  label: string;
  /** The tab id this entry navigates to — the route being gated. */
  route: TabType;
  icon: LucideIcon;
  presentation: CapabilityPresentation;
  /** Nav tooltip. Shown in both granted and locked states. */
  tooltip: string;
}

/**
 * Every capability-gated nav entry / route in the app.
 *
 * A capability may own more than one route: `geofence` covers both the alarm
 * output configuration and the org-wide geofence tuning, which is exactly what
 * the backend gates with `requireCap(capability.Geofence)`.
 *
 * `inventory` is deliberately absent. Its presentation decision is `locked`,
 * but it has no surface behind it yet — a teaser for a capability with nothing
 * to unlock would be a claim we cannot honor. The entry lands with the surface.
 */
export const CAPABILITY_NAV: readonly CapabilityNavEntry[] = [
  {
    capability: CAPABILITY_MUSTERING,
    label: 'Mustering',
    route: 'mustering',
    icon: Users,
    // Was `absent` per the 2026-07-16 decision; changed to `locked` at Tim's
    // request 2026-07-27, so mustering now presents exactly like geofence.
    presentation: 'locked',
    tooltip:
      'Run a muster drill — track who is accounted for at muster points during an evacuation',
  },
  {
    capability: CAPABILITY_GEOFENCE,
    label: 'Outputs',
    route: 'output-devices',
    icon: Siren,
    presentation: 'locked',
    tooltip: 'Manage output devices (e.g. Shelly relays) and test-fire them',
  },
  {
    capability: CAPABILITY_GEOFENCE,
    label: 'Geofence defaults',
    route: 'org-geofence-defaults',
    icon: SlidersHorizontal,
    presentation: 'locked',
    tooltip:
      'Org-wide geofence tuning (RSSI, age-out, auto-off, mode) — applied to every portal unless an output overrides it',
  },
];

/** The registry entry gating `route`, or undefined when the route is ungated. */
export function capabilityEntryForRoute(route: TabType): CapabilityNavEntry | undefined {
  return CAPABILITY_NAV.find((entry) => entry.route === route);
}

/**
 * Upsell copy per capability. Fixed text — do not embellish, and do not add
 * capability claims. Revisions are Tim's, via normal PR.
 *
 * Only capabilities whose presentation is `locked` need an entry; an `absent`
 * capability never reaches the upsell view.
 */
export const CAPABILITY_UPSELL_COPY: Record<string, { title: string; blurb: string }> = {
  [CAPABILITY_GEOFENCE]: {
    title: 'Geofence',
    blurb: 'Zone-based tracking with enter/exit alerts.',
  },
  [CAPABILITY_MUSTERING]: {
    title: 'Mustering',
    // Lifted verbatim from the nav tooltip already shipped for this surface,
    // rather than written fresh — no new capability claim. Swap freely.
    blurb: 'Track who is accounted for at muster points during an evacuation.',
  },
};
