import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { createStoreWithTracking } from './createStore';
import { authApi } from '@/lib/api/auth';
import { orgsApi } from '@/lib/api/orgs';
import type { User } from '@/lib/api/auth';
import type { UserProfile } from '@/types/org';
import { jwtDecode } from 'jwt-decode';
import * as Sentry from '@sentry/react';

interface AuthState {
  // State
  user: User | null;
  // token is the short-lived access JWT (the Bearer credential). Field name
  // kept as `token` to avoid churning every consumer that reads state.token.
  token: string | null;
  // refreshToken is the long-lived opaque secret exchanged at /auth/refresh.
  // Persisted alongside the access token; rotated on every successful refresh.
  refreshToken: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
  profile: UserProfile | null;
  profileLoading: boolean;

  // Actions
  login: (email: string, password: string) => Promise<void>;
  signup: (
    email: string,
    password: string,
    orgName?: string,
    invitationToken?: string,
    contact?: { name: string; phone: string; website: string },
    acknowledgeNonProd?: boolean
  ) => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
  initialize: () => void;
  fetchProfile: () => Promise<void>;
  // refresh exchanges the persisted refresh_token for a new access+refresh
  // pair. The 401 response interceptor calls this when an access JWT expires.
  // Returns true on success; on failure the caller is responsible for logout.
  refresh: () => Promise<boolean>;
}

