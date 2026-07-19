import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import CommissionKit from '@/components/kits/CommissionKit';
import { useTagStore } from '@/stores/tagStore';
import { useKitStore } from '@/stores/kitStore';
import { kitsApi, type KitSummary } from '@/lib/api/kits';

vi.mock('@/lib/api/kits', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/kits')>();
  return {
    ...actual,
    kitsApi: { ...actual.kitsApi, commission: vi.fn(), listByMemberEpc: vi.fn() },
  };
});
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn().mockResolvedValue(undefined),
}));

const owningKit: KitSummary = {
  id: 9,
  label: '1184015',
  status: 'active',
  created_at: '2026-07-19T00:00:00Z',
  member_count: 2,
  latest_verification: null,
};

function renderCommissionKit() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <CommissionKit />
    </QueryClientProvider>
  );
}

describe('CommissionKit already-kitted flagging (TRA-1033)', () => {
  beforeEach(() => {
    useTagStore.setState({
      tags: [
        { epc: 'AAA1', count: 1, source: 'scan', type: 'asset' },
        { epc: 'BBB2', count: 1, source: 'scan', type: 'asset' },
        { epc: 'CCC3', count: 1, source: 'scan', type: 'unknown' },
      ],
    });
    useKitStore.setState({ memberRoles: {} });
    vi.mocked(kitsApi.listByMemberEpc).mockImplementation(async (epc: string) =>
      ({ data: { data: epc === 'BBB2' ? [owningKit] : [] } }) as never
    );
    vi.mocked(kitsApi.commission).mockResolvedValue({
      data: {
        data: {
          id: 1,
          label: 'Lot-1',
          status: 'active',
          created_at: '',
          updated_at: '',
          members: [
            { asset_id: 1, role: null, name: 'a', epcs: ['AAA1'] },
            { asset_id: 2, role: null, name: 'c', epcs: ['CCC3'] },
          ],
          latest_verification: null,
        },
      },
    } as never);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('flags a tag already in an active kit and drops its role input', async () => {
    renderCommissionKit();

    await waitFor(() => {
      expect(screen.getByTestId('kit-member-owned-BBB2')).toHaveTextContent('in kit 1184015');
    });
    expect(screen.queryByTestId('kit-role-input-BBB2')).toBeNull();
    expect(screen.getByTestId('kit-role-input-AAA1')).toBeInTheDocument();
    // Eligible count excludes the already-kitted tag
    expect(screen.getByText(/Members \(2\)/)).toBeInTheDocument();
  });

  it('does not flag membership in a closed kit', async () => {
    vi.mocked(kitsApi.listByMemberEpc).mockImplementation(async (epc: string) =>
      ({
        data: { data: epc === 'BBB2' ? [{ ...owningKit, status: 'closed' }] : [] },
      }) as never
    );
    renderCommissionKit();

    await waitFor(() => {
      expect(kitsApi.listByMemberEpc).toHaveBeenCalledWith('BBB2');
    });
    expect(screen.queryByTestId('kit-member-owned-BBB2')).toBeNull();
    expect(screen.getByTestId('kit-role-input-BBB2')).toBeInTheDocument();
  });

  it('excludes already-kitted tags from the commission request', async () => {
    renderCommissionKit();
    await waitFor(() => {
      expect(screen.getByTestId('kit-member-owned-BBB2')).toBeInTheDocument();
    });

    fireEvent.change(screen.getByTestId('kit-label-input'), { target: { value: 'Lot-1' } });
    fireEvent.click(screen.getByTestId('kit-save'));

    await waitFor(() => {
      expect(kitsApi.commission).toHaveBeenCalledWith({
        label: 'Lot-1',
        members: [{ epc: 'AAA1' }, { epc: 'CCC3' }],
      });
    });
  });
});
