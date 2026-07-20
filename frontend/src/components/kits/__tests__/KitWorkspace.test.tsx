import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act, cleanup, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import KitWorkspace from '@/components/kits/KitWorkspace';
import { useTagStore } from '@/stores/tagStore';
import { useDeviceStore } from '@/stores/deviceStore';
import { useKitStore } from '@/stores/kitStore';
import { ReaderState } from '@/worker/types/reader';
import { kitsApi, type VerifyResponse, type KitSummary } from '@/lib/api/kits';

vi.mock('@/lib/api/kits', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/kits')>();
  return {
    ...actual,
    kitsApi: {
      ...actual.kitsApi,
      verify: vi.fn(),
      listByMemberEpc: vi.fn(),
      commission: vi.fn(),
      search: vi.fn(),
    },
  };
});
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn().mockResolvedValue(undefined),
}));

const emptyResult: VerifyResponse = { kits: [], unexpected: [], unknown_epcs: [] };

const owningKit: KitSummary = {
  id: 9,
  label: '1184015',
  status: 'active',
  created_at: '2026-07-19T00:00:00Z',
  member_count: 2,
  latest_verification: null,
};

function renderWorkspace() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <KitWorkspace />
    </QueryClientProvider>
  );
}

describe('KitWorkspace (flattened kits surface)', () => {
  beforeEach(() => {
    vi.mocked(kitsApi.verify).mockResolvedValue({ data: emptyResult } as never);
    vi.mocked(kitsApi.listByMemberEpc).mockResolvedValue({ data: { data: [] } } as never);
    useTagStore.setState({ tags: [] });
    useKitStore.setState({ verifyResult: null, pairSlots: { router: null, coupon: null } });
    useDeviceStore.setState({ readerState: ReaderState.CONNECTED });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('checks pairs automatically when scanning stops with tags in the session', async () => {
    renderWorkspace();

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

  it('does not auto-check an empty scan session', async () => {
    renderWorkspace();

    act(() => {
      useDeviceStore.setState({ readerState: ReaderState.SCANNING });
    });
    act(() => {
      useDeviceStore.setState({ readerState: ReaderState.CONNECTED });
    });

    await new Promise((r) => setTimeout(r, 50));
    expect(kitsApi.verify).not.toHaveBeenCalled();
  });

  it('routes unpaired scanned tags into the pair builder', async () => {
    useTagStore.setState({
      tags: [
        { epc: 'NEW1', count: 1, source: 'scan', type: 'unknown' },
        { epc: 'NEW2', count: 1, source: 'scan', type: 'asset' },
      ],
    });
    renderWorkspace();

    await waitFor(() => {
      expect(screen.getByTestId('kit-pair-builder')).toHaveTextContent('New tags (2)');
    });
  });

  it('keeps already-paired tags out of the pair builder', async () => {
    vi.mocked(kitsApi.listByMemberEpc).mockImplementation(async (epc: string) =>
      ({ data: { data: epc === 'OLD1' ? [owningKit] : [] } }) as never
    );
    useTagStore.setState({
      tags: [
        { epc: 'OLD1', count: 1, source: 'scan', type: 'asset' },
        { epc: 'NEW2', count: 1, source: 'scan', type: 'unknown' },
      ],
    });
    renderWorkspace();

    await waitFor(() => {
      expect(screen.getByTestId('kit-pair-builder')).toHaveTextContent('New tags (1)');
    });
    expect(screen.getByTestId('kit-pair-builder')).not.toHaveTextContent('OLD1');
  });

  it('Clear resets the whole session including search results', async () => {
    useTagStore.setState({
      tags: [{ epc: 'AAA1', count: 1, source: 'scan', type: 'asset' }],
    });
    useKitStore.setState({ searchQuery: '1184015', verifyResult: emptyResult });
    renderWorkspace();

    const { fireEvent } = await import('@testing-library/react');
    fireEvent.click(screen.getByTestId('kit-verify-clear'));

    expect(useTagStore.getState().tags).toHaveLength(0);
    expect(useKitStore.getState().searchQuery).toBe('');
    expect(useKitStore.getState().verifyResult).toBeNull();
  });

  it('renders the pair tree from the stored check result', async () => {
    useKitStore.setState({
      verifyResult: {
        kits: [
          {
            kit_id: 2,
            label: '1184016',
            result: 'incomplete',
            metadata: {},
            seen: [{ asset_id: 20, role: 'router', name: 'r', epcs: ['DDD444'] }],
            missing: [{ asset_id: 21, role: 'coupon', name: 'c', epcs: ['AAA111'] }],
          },
        ],
        unexpected: [],
        unknown_epcs: [],
      },
    });
    renderWorkspace();

    expect(screen.getByTestId('kit-result-incomplete-2')).toHaveTextContent('1184016');
    expect(screen.getByTestId('kit-locate-AAA111')).toBeInTheDocument();
  });
});
