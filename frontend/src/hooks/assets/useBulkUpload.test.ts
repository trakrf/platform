import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useBulkUpload } from './useBulkUpload';
import { assetsApi } from '@/lib/api/assets';

vi.mock('@/lib/api/assets');

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useBulkUpload', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should upload file and start polling', async () => {
    vi.mocked(assetsApi.uploadCSV).mockResolvedValue({
      data: {
        status: 'accepted',
        job_id: 'job123',
        status_url: '/api/v1/assets/bulk/job123',
        message: 'Upload accepted',
      },
    } as any);

    vi.mocked(assetsApi.getJobStatus).mockResolvedValue({
      data: {
        job_id: 'job123',
        status: 'processing',
        total_rows: 100,
        processed_rows: 50,
        failed_rows: 0,
        created_at: '2024-01-01',
      },
    } as any);

    const { result } = renderHook(() => useBulkUpload(), {
      wrapper: createWrapper(),
    });

    const file = new File(['test'], 'test.csv', { type: 'text/csv' });
    await result.current.upload(file);

    await waitFor(() => {
      expect(result.current.jobStatus).toBe('processing');
    });

    expect(assetsApi.uploadCSV).toHaveBeenCalledWith(file);
  });

  it('should handle upload errors', async () => {
    vi.mocked(assetsApi.uploadCSV).mockRejectedValue(
      new Error('Upload failed')
    );

    const { result } = renderHook(() => useBulkUpload(), {
      wrapper: createWrapper(),
    });

    const file = new File(['test'], 'test.csv', { type: 'text/csv' });

    await expect(result.current.upload(file)).rejects.toThrow('Upload failed');
  });
});
