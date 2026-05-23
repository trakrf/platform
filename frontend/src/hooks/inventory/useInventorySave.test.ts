import React, { type ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useInventorySave } from './useInventorySave';
import { inventoryApi } from '@/lib/api/inventory';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import toast from 'react-hot-toast';

vi.mock('@/lib/api/inventory');
vi.mock('@/lib/auth/orgContext');
vi.mock('react-hot-toast');

const mockResponse = {
  count: 5,
  location_id: 10,
  location_name: 'Warehouse A',
  timestamp: '2026-03-24T00:00:00Z',
};

const mockRequest = {
  location_identifier: 'WH-01',
  asset_identifiers: ['ASSET-0001', 'ASSET-0002', 'ASSET-0003', 'ASSET-0004', 'ASSET-0005'],
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

  it('does not auto-retry on 403 (TRA-812: the prior retry path resubmitted the same payload that just failed)', async () => {
    const error403 = Object.assign(new Error('Request failed with status code 403'), {
      response: { status: 403 },
    });
    vi.mocked(inventoryApi.save).mockRejectedValueOnce(error403);

    const { result } = renderHook(() => useInventorySave(), {
      wrapper: createWrapper(),
    });

    await expect(result.current.save(mockRequest)).rejects.toThrow(error403);
    expect(inventoryApi.save).toHaveBeenCalledTimes(1);
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

  it('on 403 access-denied: surfaces the backend detail directly, no "org mismatch" rewrite (TRA-812)', async () => {
    // The backend now returns an accurate, generic detail naming the actual
    // failure ("N of M assets are unavailable"). The hook should pass it
    // straight through — it must not rewrite every 403 into a misleading
    // "no longer match your current organization" toast like it used to.
    const access403 = Object.assign(new Error('Request failed with status code 403'), {
      response: {
        status: 403,
        data: { error: { detail: '2 of 3 assets are unavailable; refresh and try again' } },
      },
    });
    vi.mocked(inventoryApi.save).mockRejectedValue(access403);

    const { result } = renderHook(() => useInventorySave(), { wrapper: createWrapper() });
    await expect(result.current.save(mockRequest)).rejects.toThrow(access403);

    expect(inventoryApi.save).toHaveBeenCalledTimes(1);
    expect(toast.error).toHaveBeenCalledWith(
      '2 of 3 assets are unavailable; refresh and try again',
    );
    expect(toast.error).not.toHaveBeenCalledWith(
      expect.stringMatching(/no longer match your current organization/i),
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
