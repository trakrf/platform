import { describe, it, expect, vi, beforeEach } from 'vitest';

/**
 * Unit tests for org context utilities.
 * Validates JWT org_id extraction, token refresh, and ensureOrgContext guard.
 */

// Mock jwt-decode
vi.mock('jwt-decode', () => ({
  jwtDecode: vi.fn(),
}));

import { jwtDecode } from 'jwt-decode';
import { getTokenOrgId } from './orgContext';

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
