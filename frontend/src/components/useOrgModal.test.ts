/**
 * Tests for useOrgModal — create mode.
 *
 * TRA-1058 removed the manage half of this hook, and with it the TRA-204
 * showDeleteModal-reset tests: the delete flow now lives on OrgSettingsScreen,
 * which unmounts on navigation and cannot carry stale open-modal state across
 * a reopen the way this modal could.
 */

import React, { type ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useOrgModal } from './useOrgModal';
import { useOrgSwitch } from '@/hooks/orgs/useOrgSwitch';

const createOrgMock = vi.fn().mockResolvedValue({ id: 2, name: 'New Org' });

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
  useOrgSwitch: vi.fn(() => ({ createOrg: createOrgMock })),
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

const submit = () => ({ preventDefault: vi.fn() }) as unknown as React.FormEvent;

describe('useOrgModal (create)', () => {
  const mockOnClose = vi.fn();
  const defaultProps = { isOpen: true, onClose: mockOnClose };

  beforeEach(() => {
    vi.clearAllMocks();
    createOrgMock.mockResolvedValue({ id: 2, name: 'New Org' });
  });

  it('creates the org and closes on submit', async () => {
    const { result } = renderHook(() => useOrgModal(defaultProps), { wrapper: createWrapper() });

    act(() => result.current.setNewOrgName('New Org'));
    await act(async () => {
      await result.current.handleCreateOrg(submit());
    });

    // createOrg comes from useOrgSwitch, not the bare store action — it mints a
    // token for the new org and clears org-scoped caches.
    expect(vi.mocked(useOrgSwitch)).toHaveBeenCalled();
    expect(createOrgMock).toHaveBeenCalledWith('New Org');
    expect(mockOnClose).toHaveBeenCalled();
  });

  it('rejects a name shorter than two characters without calling the API', async () => {
    const { result } = renderHook(() => useOrgModal(defaultProps), { wrapper: createWrapper() });

    act(() => result.current.setNewOrgName('x'));
    await act(async () => {
      await result.current.handleCreateOrg(submit());
    });

    expect(result.current.createNameError).toMatch(/at least 2 characters/i);
    expect(createOrgMock).not.toHaveBeenCalled();
    expect(mockOnClose).not.toHaveBeenCalled();
  });

  it('surfaces a create failure and keeps the modal open', async () => {
    createOrgMock.mockRejectedValueOnce({
      response: { data: { error: { detail: 'Name already taken' } } },
    });
    const { result } = renderHook(() => useOrgModal(defaultProps), { wrapper: createWrapper() });

    act(() => result.current.setNewOrgName('Dupe Org'));
    await act(async () => {
      await result.current.handleCreateOrg(submit());
    });

    expect(result.current.createError).toBe('Name already taken');
    expect(mockOnClose).not.toHaveBeenCalled();
  });

  it('clears a previous attempt when the modal reopens', async () => {
    const { result, rerender } = renderHook(
      ({ isOpen }) => useOrgModal({ ...defaultProps, isOpen }),
      { initialProps: { isOpen: true }, wrapper: createWrapper() }
    );

    createOrgMock.mockRejectedValueOnce(new Error('boom'));
    act(() => result.current.setNewOrgName('Failed Org'));
    await act(async () => {
      await result.current.handleCreateOrg(submit());
    });
    expect(result.current.createError).toBe('boom');

    rerender({ isOpen: false });
    rerender({ isOpen: true });

    expect(result.current.newOrgName).toBe('');
    expect(result.current.createError).toBeNull();
  });
});
