import type { TabType } from '@/stores';

/**
 * The auth gate — the first of the four gates, and the only one this module
 * answers (TRA-1057).
 *
 * Precedence across the app is auth → entitlement → capability → role. Auth is
 * its own question rather than being folded into the others: a capability is a
 * property of an org, so asking it of a visitor with no org is meaningless, and
 * role is organizational, not commercial. See ADR 0002 §Frontend.
 *
 * - `allow`      — render the route normally.
 * - `pending`    — the answer isn't known yet; render the route's loading
 *                  screen, not a verdict. Same class of problem
 *                  `useCapability`'s `loading` state solves
 *                  (hooks/capability/useCapability.ts): `isAuthenticated`
 *                  starts `false` at store creation and only flips inside
 *                  `initialize()`, which runs from a mount effect — after
 *                  first paint. A signed-in visitor reloading a page would
 *                  otherwise see `signed-out` flash before correcting to
 *                  `allow`, which is worse than the redirect this gate
 *                  replaces. `token` is persisted and rehydrates
 *                  synchronously before `initialize()` runs, so its presence
 *                  is the "don't know yet" signal.
 * - `signed-out` — render `<SignedOutUpsell>` instead: what the surface does,
 *                  plus a trial path and a log-in path. Never a bare redirect;
 *                  a redirect tells the visitor nothing about their options.
 */
export type RouteAuthGate = 'allow' | 'pending' | 'signed-out';

/**
 * Routes whose content is org-scoped and therefore meaningless without an
 * account. Superset of the nine screens the old wrapper component used to
 * gate: the org-admin routes were never wrapped and rendered a screen that 401s.
 */
export const ROUTE_REQUIRES_AUTH: ReadonlySet<TabType> = new Set<TabType>([
  'assets',
  'locations',
  'kits',
  'mustering',
  'reports',
  'reports-history',
  'scan-devices',
  'live-reads',
  'output-devices',
  'org-members',
  'org-settings',
  'org-geofence-defaults',
  'api-keys',
  'webhooks',
  'admin-orgs',
]);

/**
 * Routes reachable without an account, and why each one is here:
 *
 * - `scan` / `locate` — BLE reading is local. Their save paths upsell through
 *   `GatedFab`, which is a different (entitlement) gate.
 * - `settings` / `help` — device and app settings, no org data.
 * - the auth flows — gating a login form on being logged in is a loop.
 * - `create-org` — sits mid-signup, where `isAuthenticated` may not have
 *   flipped before the flow lands here. A card that blinks in front of someone
 *   who just signed up is worse than the status quo. Deliberate (Mike,
 *   2026-07-28); do not tidy it into the set above.
 */
export const PUBLIC_ROUTES: ReadonlySet<TabType> = new Set<TabType>([
  'scan',
  'locate',
  'settings',
  'help',
  'login',
  'signup',
  'forgot-password',
  'reset-password',
  'accept-invite',
  'create-org',
]);

export function routeAuthGate(
  route: TabType,
  isAuthenticated: boolean,
  hasPersistedToken = false
): RouteAuthGate {
  if (isAuthenticated) return 'allow';
  if (!ROUTE_REQUIRES_AUTH.has(route)) return 'allow';
  // A token survived rehydration but initialize() hasn't run (or hasn't
  // finished) yet — genuinely unknown, not a verdict. Default false keeps
  // every existing two-argument call (and test) meaningful: no persisted
  // token in play means the question really is answered, not pending.
  return hasPersistedToken ? 'pending' : 'signed-out';
}
