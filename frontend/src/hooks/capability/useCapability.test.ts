import { renderHook } from '@testing-library/react';
import { describe, it, expect, beforeEach } from 'vitest';
import {
  useCapability,
  useCapabilityState,
  useCapabilityNavGate,
  useCapabilityRouteGate,
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

  it('is ungated when unauthenticated', () => {
    const { result } = renderHook(() => useCapabilityState('geofence'));
    expect(result.current).toBe('ungated');
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

describe('useCapabilityNavGate', () => {
  beforeEach(() => {
    setAuth(true);
    setOrg([]);
  });

  it('leaves an ungated route visible', () => {
    expect(renderHook(() => useCapabilityNavGate('assets')).result.current).toBe('visible');
  });

  it('hides an ungated `absent` route (mustering)', () => {
    expect(renderHook(() => useCapabilityNavGate('mustering')).result.current).toBe('hidden');
  });

  it('locks an ungated `locked` route (geofence surfaces)', () => {
    expect(renderHook(() => useCapabilityNavGate('output-devices')).result.current).toBe('locked');
    expect(renderHook(() => useCapabilityNavGate('org-geofence-defaults')).result.current).toBe(
      'locked'
    );
  });

  it('shows both presentations normally once granted', () => {
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

  it('resolves an ungated `absent` route to not-found', () => {
    expect(renderHook(() => useCapabilityRouteGate('mustering')).result.current).toBe('not-found');
  });

  it('resolves an ungated `locked` route to the upsell', () => {
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
