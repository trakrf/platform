import { renderHook } from '@testing-library/react';
import { describe, it, expect, beforeEach } from 'vitest';
import {
  useCapability,
  useCapabilityState,
  useCapabilityNavGate,
  useCapabilityRouteGate,
  navGateFor,
  routeGateFor,
} from './useCapability';
import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';

function setAuth(isAuthenticated: boolean) {
  useAuthStore.setState({ isAuthenticated } as never);
}

/** `capabilities === null` models "profile not loaded yet" (currentOrg null). */
function setOrg(capabilities: string[] | null) {
  useOrgStore.setState({
    currentOrg:
      capabilities === null
        ? null
        : ({
            id: 1,
            name: 'Acme',
            identifier: 'acme',
            role: 'owner',
            is_entitled: true,
            subscription_enabled: true,
            subscription_expires_at: null,
            capabilities,
          } as never),
  } as never);
}

describe('useCapabilityState', () => {
  beforeEach(() => {
    setAuth(false);
    setOrg(null);
  });

  // Not `ungated` — that would assert an org lacks the capability when there
  // is no org at all.
  it('is no-org when unauthenticated', () => {
    const { result } = renderHook(() => useCapabilityState('geofence'));
    expect(result.current).toBe('no-org');
  });

  it('is loading while authenticated with no org yet', () => {
    setAuth(true);
    setOrg(null);
    const { result } = renderHook(() => useCapabilityState('geofence'));
    expect(result.current).toBe('loading');
  });

  it('is granted when the org holds the capability', () => {
    setAuth(true);
    setOrg(['geofence', 'mustering']);
    const { result } = renderHook(() => useCapabilityState('geofence'));
    expect(result.current).toBe('granted');
  });

  it('is ungated when the org holds no grants', () => {
    setAuth(true);
    setOrg([]);
    const { result } = renderHook(() => useCapabilityState('geofence'));
    expect(result.current).toBe('ungated');
  });

  it('is ungated when the capabilities field is missing entirely', () => {
    setAuth(true);
    setOrg([]);
    useOrgStore.setState({
      currentOrg: { id: 1, name: 'Acme', identifier: 'acme', role: 'owner' } as never,
    } as never);
    const { result } = renderHook(() => useCapabilityState('geofence'));
    expect(result.current).toBe('ungated');
  });
});

describe('useCapability', () => {
  beforeEach(() => {
    setAuth(false);
    setOrg(null);
  });

  it('is true only for a granted capability', () => {
    setAuth(true);
    setOrg(['mustering']);
    expect(renderHook(() => useCapability('mustering')).result.current).toBe(true);
    expect(renderHook(() => useCapability('geofence')).result.current).toBe(false);
  });

  // The deliberate inversion vs useEntitlement, which fails OPEN here.
  it('fails CLOSED while the profile is loading', () => {
    setAuth(true);
    setOrg(null);
    expect(renderHook(() => useCapability('geofence')).result.current).toBe(false);
  });

  it('reflects a capability set that changes on org switch', () => {
    setAuth(true);
    setOrg(['geofence']);
    const { result, rerender } = renderHook(() => useCapability('geofence'));
    expect(result.current).toBe(true);

    setOrg([]);
    rerender();
    expect(result.current).toBe(false);
  });
});

/**
 * Presentation policy as pure data. No registry entry currently uses `absent`
 * (mustering moved to `locked` on 2026-07-27), so these tests are what keep
 * that branch honest — the hook tests below can only cover what the registry
 * happens to declare today.
 */
describe('navGateFor / routeGateFor', () => {
  it('hides an ungated `absent` entry and resolves its route to not-found', () => {
    expect(navGateFor('absent', 'ungated')).toBe('hidden');
    expect(routeGateFor('absent', 'ungated')).toBe('not-found');
  });

  it('locks an ungated `locked` entry and resolves its route to the upsell', () => {
    expect(navGateFor('locked', 'ungated')).toBe('locked');
    expect(routeGateFor('locked', 'ungated')).toBe('upsell');
  });

  it('shows and allows either presentation once granted', () => {
    for (const p of ['absent', 'locked'] as const) {
      expect(navGateFor(p, 'granted')).toBe('visible');
      expect(routeGateFor(p, 'granted')).toBe('allow');
    }
  });

  it('fails closed in nav and waits in routing while loading, either presentation', () => {
    for (const p of ['absent', 'locked'] as const) {
      expect(navGateFor(p, 'loading')).toBe('hidden');
      expect(routeGateFor(p, 'loading')).toBe('loading');
    }
  });

  // Signed out: hide in nav (no teaser aimed at an org that doesn't exist), but
  // don't gate the route — the screen owns signed-out handling.
  it('hides in nav and falls through in routing with no org, either presentation', () => {
    for (const p of ['absent', 'locked'] as const) {
      expect(navGateFor(p, 'no-org')).toBe('hidden');
      expect(routeGateFor(p, 'no-org')).toBe('allow');
    }
  });
});

describe('useCapabilityNavGate', () => {
  beforeEach(() => {
    setAuth(true);
    setOrg([]);
  });

  it('leaves an ungated route visible', () => {
    expect(renderHook(() => useCapabilityNavGate('assets')).result.current).toBe('visible');
  });

  it('locks every ungated gated route (all registry entries are `locked`)', () => {
    expect(renderHook(() => useCapabilityNavGate('mustering')).result.current).toBe('locked');
    expect(renderHook(() => useCapabilityNavGate('output-devices')).result.current).toBe('locked');
    expect(renderHook(() => useCapabilityNavGate('org-geofence-defaults')).result.current).toBe(
      'locked'
    );
  });

  it('shows gated routes normally once granted', () => {
    setOrg(['geofence', 'mustering']);
    expect(renderHook(() => useCapabilityNavGate('mustering')).result.current).toBe('visible');
    expect(renderHook(() => useCapabilityNavGate('output-devices')).result.current).toBe('visible');
  });

  it('hides every gated entry while the capability set is loading', () => {
    setOrg(null);
    expect(renderHook(() => useCapabilityNavGate('mustering')).result.current).toBe('hidden');
    expect(renderHook(() => useCapabilityNavGate('output-devices')).result.current).toBe('hidden');
  });
});

describe('useCapabilityRouteGate', () => {
  beforeEach(() => {
    setAuth(true);
    setOrg([]);
  });

  it('allows ungated routes', () => {
    expect(renderHook(() => useCapabilityRouteGate('assets')).result.current).toBe('allow');
  });

  it('resolves every ungated gated route to the upsell', () => {
    expect(renderHook(() => useCapabilityRouteGate('mustering')).result.current).toBe('upsell');
    expect(renderHook(() => useCapabilityRouteGate('output-devices')).result.current).toBe('upsell');
    expect(renderHook(() => useCapabilityRouteGate('org-geofence-defaults')).result.current).toBe(
      'upsell'
    );
  });

  it('allows a granted route', () => {
    setOrg(['geofence', 'mustering']);
    expect(renderHook(() => useCapabilityRouteGate('mustering')).result.current).toBe('allow');
    expect(renderHook(() => useCapabilityRouteGate('output-devices')).result.current).toBe('allow');
  });

  // A bookmarked gated URL must not be bounced before the profile lands.
  it('waits rather than bouncing while the capability set is loading', () => {
    setOrg(null);
    expect(renderHook(() => useCapabilityRouteGate('mustering')).result.current).toBe('loading');
    expect(renderHook(() => useCapabilityRouteGate('output-devices')).result.current).toBe(
      'loading'
    );
  });
});
