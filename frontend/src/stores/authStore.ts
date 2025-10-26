import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { createStoreWithTracking } from './createStore';
import { authApi } from '@/lib/api/auth';
import type { User } from '@/lib/api/auth';

interface AuthState {
  // State
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;

  // Actions
  login: (email: string, password: string) => Promise<void>;
  signup: (email: string, password: string, accountName: string) => Promise<void>;
  logout: () => void;
  clearError: () => void;
  initialize: () => void;
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

        // Login action
        login: async (email: string, password: string) => {
          set({ isLoading: true, error: null });
          try {
            const response = await authApi.login({ email, password });
            const { token, user } = response.data.data;

            set({
              token,
              user,
              isAuthenticated: true,
              isLoading: false,
              error: null,
            });
          } catch (err: any) {
            const errorMessage = err.response?.data?.error || 'Login failed';
            set({
              error: errorMessage,
              isLoading: false,
            });
            throw err;
          }
        },

        // Signup action
        signup: async (email: string, password: string, accountName: string) => {
          set({ isLoading: true, error: null });
          try {
            const response = await authApi.signup({
              email,
              password,
              account_name: accountName,
            });
            const { token, user } = response.data.data;

            set({
              token,
              user,
              isAuthenticated: true,
              isLoading: false,
              error: null,
            });
          } catch (err: any) {
            const errorMessage = err.response?.data?.error || 'Signup failed';
            set({
              error: errorMessage,
              isLoading: false,
            });
            throw err;
          }
        },

        // Logout action
        logout: () => {
          set({
            user: null,
            token: null,
            isAuthenticated: false,
            error: null,
          });
        },

        // Clear error
        clearError: () => set({ error: null }),

        // Initialize - restore from persisted state
        initialize: () => {
          const state = get();
          if (state.token && state.user) {
            set({ isAuthenticated: true });
          } else {
            set({ isAuthenticated: false });
          }
        },
      }),
      'authStore' // OpenReplay tracking name
    ),
    {
      name: 'auth-storage',
      partialize: (state) => ({
        token: state.token,
        user: state.user,
      }),
      // Sanitize for OpenReplay - redact sensitive data
      onRehydrateStorage: () => (state) => {
        if (state) {
          // Sanitize token from OpenReplay tracking
          if ((window as any).__OPENREPLAY__) {
            console.log('AuthStore: Sanitizing sensitive data for OpenReplay');
          }
        }
      },
    }
  )
);
