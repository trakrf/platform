/**
 * Tests for useOrgModal hook - TRA-204 regression prevention
 */

import React, { type ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useOrgModal } from './useOrgModal';

// Mock dependencies
vi.mock('@/stores', () => ({
  useOrgStore: vi.fn(() => ({
    currentOrg: { id: 1, name: 'Test Org' },
    currentRole: 'owner',
    isLoading: false,
  })),
  useAuthStore: vi.fn(() => ({
    profile: { id: 1 },
    fetchProfile: vi.fn().mockResolvedValue(undefined),
  })),
}));

vi.mock('@/hooks/orgs/useOrgSwitch', () => ({
  useOrgSwitch: vi.fn(() => ({
    createOrg: vi.fn().mockResolvedValue({ id: 2, name: 'New Org' }),
  })),
}));

vi.mock('@/lib/api/orgs', () => ({
  orgsApi: {
    listMembers: vi.fn().mockResolvedValue({ data: { data: [] } }),
    delete: vi.fn().mockResolvedValue({}),
  },
}));

vi.mock('@/lib/auth/orgContext', () => ({
  refreshOrgToken: vi.fn().mockResolvedValue(true),
}));

vi.mock('@/lib/cache/orgScopedCache', () => ({
  invalidateAllOrgScopedData: vi.fn().mockResolvedValue(undefined),
}));

vi.mock('react-hot-toast', () => ({
  default: Object.assign(vi.fn(), {
    success: vi.fn(),
    error: vi.fn(),
  }),
}));

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useOrgModal', () => {
  const mockOnClose = vi.fn();

  const defaultProps = {
    isOpen: true,
    onClose: mockOnClose,
    mode: 'manage' as const,
    defaultTab: 'members' as const,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('TRA-204: showDeleteModal state management', () => {
    it('initializes showDeleteModal as false', () => {
      const { result } = renderHook(() => useOrgModal(defaultProps), { wrapper: createWrapper() });
      expect(result.current.showDeleteModal).toBe(false);
    });

    it('resets showDeleteModal when modal opens in manage mode', () => {
      const { result, rerender } = renderHook(
        ({ isOpen }) => useOrgModal({ ...defaultProps, isOpen }),
        { initialProps: { isOpen: false }, wrapper: createWrapper() }
      );

      // Simulate having stale state by opening delete modal
      act(() => {
        result.current.openDeleteModal();
      });
      expect(result.current.showDeleteModal).toBe(true);

      // Close and reopen modal
      rerender({ isOpen: false });
      rerender({ isOpen: true });

      // showDeleteModal should be reset to false
      expect(result.current.showDeleteModal).toBe(false);
    });

    it('resets showDeleteModal after successful org deletion', async () => {
      const { result } = renderHook(() => useOrgModal(defaultProps), { wrapper: createWrapper() });

      // Open delete modal
      act(() => {
        result.current.openDeleteModal();
      });
      expect(result.current.showDeleteModal).toBe(true);

      // Perform deletion
      await act(async () => {
        await result.current.handleDeleteOrg('Test Org');
      });

      // showDeleteModal should be reset
      expect(result.current.showDeleteModal).toBe(false);
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  describe('openDeleteModal and closeDeleteModal', () => {
    it('opens and closes delete modal', () => {
      const { result } = renderHook(() => useOrgModal(defaultProps), { wrapper: createWrapper() });

      expect(result.current.showDeleteModal).toBe(false);

      act(() => {
        result.current.openDeleteModal();
      });
      expect(result.current.showDeleteModal).toBe(true);

      act(() => {
        result.current.closeDeleteModal();
      });
      expect(result.current.showDeleteModal).toBe(false);
    });
  });
});
