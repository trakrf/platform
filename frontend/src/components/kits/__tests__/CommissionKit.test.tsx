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

describe('CommissionKit pair model (TRA-1033)', () => {
  beforeEach(() => {
    useTagStore.setState({
      tags: [
        { epc: 'RRR1', count: 1, source: 'scan', type: 'asset' },
        { epc: 'CCC2', count: 1, source: 'scan', type: 'unknown' },
      ],
    });
    useKitStore.setState({ pairSlots: { router: null, coupon: null } });
    vi.mocked(kitsApi.listByMemberEpc).mockResolvedValue({ data: { data: [] } } as never);
    vi.mocked(kitsApi.commission).mockResolvedValue({
      data: {
        data: {
          id: 1,
          label: 'Lot-1',
          status: 'active',
          metadata: {},
          created_at: '',
          updated_at: '',
          members: [
            { asset_id: 1, role: 'router', name: 'r', epcs: ['RRR1'] },
            { asset_id: 2, role: 'coupon', name: 'c', epcs: ['CCC2'] },
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

  it('auto-assigns the first two scanned tags: router then coupon', async () => {
    renderCommissionKit();
    await waitFor(() => {
      expect(screen.getByTestId('kit-slot-router')).toHaveTextContent('RRR1');
      expect(screen.getByTestId('kit-slot-coupon')).toHaveTextContent('CCC2');
    });
  });

  it('swap flips router and coupon', async () => {
    renderCommissionKit();
    await waitFor(() => {
      expect(screen.getByTestId('kit-slot-coupon')).toHaveTextContent('CCC2');
    });
    fireEvent.click(screen.getByTestId('kit-pair-swap'));
    expect(screen.getByTestId('kit-slot-router')).toHaveTextContent('CCC2');
    expect(screen.getByTestId('kit-slot-coupon')).toHaveTextContent('RRR1');
  });

  it('assigning a tag to a slot evicts it from the other slot', async () => {
    renderCommissionKit();
    await waitFor(() => {
      expect(screen.getByTestId('kit-slot-router')).toHaveTextContent('RRR1');
    });
    // Move RRR1 to the coupon slot: coupon=RRR1, router must not keep it
    fireEvent.click(screen.getByTestId('kit-assign-coupon-RRR1'));
    expect(screen.getByTestId('kit-slot-coupon')).toHaveTextContent('RRR1');
    expect(screen.getByTestId('kit-slot-router')).not.toHaveTextContent('RRR1');
  });

  it('saves the pair with router/coupon roles and QA fields', async () => {
    renderCommissionKit();
    await waitFor(() => {
      expect(screen.getByTestId('kit-slot-coupon')).toHaveTextContent('CCC2');
    });

    fireEvent.change(screen.getByTestId('kit-label-input'), { target: { value: 'Lot-1' } });
    fireEvent.change(screen.getByTestId('kit-qa-part'), { target: { value: 'PN-778' } });
    fireEvent.click(screen.getByTestId('kit-save'));

    await waitFor(() => {
      expect(kitsApi.commission).toHaveBeenCalledWith({
        label: 'Lot-1',
        members: [
          { epc: 'RRR1', role: 'router' },
          { epc: 'CCC2', role: 'coupon' },
        ],
        metadata: { part: 'PN-778' },
      });
    });
  });

  it('evicts a tag from the slots once it resolves as already-paired', async () => {
    vi.mocked(kitsApi.listByMemberEpc).mockImplementation(async (epc: string) =>
      ({ data: { data: epc === 'RRR1' ? [owningKit] : [] } }) as never
    );
    renderCommissionKit();

    await waitFor(() => {
      expect(screen.getByTestId('kit-member-owned-RRR1')).toHaveTextContent('in Lot 1184015');
    });
    await waitFor(() => {
      // RRR1 was auto-assigned pre-resolution, then evicted; CCC2 keeps its
      // slot and the pair stays incomplete.
      const slots = [
        screen.getByTestId('kit-slot-router').textContent,
        screen.getByTestId('kit-slot-coupon').textContent,
      ].join(' ');
      expect(slots).not.toContain('RRR1');
      expect(slots).toContain('CCC2');
      expect(slots).toContain('scan tag…');
    });
    expect(screen.getByTestId('kit-save')).toBeDisabled();
  });
});
