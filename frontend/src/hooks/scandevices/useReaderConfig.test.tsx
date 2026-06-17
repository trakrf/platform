import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useReaderConfig } from './useReaderConfig';
import { scanDevicesApi } from '@/lib/api/scandevices';

vi.mock('@/lib/api/scandevices');

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  }
  return Wrapper;
}

const busyError = {
  response: { status: 409, data: { error: 'reader_busy', held_by: '192.168.50.203' } },
};

describe('useReaderConfig', () => {
  beforeEach(() => vi.clearAllMocks());

  it('surfaces a typed busy state on a 409 reader_busy', async () => {
    vi.mocked(scanDevicesApi.getReaderConfig).mockRejectedValue(busyError);
    const { result } = renderHook(() => useReaderConfig(10), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.busy).not.toBeNull());
    expect(result.current.busy?.held_by).toBe('192.168.50.203');
  });

  it('retryWithForce re-requests with force=true', async () => {
    vi.mocked(scanDevicesApi.getReaderConfig)
      .mockRejectedValueOnce(busyError)
      .mockResolvedValueOnce({ data: { data: { capabilities: { antennas: 1 }, config: { antennas: [] } } } } as never);
    const { result } = renderHook(() => useReaderConfig(10), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.busy).not.toBeNull());
    act(() => result.current.retryWithForce());
    await waitFor(() => expect(result.current.busy).toBeNull());
    expect(scanDevicesApi.getReaderConfig).toHaveBeenLastCalledWith(10, true);
  });
});
