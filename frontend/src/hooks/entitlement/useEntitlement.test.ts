import { renderHook } from '@testing-library/react';
import { describe, it, expect, beforeEach } from 'vitest';
import { useEntitlement } from './useEntitlement';
import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';

function setAuth(isAuthenticated: boolean) {
  useAuthStore.setState({ isAuthenticated } as never);
}
function setOrg(isEntitled: boolean | null, expiresAt: string | null = null) {
  useOrgStore.setState({
    currentOrg:
      isEntitled === null
        ? null
        : ({
            id: 1,
            name: 'Acme',
            identifier: 'acme',
            role: 'owner',
            is_entitled: isEntitled,
            subscription_enabled: isEntitled,
            subscription_expires_at: expiresAt,
          } as never),
  } as never);
}

describe('useEntitlement', () => {
  beforeEach(() => {
    setAuth(false);
    setOrg(null);
  });

  it('returns logged-out when unauthenticated', () => {
    setAuth(false);
    setOrg(null);
    const { result } = renderHook(() => useEntitlement());
    expect(result.current.state).toBe('logged-out');
    expect(result.current.isLocked).toBe(true);
    expect(result.current.isEntitled).toBe(false);
  });

  it('returns entitled when authenticated and is_entitled', () => {
    setAuth(true);
    setOrg(true);
    const { result } = renderHook(() => useEntitlement());
    expect(result.current.state).toBe('entitled');
    expect(result.current.isLocked).toBe(false);
    expect(result.current.isEntitled).toBe(true);
  });

  it('returns lapsed when authenticated but not entitled', () => {
    setAuth(true);
    setOrg(false, '2026-01-01T00:00:00Z');
    const { result } = renderHook(() => useEntitlement());
    expect(result.current.state).toBe('lapsed');
    expect(result.current.isLocked).toBe(true);
    expect(result.current.subscriptionExpiresAt).toBe('2026-01-01T00:00:00Z');
  });

  it('fails open (unlocked) when authenticated but the org has not loaded yet', () => {
    // currentOrg === null means the profile fetch is still in flight; do not flash
    // grayed controls at entitled users. The backend enforces entitlement anyway.
    setAuth(true);
    setOrg(null);
    const { result } = renderHook(() => useEntitlement());
    expect(result.current.state).toBe('entitled');
    expect(result.current.isLocked).toBe(false);
  });
});
