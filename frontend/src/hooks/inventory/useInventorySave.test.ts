import React, { type ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useInventorySave } from './useInventorySave';
import { inventoryApi } from '@/lib/api/inventory';
import { ensureOrgContext, refreshOrgToken } from '@/lib/auth/orgContext';
import toast from 'react-hot-toast';

vi.mock('@/lib/api/inventory');
vi.mock('@/lib/auth/orgContext');
vi.mock('react-hot-toast');

// Central invalidation is the DRY sink for org-context drift (TRA-318).
// The hook delegates to it on storage-path 403 rather than clearing tags
// itself.
const mockInvalidateAllOrgScopedData = vi.fn().mockResolvedValue(undefined);
vi.mock('@/lib/cache/orgScopedCache', () => ({
  invalidateAllOrgScopedData: (...args: unknown[]) =>
    mockInvalidateAllOrgScopedData(...args),
}));

const mockResponse = {
  count: 5,
  location_id: 10,
  location_name: 'Warehouse A',
  timestamp: '2026-03-24T00:00:00Z',
};

const mockRequest = {
  location_id: 10,
  asset_ids: [1, 2, 3, 4, 5],
};

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useInventorySave', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(ensureOrgContext).mockResolvedValue(42);
  });

  it('shows success toast with count and location name on save', async () => {
    vi.mocked(inventoryApi.save).mockResolvedValue({
      data: { data: mockResponse },
    } as any);

    const { result } = renderHook(() => useInventorySave(), {
      wrapper: createWrapper(),
    });

    await result.current.save(mockRequest);

    expect(toast.success).toHaveBeenCalledWith('5 assets saved to Warehouse A');
  });

  it('calls ensureOrgContext before API call', async () => {
    vi.mocked(inventoryApi.save).mockResolvedValue({
      data: { data: mockResponse },
    } as any);

    const callOrder: string[] = [];
    vi.mocked(ensureOrgContext).mockImplementation(async () => {
      callOrder.push('ensureOrgContext');
      return 42;
    });
    vi.mocked(inventoryApi.save).mockImplementation(async () => {
      callOrder.push('inventoryApi.save');
      return { data: { data: mockResponse } } as any;
    });

    const { result } = renderHook(() => useInventorySave(), {
      wrapper: createWrapper(),
    });

    await result.current.save(mockRequest);

    expect(ensureOrgContext).toHaveBeenCalledOnce();
    expect(callOrder).toEqual(['ensureOrgContext', 'inventoryApi.save']);
  });

  it('retries once after refreshOrgToken on 403', async () => {
    const error403 = Object.assign(new Error('Request failed with status code 403'), {
      response: { status: 403 },
    });
    vi.mocked(inventoryApi.save)
      .mockRejectedValueOnce(error403)
      .mockResolvedValueOnce({ data: { data: mockResponse } } as any);
    vi.mocked(refreshOrgToken).mockResolvedValue(true);

    const { result } = renderHook(() => useInventorySave(), {
      wrapper: createWrapper(),
    });

    await result.current.save(mockRequest);

    expect(refreshOrgToken).toHaveBeenCalledOnce();
    expect(inventoryApi.save).toHaveBeenCalledTimes(2);
    expect(toast.success).toHaveBeenCalledWith('5 assets saved to Warehouse A');
  });

  it('throws error when 403 refresh fails', async () => {
    const error403 = Object.assign(new Error('Request failed with status code 403'), {
      response: { status: 403 },
    });
    vi.mocked(inventoryApi.save).mockRejectedValueOnce(error403);
    vi.mocked(refreshOrgToken).mockResolvedValue(false);

    const { result } = renderHook(() => useInventorySave(), {
      wrapper: createWrapper(),
    });

    await expect(result.current.save(mockRequest)).rejects.toThrow(error403);
    expect(refreshOrgToken).toHaveBeenCalledOnce();
    expect(toast.error).toHaveBeenCalledWith('Failed to save inventory');
  });

  it('shows error toast on non-403 failure', async () => {
    vi.mocked(inventoryApi.save).mockRejectedValue(new Error('Network error'));

    const { result } = renderHook(() => useInventorySave(), {
      wrapper: createWrapper(),
    });

    await expect(result.current.save(mockRequest)).rejects.toThrow('Network error');

    expect(toast.error).toHaveBeenCalledWith('Failed to save inventory');
  });

  it('isSaving reflects pending state', async () => {
    let resolveSave!: (value: any) => void;
    vi.mocked(inventoryApi.save).mockImplementation(
      () => new Promise((resolve) => { resolveSave = resolve; }),
    );

    const { result } = renderHook(() => useInventorySave(), {
      wrapper: createWrapper(),
    });

    expect(result.current.isSaving).toBe(false);

    let savePromise: Promise<any>;
    act(() => {
      savePromise = result.current.save(mockRequest);
    });

    await waitFor(() => {
      expect(result.current.isSaving).toBe(true);
    });

    resolveSave({ data: { data: mockResponse } });
    await savePromise!;

    await waitFor(() => {
      expect(result.current.isSaving).toBe(false);
    });
  });

  it('on 403 access-denied: calls central invalidation, then retries (TRA-426)', async () => {
    const access403 = Object.assign(new Error('Request failed with status code 403'), {
      response: {
        status: 403,
        data: { error: { detail: 'assets not found or access denied (org_id=7, valid=2/3)' } },
      },
    });
    vi.mocked(inventoryApi.save)
      .mockRejectedValueOnce(access403)
      .mockResolvedValueOnce({ data: { data: mockResponse } } as any);
    vi.mocked(refreshOrgToken).mockResolvedValue(true);

    const { result } = renderHook(() => useInventorySave(), { wrapper: createWrapper() });
    await result.current.save(mockRequest);

    expect(mockInvalidateAllOrgScopedData).toHaveBeenCalledOnce();
    expect(inventoryApi.save).toHaveBeenCalledTimes(2);
    expect(toast.success).toHaveBeenCalledWith('5 assets saved to Warehouse A');
  });

  it('surfaces backend detail in toast when 403 retry still fails (TRA-426)', async () => {
    const access403 = Object.assign(new Error('Request failed with status code 403'), {
      response: {
        status: 403,
        data: { error: { detail: 'assets not found or access denied (org_id=7, valid=2/3)' } },
      },
    });
    vi.mocked(inventoryApi.save).mockRejectedValue(access403);
    vi.mocked(refreshOrgToken).mockResolvedValue(true);

    const { result } = renderHook(() => useInventorySave(), { wrapper: createWrapper() });
    await expect(result.current.save(mockRequest)).rejects.toThrow(access403);

    expect(toast.error).toHaveBeenCalledWith(
      'Some scans no longer match your current organization. Please clear and rescan.',
    );
  });

  it('shows specific message on org context error', async () => {
    const orgError = new Error('No organization context. Please select an organization and try again.');
    vi.mocked(ensureOrgContext).mockRejectedValue(orgError);

    const { result } = renderHook(() => useInventorySave(), {
      wrapper: createWrapper(),
    });

    await expect(result.current.save(mockRequest)).rejects.toThrow(orgError);

    expect(toast.error).toHaveBeenCalledWith(
      'No organization context. Please select an organization and try again.',
    );
    expect(inventoryApi.save).not.toHaveBeenCalled();
  });
});
