import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, act, cleanup, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import VerifyKit from '@/components/kits/VerifyKit';
import { useTagStore } from '@/stores/tagStore';
import { useDeviceStore } from '@/stores/deviceStore';
import { useKitStore } from '@/stores/kitStore';
import { ReaderState } from '@/worker/types/reader';
import { kitsApi, type VerifyResponse } from '@/lib/api/kits';

vi.mock('@/lib/api/kits', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/kits')>();
  return { ...actual, kitsApi: { ...actual.kitsApi, verify: vi.fn() } };
});
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn().mockResolvedValue(undefined),
}));

const emptyResult: VerifyResponse = { kits: [], unexpected: [], unknown_epcs: [] };

function renderVerifyKit() {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <VerifyKit />
    </QueryClientProvider>
  );
}

describe('VerifyKit auto-verify (TRA-1033)', () => {
  beforeEach(() => {
    vi.mocked(kitsApi.verify).mockResolvedValue({ data: emptyResult } as never);
    useTagStore.setState({ tags: [] });
    useKitStore.setState({ verifyResult: null });
    useDeviceStore.setState({ readerState: ReaderState.CONNECTED });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('verifies automatically when scanning stops with tags in the session', async () => {
    renderVerifyKit();

    act(() => {
      useDeviceStore.setState({ readerState: ReaderState.SCANNING });
      useTagStore.setState({
        tags: [{ epc: 'AAA1', count: 1, source: 'scan', type: 'asset' }],
      });
    });
    act(() => {
      useDeviceStore.setState({ readerState: ReaderState.CONNECTED });
    });

    await waitFor(() => {
      expect(kitsApi.verify).toHaveBeenCalledWith({ epcs: ['AAA1'] });
    });
    await waitFor(() => {
      expect(useKitStore.getState().verifyResult).toEqual(emptyResult);
    });
  });

  it('does not auto-verify an empty scan session', async () => {
    renderVerifyKit();

    act(() => {
      useDeviceStore.setState({ readerState: ReaderState.SCANNING });
    });
    act(() => {
      useDeviceStore.setState({ readerState: ReaderState.CONNECTED });
    });

    await new Promise((r) => setTimeout(r, 50));
    expect(kitsApi.verify).not.toHaveBeenCalled();
  });

  it('does not auto-verify on non-scanning state transitions', async () => {
    useTagStore.setState({
      tags: [{ epc: 'AAA1', count: 1, source: 'scan', type: 'asset' }],
    });
    renderVerifyKit();

    act(() => {
      useDeviceStore.setState({ readerState: ReaderState.BUSY });
    });
    act(() => {
      useDeviceStore.setState({ readerState: ReaderState.CONNECTED });
    });

    await new Promise((r) => setTimeout(r, 50));
    expect(kitsApi.verify).not.toHaveBeenCalled();
  });
});
