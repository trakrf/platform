import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';
import { capabilityEntryForRoute } from '@/components/capability/registry';
import type { CapabilityPresentation } from '@/components/capability/registry';
import type { TabType } from '@/stores';

/**
 * Resolution of a capability grant.
 *
 * Two of these mean "not answerable", for different reasons, and they resolve
 * differently in routing:
 *
 * - `no-org` — nobody is signed in. A capability is a property of an *org*, so
 *   with no org the question is not merely unanswered, it is meaningless.
 *   Answering `ungated` here would assert that an org lacks something when
 *   there is no org, and would show a signed-out visitor copy addressed to
 *   "your organization".
 * - `loading` — signed in, profile not back yet. Genuinely unknown, and worth
 *   waiting for: a bookmarked URL to a granted surface must not be bounced to
 *   not-found just because the profile is in flight.
 *
 * Nav treats both as "do not render". Routing waits on `loading` but falls
 * through on `no-org`, leaving signed-out handling to the screen — which is how
 * every other org-scoped tab already behaves.
 */
export type CapabilityState = 'no-org' | 'loading' | 'granted' | 'ungated';

function capabilitiesOf(caps: string[] | undefined): string[] {
  return caps ?? [];
}

export function useCapabilityState(capability: string): CapabilityState {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const currentOrg = useOrgStore((s) => s.currentOrg);

  if (!isAuthenticated) return 'no-org';
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
 * Nav decision for a gated entry, as pure data.
 *
 * Split out from the hook so both presentations stay covered by tests
 * regardless of which ones the registry happens to use today. `absent` has no
 * registry entry at the moment (mustering moved to `locked` on 2026-07-27) and
 * this is what keeps it real, tested code rather than an untested branch.
 */
export function navGateFor(
  presentation: CapabilityPresentation,
  state: CapabilityState
): CapabilityNavGate {
  if (state === 'granted') return 'visible';
  // Fail-closed whenever the answer isn't available, in both presentations:
  // nothing pops in and then out, and a signed-out visitor is never shown a
  // teaser whose only honest CTA would be "sign up", not "contact us".
  if (state === 'loading' || state === 'no-org') return 'hidden';
  return presentation === 'locked' ? 'locked' : 'hidden';
}

/** Nav presentation for `route`. Ungated routes are always `visible`. */
export function useCapabilityNavGate(route: TabType): CapabilityNavGate {
  const entry = capabilityEntryForRoute(route);
  const state = useCapabilityState(entry?.capability ?? '');

  if (!entry) return 'visible';
  return navGateFor(entry.presentation, state);
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

/** Route decision as pure data. Same rationale as `navGateFor`. */
export function routeGateFor(
  presentation: CapabilityPresentation,
  state: CapabilityState
): CapabilityRouteGate {
  if (state === 'granted') return 'allow';
  if (state === 'loading') return 'loading';
  // Signed out: the capability gate has no opinion, so this answers `allow`
  // and defers. Retained as a defensive default, but routing no longer
  // reaches it in practice — all three capability-gated routes are in
  // `ROUTE_REQUIRES_AUTH`, so the auth gate (routePolicy, TRA-1057) answers
  // first and renders the signed-out card before this hook is ever asked.
  // Gating here too would either strand the visitor on a spinner or upsell
  // them on behalf of an org they don't have, if that ever changed.
  if (state === 'no-org') return 'allow';
  return presentation === 'locked' ? 'upsell' : 'not-found';
}

export function useCapabilityRouteGate(route: TabType): CapabilityRouteGate {
  const entry = capabilityEntryForRoute(route);
  const state = useCapabilityState(entry?.capability ?? '');

  if (!entry) return 'allow';
  return routeGateFor(entry.presentation, state);
}
