import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { createStoreWithTracking } from './createStore';
import { authApi } from '@/lib/api/auth';
import { orgsApi } from '@/lib/api/orgs';
import type { User } from '@/lib/api/auth';
import type { UserProfile } from '@/types/org';
import { jwtDecode } from 'jwt-decode';

interface AuthState {
  // State
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
  profile: UserProfile | null;
  profileLoading: boolean;

  // Actions
  login: (email: string, password: string) => Promise<void>;
  signup: (email: string, password: string) => Promise<void>;
  logout: () => void;
  clearError: () => void;
  initialize: () => void;
  fetchProfile: () => Promise<void>;
}

export const useAuthStore = create<AuthState>()(
  persist(
    createStoreWithTracking(
      (set, get) => ({
        // Initial state
        user: null,
        token: null,
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
            const { token, user } = response.data.data; // Backend wraps response in {data: {token, user}}

            set({
              token,
              user,
              isAuthenticated: true,
              isLoading: false,
              error: null,
            });
          } catch (err: any) {
            // Extract error message from RFC 7807 Problem Details format
            // Handle empty strings by checking truthy AND non-empty
            const data = err.response?.data;
            const errorObj = data?.error || data; // Handle both nested and flat structures
            let errorMessage =
              (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) ||
              (typeof errorObj?.title === 'string' && errorObj.title.trim()) ||
              (typeof data?.error === 'string' && data.error.trim()) ||
              (typeof err.message === 'string' && err.message.trim()) ||
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
        signup: async (email: string, password: string) => {
          set({ isLoading: true, error: null });
          try {
            const response = await authApi.signup({
              email,
              password,
            });
            const { token, user } = response.data.data; // Backend wraps response in {data: {token, user}}

            set({
              token,
              user,
              isAuthenticated: true,
              isLoading: false,
              error: null,
            });
          } catch (err: any) {
            // Extract error message from RFC 7807 Problem Details format
            // Handle empty strings by checking truthy AND non-empty
            const data = err.response?.data;
            const errorObj = data?.error || data; // Handle both nested and flat structures
            let errorMessage =
              (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) ||
              (typeof errorObj?.title === 'string' && errorObj.title.trim()) ||
              (typeof data?.error === 'string' && data.error.trim()) ||
              (typeof err.message === 'string' && err.message.trim()) ||
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

        logout: () => {
          set({
            user: null,
            token: null,
            isAuthenticated: false,
            error: null,
            profile: null,
          });
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
      }),
      'authStore'
    ),
    {
      name: 'auth-storage',
      partialize: (state) => ({
        token: state.token,
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
