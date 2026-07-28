import { describe, it, expect } from 'vitest';
import {
  ROUTE_REQUIRES_AUTH,
  PUBLIC_ROUTES,
  routeAuthGate,
} from './routePolicy';
import type { TabType } from '@/stores';

/**
 * Every tab the app can route to, keyed off `TabType` itself rather than
 * hand-copied — so adding a member to the `TabType` union without updating
 * this map is a typecheck failure (a missing key), not a silently-passing
 * test that forgot to gate the new route. `ALL_TABS` below is derived from
 * this map's keys, so the assertions that use it stay exhaustive for free.
 */
const ALL_TABS_MAP: Record<TabType, true> = {
  scan: true,
  settings: true,
  locate: true,
  kits: true,
  help: true,
  assets: true,
  locations: true,
  'scan-devices': true,
  'output-devices': true,
  'live-reads': true,
  reports: true,
  'reports-history': true,
  mustering: true,
  login: true,
  signup: true,
  'forgot-password': true,
  'reset-password': true,
  'create-org': true,
  'org-members': true,
  'org-settings': true,
  'org-geofence-defaults': true,
  'accept-invite': true,
  'api-keys': true,
  webhooks: true,
  'admin-orgs': true,
};

const ALL_TABS: TabType[] = Object.keys(ALL_TABS_MAP) as TabType[];

describe('routePolicy', () => {
  it('classifies every tab exactly once', () => {
    for (const tab of ALL_TABS) {
      const inAuth = ROUTE_REQUIRES_AUTH.has(tab);
      const inPublic = PUBLIC_ROUTES.has(tab);
      expect(inAuth || inPublic, `${tab} is unclassified`).toBe(true);
      expect(inAuth && inPublic, `${tab} is in both sets`).toBe(false);
    }
  });

  it('keeps the locally-useful surfaces public', () => {
    for (const tab of ['scan', 'locate', 'settings', 'help'] as TabType[]) {
      expect(routeAuthGate(tab, false)).toBe('allow');
    }
  });

  it('keeps every auth flow public, including create-org', () => {
    // create-org sits mid-signup, where isAuthenticated may not have flipped
    // yet; gating it would flash a card at someone who just signed up.
    for (const tab of [
      'login', 'signup', 'forgot-password', 'reset-password', 'accept-invite', 'create-org',
    ] as TabType[]) {
      expect(routeAuthGate(tab, false)).toBe('allow');
    }
  });

  it('gates the org-scoped surfaces when signed out', () => {
    for (const tab of [
      'assets', 'locations', 'kits', 'mustering', 'reports', 'reports-history',
      'scan-devices', 'live-reads', 'output-devices', 'org-members',
      'org-settings', 'org-geofence-defaults', 'api-keys', 'webhooks', 'admin-orgs',
    ] as TabType[]) {
      expect(routeAuthGate(tab, false)).toBe('signed-out');
    }
  });

  it('allows everything once authenticated', () => {
    for (const tab of ALL_TABS) {
      expect(routeAuthGate(tab, true)).toBe('allow');
    }
  });

  it('holds a persisted-token reload as pending rather than flashing signed-out', () => {
    // Token rehydrated synchronously, initialize() hasn't resolved it yet.
    expect(routeAuthGate('reports', false, true)).toBe('pending');
  });

  it('answers signed-out on an auth-required route with no persisted token', () => {
    expect(routeAuthGate('reports', false, false)).toBe('signed-out');
  });

  it('leaves public routes allowed regardless of the persisted-token flag', () => {
    expect(routeAuthGate('scan', false, true)).toBe('allow');
    expect(routeAuthGate('scan', false, false)).toBe('allow');
  });

  it('leaves an authenticated user allowed regardless of the persisted-token flag', () => {
    expect(routeAuthGate('reports', true, true)).toBe('allow');
    expect(routeAuthGate('reports', true, false)).toBe('allow');
  });
});
