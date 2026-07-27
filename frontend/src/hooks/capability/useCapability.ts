import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';
import { capabilityEntryForRoute } from '@/components/capability/registry';
import type { TabType } from '@/stores';

/**
 * Tri-state resolution of a capability grant.
 *
 * `loading` exists only for route resolution: a bookmarked URL for a granted
 * surface must not be bounced to not-found just because the profile has not
 * arrived yet. Nav rendering collapses `loading` into "do not render".
 */
export type CapabilityState = 'loading' | 'granted' | 'ungated';

function capabilitiesOf(caps: string[] | undefined): string[] {
  return caps ?? [];
}

export function useCapabilityState(capability: string): CapabilityState {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const currentOrg = useOrgStore((s) => s.currentOrg);

  if (!isAuthenticated) return 'ungated';
  // Authenticated but the profile hasn't landed: not yet knowable.
  if (!currentOrg) return 'loading';
  return capabilitiesOf(currentOrg.capabilities).includes(capability) ? 'granted' : 'ungated';
}

/**
 * Whether the current org holds `capability` (ADR 0002 §6 / TRA-1026).
 *
 * Mirrors `useEntitlement()` with one deliberate inversion: `useEntitlement`
 * fails OPEN when the org is null, so entitled users don't see a flash of
 * grayed controls. `useCapability` fails CLOSED — while the profile is loading
 * or the org is null this returns false, so a gated surface never flashes on
 * and then disappears. Do not "fix" this to match useEntitlement.
 *
 * This is UX only. The security boundary is RequireCap on the backend.
 */
export function useCapability(capability: string): boolean {
  return useCapabilityState(capability) === 'granted';
}

/** How a capability-gated nav entry should render. */
export type CapabilityNavGate = 'visible' | 'locked' | 'hidden';

/**
 * Nav presentation for `route`. Ungated routes are always `visible`.
 *
 * While the capability set is loading, a gated entry is `hidden` in both
 * presentations — fail-closed, so nothing pops in and then out.
 */
export function useCapabilityNavGate(route: TabType): CapabilityNavGate {
  const entry = capabilityEntryForRoute(route);
  const state = useCapabilityState(entry?.capability ?? '');

  if (!entry) return 'visible';
  if (state === 'granted') return 'visible';
  if (state === 'loading') return 'hidden';
  return entry.presentation === 'locked' ? 'locked' : 'hidden';
}

/**
 * How a route should resolve.
 *
 * - `allow` — render the real surface (and only then fetch its chunk).
 * - `loading` — capability set not yet known; render the route's loading
 *   screen. The real chunk is not requested.
 * - `not-found` — ungated `absent` route; resolve it the way the app resolves
 *   any unknown route (fall back to the default tab).
 * - `upsell` — ungated `locked` route; render the upsell view.
 */
export type CapabilityRouteGate = 'allow' | 'loading' | 'not-found' | 'upsell';

export function useCapabilityRouteGate(route: TabType): CapabilityRouteGate {
  const entry = capabilityEntryForRoute(route);
  const state = useCapabilityState(entry?.capability ?? '');

  if (!entry) return 'allow';
  if (state === 'granted') return 'allow';
  if (state === 'loading') return 'loading';
  return entry.presentation === 'locked' ? 'upsell' : 'not-found';
}
