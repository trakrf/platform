/**
 * Shared org-context utilities for verifying and refreshing
 * the JWT's current_org_id claim.
 *
 * Used by any code that needs to ensure the token has valid
 * org context before making org-scoped API calls.
 */
import { jwtDecode } from 'jwt-decode';
import { useAuthStore } from '@/stores/authStore';
import { orgsApi } from '@/lib/api/orgs';

interface JwtClaims {
  current_org_id?: number;
  user_id?: number;
}

/**
 * Extract current_org_id from the JWT in localStorage.
 * Returns null if missing, zero, or token is invalid.
 */
export function getTokenOrgId(): number | null {
  try {
    const authStorage = localStorage.getItem('auth-storage');
    if (!authStorage) return null;

    const { state } = JSON.parse(authStorage);
    if (!state?.token) return null;

    const decoded = jwtDecode<JwtClaims>(state.token);
    return decoded.current_org_id || null; // treats 0 and undefined as null
  } catch {
    return null;
  }
}

/**
 * Call setCurrentOrg for the given org and persist the returned token.
 *
 * Zustand persist middleware wraps setState, so the token persists to localStorage.
 */
export async function setOrgToken(orgId: number): Promise<void> {
  const response = await orgsApi.setCurrentOrg({ org_id: orgId });
  useAuthStore.setState({ token: response.data.token });
}

/**
 * Refresh the JWT with the current org context.
 *
 * Fetches the user profile if not loaded, then calls setCurrentOrg
 * to get a new token with the org_id claim.
 *
 * Returns true if the token was successfully refreshed.
 */
export async function refreshOrgToken(): Promise<boolean> {
  try {
    const profile = useAuthStore.getState().profile;
    if (!profile) {
      await useAuthStore.getState().fetchProfile();
    }
    const currentOrgId = useAuthStore.getState().profile?.current_org?.id;
    if (!currentOrgId) return false;

    await setOrgToken(currentOrgId);
    return true;
  } catch {
    return false;
  }
}

/**
 * Ensure the JWT has a valid current_org_id claim.
 *
 * Checks the token first; if the claim is missing, attempts
 * a token refresh. Throws if org context cannot be established.
 */
export async function ensureOrgContext(): Promise<number> {
  const orgId = getTokenOrgId();
  if (orgId) return orgId;

  console.warn('[OrgContext] JWT missing org_id claim, refreshing token');
  const refreshed = await refreshOrgToken();
  if (!refreshed) {
    throw new Error('No organization context. Please select an organization and try again.');
  }

  const newOrgId = getTokenOrgId();
  if (!newOrgId) {
    throw new Error('No organization context. Please select an organization and try again.');
  }
  return newOrgId;
}
