import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAuthStore } from './authStore';
import { authApi } from '@/lib/api/auth';
import { orgsApi } from '@/lib/api/orgs';
import { jwtDecode } from 'jwt-decode';

// Mock the APIs
vi.mock('@/lib/api/auth');
vi.mock('@/lib/api/orgs');

// Mock jwt-decode
vi.mock('jwt-decode');

// Mock the cache invalidation module
vi.mock('@/lib/cache/orgScopedCache', () => ({
  invalidateAllOrgScopedData: vi.fn().mockResolvedValue(undefined),
}));

// Mock the queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryClient: {},
}));

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

      // Mock getProfile to return a profile with current_org
      vi.mocked(orgsApi.getProfile).mockResolvedValue({
        data: {
          data: {
            id: 1,
            email: 'test@example.com',
            name: 'Test User',
            current_org: { id: 1, name: 'Test Org', role: 'owner' as const },
            orgs: [{ id: 1, name: 'Test Org' }],
          },
        },
      } as any);

      // Mock setCurrentOrg to return a token with org_id
      vi.mocked(orgsApi.setCurrentOrg).mockResolvedValue({
        data: { token: 'test-token-with-org', message: 'ok' },
      } as any);

      await useAuthStore.getState().login('test@example.com', 'password123');

      const state = useAuthStore.getState();
      // Token is updated to the one with org_id claim
      expect(state.token).toBe('test-token-with-org');
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

      // Mock getProfile to return a profile with current_org
      vi.mocked(orgsApi.getProfile).mockResolvedValue({
        data: {
          data: {
            id: 2,
            email: 'newuser@example.com',
            name: 'New User',
            current_org: { id: 1, name: 'New Org', role: 'owner' as const },
            orgs: [{ id: 1, name: 'New Org' }],
          },
        },
      } as any);

      // Mock setCurrentOrg to return a token with org_id
      vi.mocked(orgsApi.setCurrentOrg).mockResolvedValue({
        data: { token: 'new-user-token-with-org', message: 'ok' },
      } as any);

      await useAuthStore.getState().signup('newuser@example.com', 'password123');

      const state = useAuthStore.getState();
      // Token is updated to the one with org_id claim
      expect(state.token).toBe('new-user-token-with-org');
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
        useAuthStore.getState().signup('existing@example.com', 'password')
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
    beforeEach(() => {
      vi.clearAllMocks();
    });

    it('should set isAuthenticated to true if token is valid and not expired', () => {
      const futureTimestamp = Math.floor(Date.now() / 1000) + 3600; // 1 hour from now

      vi.mocked(jwtDecode).mockReturnValue({
        exp: futureTimestamp,
        user_id: 1,
        email: 'test@example.com',
      });

      useAuthStore.setState({
        token: 'valid-token',
        user: {
          id: 1,
          email: 'test@example.com',
          name: 'Test',
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
        isAuthenticated: false,
      });

      useAuthStore.getState().initialize();

      expect(useAuthStore.getState().isAuthenticated).toBe(true);
      expect(useAuthStore.getState().token).toBe('valid-token');
      expect(jwtDecode).toHaveBeenCalledWith('valid-token');
    });

    it('should clear auth state if token is expired', () => {
      const pastTimestamp = Math.floor(Date.now() / 1000) - 3600; // 1 hour ago

      vi.mocked(jwtDecode).mockReturnValue({
        exp: pastTimestamp,
        user_id: 1,
        email: 'test@example.com',
      });

      useAuthStore.setState({
        token: 'expired-token',
        user: {
          id: 1,
          email: 'test@example.com',
          name: 'Test',
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
        isAuthenticated: true,
      });

      useAuthStore.getState().initialize();

      expect(useAuthStore.getState().isAuthenticated).toBe(false);
      expect(useAuthStore.getState().token).toBeNull();
      expect(useAuthStore.getState().user).toBeNull();
    });

    it('should clear auth state if JWT decode fails (malformed token)', () => {
      vi.mocked(jwtDecode).mockImplementation(() => {
        throw new Error('Invalid token format');
      });

      useAuthStore.setState({
        token: 'malformed-token',
        user: {
          id: 1,
          email: 'test@example.com',
          name: 'Test',
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
        isAuthenticated: true,
      });

      useAuthStore.getState().initialize();

      expect(useAuthStore.getState().isAuthenticated).toBe(false);
      expect(useAuthStore.getState().token).toBeNull();
      expect(useAuthStore.getState().user).toBeNull();
    });

    it('should set isAuthenticated to false if no token exists', () => {
      useAuthStore.setState({
        token: null,
        user: null,
        isAuthenticated: true,
      });

      useAuthStore.getState().initialize();

      expect(useAuthStore.getState().isAuthenticated).toBe(false);
      expect(jwtDecode).not.toHaveBeenCalled();
    });

    it('should handle missing exp claim gracefully', () => {
      vi.mocked(jwtDecode).mockReturnValue({
        user_id: 1,
        email: 'test@example.com',
        // No exp claim
      });

      useAuthStore.setState({
        token: 'token-without-exp',
        user: {
          id: 1,
          email: 'test@example.com',
          name: 'Test',
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
        isAuthenticated: false,
      });

      useAuthStore.getState().initialize();

      // Without exp claim, token is considered valid
      expect(useAuthStore.getState().isAuthenticated).toBe(true);
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

      // Mock getProfile to return a profile with current_org
      vi.mocked(orgsApi.getProfile).mockResolvedValue({
        data: {
          data: {
            id: 3,
            email: 'persist@example.com',
            name: 'Persist Test',
            current_org: { id: 1, name: 'Test Org', role: 'owner' as const },
            orgs: [{ id: 1, name: 'Test Org' }],
          },
        },
      } as any);

      // Mock setCurrentOrg to return a token with org_id
      vi.mocked(orgsApi.setCurrentOrg).mockResolvedValue({
        data: { token: 'persist-test-token-with-org', message: 'ok' },
      } as any);

      await useAuthStore.getState().login('persist@example.com', 'password');

      // Check localStorage
      const stored = localStorage.getItem('auth-storage');
      expect(stored).toBeTruthy();

      const parsed = JSON.parse(stored!);
      // Token is updated to the one with org_id claim
      expect(parsed.state.token).toBe('persist-test-token-with-org');
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
