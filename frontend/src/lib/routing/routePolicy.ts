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
 * - `signed-out` — render `<SignedOutUpsell>` instead: what the surface does,
 *                  plus a trial path and a log-in path. Never a bare redirect;
 *                  a redirect tells the visitor nothing about their options.
 */
export type RouteAuthGate = 'allow' | 'signed-out';

/**
 * Routes whose content is org-scoped and therefore meaningless without an
 * account. Superset of the nine screens `ProtectedRoute` used to wrap: the
 * org-admin routes were never wrapped and rendered a screen that 401s.
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

export function routeAuthGate(route: TabType, isAuthenticated: boolean): RouteAuthGate {
  if (isAuthenticated) return 'allow';
  return ROUTE_REQUIRES_AUTH.has(route) ? 'signed-out' : 'allow';
}