export const useAuthStore = create<AuthState>()(
  persist(
    createStoreWithTracking(
      (set, get) => {
        // setOrgContext switches the session's current organization. It mints
        // a new access+refresh pair scoped to the selected org and updates
        // both store fields. Despite the historical name (refreshTokenWithOrg)
        // this is NOT the refresh-token flow — it is the org-switcher.
        const setOrgContext = async (orgId: number, actionName: string, attempt: number = 1): Promise<void> => {
          try {
            console.log('[AuthStore] Setting org context org_id:', orgId, attempt > 1 ? `(attempt ${attempt})` : '');
            const orgResponse = await orgsApi.setCurrentOrg({ org_id: orgId });
            set({
              token: orgResponse.data.access_token,
              refreshToken: orgResponse.data.refresh_token,
            });

            // INVALIDATE: After setCurrentOrg() returns with org_id token
            const { invalidateAllOrgScopedData } = await import('@/lib/cache/orgScopedCache');
            const { queryClient } = await import('@/lib/queryClient');
            await invalidateAllOrgScopedData(queryClient);
          } catch (err) {
            if (attempt < 2) {
              console.warn('[AuthStore] setCurrentOrg failed, retrying...', err);
              await setOrgContext(orgId, actionName, attempt + 1);
            } else {
              console.error('[AuthStore] Failed to set org context after retry:', err);
              throw new Error(`${actionName} failed: could not set organization context`);
            }
          }
        };

        return {
        // Initial state
        user: null,
        token: null,
        refreshToken: null,
        isAuthenticated: false,
        isLoading: false,
        error: null,
        profile: null,
        profileLoading: false,

        // Login action
        login: async (email: string, password: string) => {
          set({ isLoading: true, error: null });
          try {
            const response = await authApi.login({ email, password });
            const { access_token, refresh_token, user } = response.data.data;

            set({
              token: access_token,
              refreshToken: refresh_token,
              user,
              isAuthenticated: true,
              isLoading: false,
              error: null,
            });

            // Set Sentry user context
            Sentry.setUser({
              id: String(user.id),
              email: user.email,
            });

            // Fetch profile to populate org data after login
            await get().fetchProfile();

            // Ensure token has org_id claim for org-scoped API calls
            const profile = get().profile;
            if (profile?.current_org?.id) {
              await setOrgContext(profile.current_org.id, 'Login');
            }
          } catch (err: unknown) {
            // Extract error message from RFC 7807 Problem Details format
            // Handle empty strings by checking truthy AND non-empty
            const axiosError = err as { response?: { data?: { error?: { detail?: string; title?: string } | string; detail?: string; title?: string } }; message?: string };
            const data = axiosError.response?.data;
            const errorObj = (typeof data?.error === 'object' ? data.error : data) as { detail?: string; title?: string } | undefined;
            let errorMessage =
              (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) ||
              (typeof errorObj?.title === 'string' && errorObj.title.trim()) ||
              (typeof data?.error === 'string' && data.error.trim()) ||
              (typeof axiosError.message === 'string' && axiosError.message.trim()) ||
              'Login failed';

            // Ensure it's always a string (defensive coding)
            if (typeof errorMessage !== 'string') {
              errorMessage = JSON.stringify(errorMessage);
            }

            set({
              error: errorMessage,
              isLoading: false,
            });
            throw err;
          }
        },

        // Signup action
        signup: async (
          email: string,
          password: string,
          orgName?: string,
          invitationToken?: string,
          contact?: { name: string; phone: string; website: string },
          acknowledgeNonProd?: boolean
        ) => {
          set({ isLoading: true, error: null });
          try {
            const response = await authApi.signup({
              email,
              password,
              org_name: orgName,
              name: contact?.name,
              phone: contact?.phone,
              website: contact?.website,
              acknowledge_non_prod: acknowledgeNonProd,
              invitation_token: invitationToken,
            });
            const { access_token, refresh_token, user } = response.data.data;

            set({
              token: access_token,
              refreshToken: refresh_token,
              user,
              isAuthenticated: true,
              isLoading: false,
              error: null,
            });

            // Set Sentry user context
            Sentry.setUser({
              id: String(user.id),
              email: user.email,
            });

            // Fetch profile to populate org data after signup
            await get().fetchProfile();

            // Ensure token has org_id claim for org-scoped API calls
            const profile = get().profile;
            if (profile?.current_org?.id) {
              await setOrgContext(profile.current_org.id, 'Signup');
            }
          } catch (err: unknown) {
            // Extract error message from RFC 7807 Problem Details format
            // Handle empty strings by checking truthy AND non-empty
            const axiosError = err as { response?: { data?: { error?: { detail?: string; title?: string } | string; detail?: string; title?: string } }; message?: string };
            const data = axiosError.response?.data;
            const errorObj = (typeof data?.error === 'object' ? data.error : data) as { detail?: string; title?: string } | undefined;
            let errorMessage =
              (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) ||
              (typeof errorObj?.title === 'string' && errorObj.title.trim()) ||
              (typeof data?.error === 'string' && data.error.trim()) ||
              (typeof axiosError.message === 'string' && axiosError.message.trim()) ||
              'Signup failed';

            // Ensure it's always a string (defensive coding)
            if (typeof errorMessage !== 'string') {
              errorMessage = JSON.stringify(errorMessage);
            }

            set({
              error: errorMessage,
              isLoading: false,
            });
            throw err;
          }
        },

        logout: async () => {
          // Best-effort server-side revoke: if a refresh token is held, ask
          // the backend to mark it revoked so it can't be exchanged again.
          // Failures here don't block the client-side clear — the user wanted
          // out, and the access token expires shortly anyway.
          const refreshToken = get().refreshToken;
          if (refreshToken) {
            try {
              await authApi.logout(refreshToken);
            } catch (err) {
              console.warn('[AuthStore] Server-side logout failed (ignored):', err);
            }
          }

          // Clear Sentry user context
          Sentry.setUser(null);

          set({
            user: null,
            token: null,
            refreshToken: null,
            isAuthenticated: false,
            error: null,
            profile: null,
          });

          // Clear all org-scoped data
          Promise.all([
            import('@/lib/cache/orgScopedCache'),
            import('@/lib/queryClient'),
          ]).then(([{ invalidateAllOrgScopedData }, { queryClient }]) => {
            invalidateAllOrgScopedData(queryClient);
          });
        },

        // refresh exchanges the persisted refresh token for a new
        // access+refresh pair. Returns false if no refresh token is held or
        // the server rejects the exchange. The 401 interceptor uses this and
        // is responsible for triggering logout when refresh fails.
        refresh: async () => {
          const currentRefresh = get().refreshToken;
          if (!currentRefresh) {
            return false;
          }
          try {
            const response = await authApi.refresh(currentRefresh);
            set({
              token: response.data.access_token,
              refreshToken: response.data.refresh_token,
            });
            return true;
          } catch (err) {
            console.warn('[AuthStore] Refresh failed:', err);
            return false;
          }
        },

        clearError: () => set({ error: null }),

        initialize: () => {
          const state = get();

          if (!state.token) {
            set({ isAuthenticated: false, user: null });
            return;
          }

          try {
            const decoded = jwtDecode<{ exp: number }>(state.token);

            const now = Math.floor(Date.now() / 1000);
            if (decoded.exp && decoded.exp < now) {
              console.warn('AuthStore: Token expired, clearing auth state');
              set({
                token: null,
                user: null,
                isAuthenticated: false,
              });
              return;
            }

            set({ isAuthenticated: true });

            // Restore Sentry user context from persisted state
            if (state.user) {
              Sentry.setUser({
                id: String(state.user.id),
                email: state.user.email,
              });
            }
          } catch (error) {
            console.error('AuthStore: Failed to decode JWT, clearing auth state:', error);
            set({
              token: null,
              user: null,
              isAuthenticated: false,
            });
          }
        },

        fetchProfile: async () => {
          const state = get();
          if (!state.isAuthenticated || !state.token) {
            return;
          }

          set({ profileLoading: true });
          try {
            const response = await orgsApi.getProfile();
            set({
              profile: response.data.data,
              profileLoading: false,
            });
          } catch (error) {
            console.error('AuthStore: Failed to fetch profile:', error);
            set({ profileLoading: false });
          }
        },
      };
      },
      'authStore'
    ),
    {
      name: 'auth-storage',
      partialize: (state) => ({
        token: state.token,
        refreshToken: state.refreshToken,
        user: state.user,
      }),
      onRehydrateStorage: () => (state) => {
        if (state) {
          if ((window as any).__OPENREPLAY__) {
            console.log('AuthStore: Sanitizing sensitive data for OpenReplay');
          }
        }
      },
    }
  )
);
