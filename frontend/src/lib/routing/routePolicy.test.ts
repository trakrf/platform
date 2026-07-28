import { describe, it, expect } from 'vitest';
import {
  ROUTE_REQUIRES_AUTH,
  PUBLIC_ROUTES,
  routeAuthGate,
} from './routePolicy';
import type { TabType } from '@/stores';

/**
 * Every tab the app can route to. Kept literal rather than derived, so adding a
 * TabType without deciding its auth answer fails here instead of defaulting.
 */
const ALL_TABS: TabType[] = [
  'scan', 'settings', 'locate', 'kits', 'help', 'assets', 'locations',
  'scan-devices', 'output-devices', 'live-reads', 'reports', 'reports-history',
  'mustering', 'login', 'signup', 'forgot-password', 'reset-password',
  'create-org', 'org-members', 'org-settings', 'org-geofence-defaults',
  'accept-invite', 'api-keys', 'webhooks', 'admin-orgs',
];

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
