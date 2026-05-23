import { describe, it, expect, vi, beforeEach } from 'vitest';

/**
 * Unit tests for org context utilities.
 * Validates JWT org_id extraction, token refresh, and ensureOrgContext guard.
 */

// Mock jwt-decode
vi.mock('jwt-decode', () => ({
  jwtDecode: vi.fn(),
}));

// Mock the orgs API (used by setOrgToken inside refreshOrgToken)
vi.mock('@/lib/api/orgs', () => ({
  orgsApi: {
    setCurrentOrg: vi.fn(),
  },
}));

import { jwtDecode } from 'jwt-decode';
import { getTokenOrgId, ensureOrgContext } from './orgContext';
import { useAuthStore } from '@/stores/authStore';
import { orgsApi } from '@/lib/api/orgs';

describe('getTokenOrgId', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    localStorage.clear();
  });

  it('returns org_id from valid token with org claim', () => {
    localStorage.setItem('auth-storage', JSON.stringify({
      state: { token: 'valid.jwt.token' },
    }));
    vi.mocked(jwtDecode).mockReturnValue({ current_org_id: 42 });

    expect(getTokenOrgId()).toBe(42);
  });

  it('returns null when no auth storage', () => {
    expect(getTokenOrgId()).toBeNull();
  });

  it('returns null when token has no org_id claim', () => {
    localStorage.setItem('auth-storage', JSON.stringify({
      state: { token: 'valid.jwt.token' },
    }));
    vi.mocked(jwtDecode).mockReturnValue({ user_id: 1 });

    expect(getTokenOrgId()).toBeNull();
  });

  it('returns null when org_id is 0', () => {
    localStorage.setItem('auth-storage', JSON.stringify({
      state: { token: 'valid.jwt.token' },
    }));
    vi.mocked(jwtDecode).mockReturnValue({ current_org_id: 0 });

    expect(getTokenOrgId()).toBeNull();
  });

  it('returns null when jwt-decode throws', () => {
    localStorage.setItem('auth-storage', JSON.stringify({
      state: { token: 'corrupt.token' },
    }));
    vi.mocked(jwtDecode).mockImplementation(() => { throw new Error('Invalid token'); });

    expect(getTokenOrgId()).toBeNull();
  });
});

describe('ensureOrgContext drift detection (TRA-812)', () => {
  // Only current_org.id is exercised here; cast as any to keep the test setup
  // independent of the UserProfile shape (which carries a bunch of fields we
  // don't care about for this guard).
  function setProfileWithOrg(orgId: number | null) {
    useAuthStore.setState({
      profile: orgId === null ? null : ({ current_org: { id: orgId } } as any),
    });
  }

  // Map of token-string → decoded payload, so a single shared jwtDecode mock
  // can return the right claim per token regardless of call order.
  const tokenClaims = new Map<string, Record<string, unknown>>();

  function setStoredToken(token: string, claims: Record<string, unknown>) {
    // Route through the zustand store so the persist middleware doesn't
    // clobber the localStorage write on the next setState in this test.
    tokenClaims.set(token, claims);
    useAuthStore.setState({ token });
  }

  function tokenWithOrg(orgId: number) {
    setStoredToken('jwt.token', { current_org_id: orgId });
  }

  beforeEach(() => {
    vi.resetAllMocks();
    localStorage.clear();
    tokenClaims.clear();
    useAuthStore.setState({ profile: null, token: null });

    vi.mocked(jwtDecode).mockImplementation((token: string) => {
      return (tokenClaims.get(token) ?? {}) as any;
    });

    // setOrgToken (called by refreshOrgToken) drives the realignment by
    // calling orgsApi.setCurrentOrg and then useAuthStore.setState with the
    // returned token; the persist middleware writes that through to
    // localStorage. We only need to register the new token's claims here so
    // the subsequent jwtDecode call returns the matching payload.
    vi.mocked(orgsApi.setCurrentOrg).mockImplementation(async ({ org_id }: { org_id: number }) => {
      const newToken = `aligned.${org_id}.jwt`;
      tokenClaims.set(newToken, { current_org_id: org_id });
      return { data: { token: newToken } } as any;
    });
  });

  it('returns token org_id when it matches the profile current_org', async () => {
    tokenWithOrg(42);
    setProfileWithOrg(42);

    const orgId = await ensureOrgContext();
    expect(orgId).toBe(42);
    expect(orgsApi.setCurrentOrg).not.toHaveBeenCalled();
  });

  it('returns token org_id when profile is not yet loaded', async () => {
    tokenWithOrg(42);
    setProfileWithOrg(null);

    const orgId = await ensureOrgContext();
    expect(orgId).toBe(42);
    expect(orgsApi.setCurrentOrg).not.toHaveBeenCalled();
  });

  it('refreshes the token when JWT and profile disagree on org', async () => {
    // Token says org 7, profile says org 42 — the persisted JWT has drifted
    // from the user's current_org. Realign by minting a fresh token against
    // the profile's org.
    tokenWithOrg(7);
    setProfileWithOrg(42);

    const orgId = await ensureOrgContext();

    expect(orgsApi.setCurrentOrg).toHaveBeenCalledWith({ org_id: 42 });
    expect(orgId).toBe(42);
  });

  it('refreshes the token when JWT has no org_id but profile has one', async () => {
    setStoredToken('jwt.no-org.token', { user_id: 1 }); // no current_org_id
    setProfileWithOrg(42);

    const orgId = await ensureOrgContext();

    expect(orgsApi.setCurrentOrg).toHaveBeenCalledWith({ org_id: 42 });
    expect(orgId).toBe(42);
  });

  it('throws when realignment cannot establish an org context', async () => {
    tokenWithOrg(7);
    setProfileWithOrg(42);

    // setCurrentOrg fails (e.g., backend returns an error)
    vi.mocked(orgsApi.setCurrentOrg).mockRejectedValue(new Error('boom'));

    await expect(ensureOrgContext()).rejects.toThrow('No organization context');
  });
});
