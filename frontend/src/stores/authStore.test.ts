import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAuthStore } from './authStore';
import { authApi } from '@/lib/api/auth';

// Mock the API
vi.mock('@/lib/api/auth');

describe('authStore', () => {
  beforeEach(() => {
    // Clear localStorage before each test
    localStorage.clear();

    // Reset store state
    useAuthStore.setState({
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
      error: null,
    });

    vi.clearAllMocks();
  });

  describe('login', () => {
    it('should login successfully and store token + user', async () => {
      const mockResponse = {
        data: {
          data: {
            token: 'test-token-123',
            user: {
              id: 1,
              email: 'test@example.com',
              name: 'Test User',
              created_at: '2025-01-01T00:00:00Z',
              updated_at: '2025-01-01T00:00:00Z',
            },
          },
        },
      };

      vi.mocked(authApi.login).mockResolvedValue(mockResponse as any);

      await useAuthStore.getState().login('test@example.com', 'password123');

      const state = useAuthStore.getState();
      expect(state.token).toBe('test-token-123');
      expect(state.user?.email).toBe('test@example.com');
      expect(state.user?.name).toBe('Test User');
      expect(state.isAuthenticated).toBe(true);
      expect(state.isLoading).toBe(false);
      expect(state.error).toBeNull();
    });

    it('should handle login failure and set error message', async () => {
      const mockError = {
        response: {
          data: {
            error: 'Invalid credentials',
          },
        },
      };

      vi.mocked(authApi.login).mockRejectedValue(mockError);

      await expect(
        useAuthStore.getState().login('test@example.com', 'wrongpassword')
      ).rejects.toEqual(mockError);

      const state = useAuthStore.getState();
      expect(state.error).toBe('Invalid credentials');
      expect(state.isAuthenticated).toBe(false);
      expect(state.token).toBeNull();
      expect(state.user).toBeNull();
    });

    it('should use fallback error message if backend does not provide one', async () => {
      const mockError = {
        response: {},
      };

      vi.mocked(authApi.login).mockRejectedValue(mockError);

      await expect(
        useAuthStore.getState().login('test@example.com', 'password')
      ).rejects.toEqual(mockError);

      const state = useAuthStore.getState();
      expect(state.error).toBe('Login failed');
    });
  });

  describe('signup', () => {
    it('should signup successfully and store token + user', async () => {
      const mockResponse = {
        data: {
          data: {
            token: 'new-user-token',
            user: {
              id: 2,
              email: 'newuser@example.com',
              name: 'New User',
              created_at: '2025-01-01T00:00:00Z',
              updated_at: '2025-01-01T00:00:00Z',
            },
          },
        },
      };

      vi.mocked(authApi.signup).mockResolvedValue(mockResponse as any);

      await useAuthStore.getState().signup('newuser@example.com', 'password123', 'My Account');

      const state = useAuthStore.getState();
      expect(state.token).toBe('new-user-token');
      expect(state.user?.email).toBe('newuser@example.com');
      expect(state.isAuthenticated).toBe(true);
      expect(state.error).toBeNull();
    });

    it('should handle signup failure', async () => {
      const mockError = {
        response: {
          data: {
            error: 'Email already exists',
          },
        },
      };

      vi.mocked(authApi.signup).mockRejectedValue(mockError);

      await expect(
        useAuthStore.getState().signup('existing@example.com', 'password', 'Account')
      ).rejects.toEqual(mockError);

      const state = useAuthStore.getState();
      expect(state.error).toBe('Email already exists');
      expect(state.isAuthenticated).toBe(false);
    });
  });

  describe('logout', () => {
    it('should clear all auth state', () => {
      // Set some state first
      useAuthStore.setState({
        token: 'test-token',
        user: {
          id: 1,
          email: 'test@example.com',
          name: 'Test',
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
        isAuthenticated: true,
      });

      useAuthStore.getState().logout();

      const state = useAuthStore.getState();
      expect(state.token).toBeNull();
      expect(state.user).toBeNull();
      expect(state.isAuthenticated).toBe(false);
      expect(state.error).toBeNull();
    });
  });

  describe('initialize', () => {
    it('should set isAuthenticated to true if token and user exist', () => {
      useAuthStore.setState({
        token: 'persisted-token',
        user: {
          id: 1,
          email: 'test@example.com',
          name: 'Test',
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
        isAuthenticated: false, // Simulate after reload
      });

      useAuthStore.getState().initialize();

      expect(useAuthStore.getState().isAuthenticated).toBe(true);
    });

    it('should set isAuthenticated to false if no token', () => {
      useAuthStore.setState({
        token: null,
        user: null,
        isAuthenticated: true,
      });

      useAuthStore.getState().initialize();

      expect(useAuthStore.getState().isAuthenticated).toBe(false);
    });
  });

  describe('clearError', () => {
    it('should clear error state', () => {
      useAuthStore.setState({
        error: 'Some error message',
      });

      useAuthStore.getState().clearError();

      expect(useAuthStore.getState().error).toBeNull();
    });
  });

  describe('persistence', () => {
    it('should persist token and user to localStorage', async () => {
      const mockResponse = {
        data: {
          data: {
            token: 'persist-test-token',
            user: {
              id: 3,
              email: 'persist@example.com',
              name: 'Persist Test',
              created_at: '2025-01-01T00:00:00Z',
              updated_at: '2025-01-01T00:00:00Z',
            },
          },
        },
      };

      vi.mocked(authApi.login).mockResolvedValue(mockResponse as any);

      await useAuthStore.getState().login('persist@example.com', 'password');

      // Check localStorage
      const stored = localStorage.getItem('auth-storage');
      expect(stored).toBeTruthy();

      const parsed = JSON.parse(stored!);
      expect(parsed.state.token).toBe('persist-test-token');
      expect(parsed.state.user.email).toBe('persist@example.com');
    });

    it('should not persist isLoading or error state', async () => {
      const mockResponse = {
        data: {
          data: {
            token: 'test-token',
            user: {
              id: 1,
              email: 'test@example.com',
              name: 'Test',
              created_at: '2025-01-01T00:00:00Z',
              updated_at: '2025-01-01T00:00:00Z',
            },
          },
        },
      };

      vi.mocked(authApi.login).mockResolvedValue(mockResponse as any);

      await useAuthStore.getState().login('test@example.com', 'password');

      const stored = localStorage.getItem('auth-storage');
      const parsed = JSON.parse(stored!);

      // Only token and user should be persisted (partialize)
      expect(parsed.state.isLoading).toBeUndefined();
      expect(parsed.state.error).toBeUndefined();
      expect(parsed.state.isAuthenticated).toBeUndefined();
    });
  });
});
