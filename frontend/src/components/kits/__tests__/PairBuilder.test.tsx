import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import PairBuilder from '@/components/kits/PairBuilder';
import { useKitStore } from '@/stores/kitStore';
import { kitsApi } from '@/lib/api/kits';
import type { TagInfo } from '@/stores/tagStore';

vi.mock('@/lib/api/kits', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/kits')>();
  return { ...actual, kitsApi: { ...actual.kitsApi, commission: vi.fn() } };
});
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn().mockResolvedValue(undefined),
}));

const tag = (epc: string, type: TagInfo['type'] = 'unknown'): TagInfo => ({
  epc,
  count: 1,
  source: 'scan',
  type,
});

function renderBuilder(tags: TagInfo[], onSaved?: () => void) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <PairBuilder tags={tags} onSaved={onSaved} />
    </QueryClientProvider>
  );
}

describe('PairBuilder (flattened kits surface)', () => {
  beforeEach(() => {
    useKitStore.setState({ pairSlots: { router: null, coupon: null } });
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

  it('renders nothing when there are no uncommissioned tags', () => {
    renderBuilder([]);
    expect(screen.queryByTestId('kit-pair-builder')).toBeNull();
  });

  it('auto-assigns the first two tags: router then coupon', async () => {
    renderBuilder([tag('RRR1'), tag('CCC2')]);
    await waitFor(() => {
      expect(screen.getByTestId('kit-slot-router')).toHaveTextContent('RRR1');
      expect(screen.getByTestId('kit-slot-coupon')).toHaveTextContent('CCC2');
    });
  });

  it('swap flips router and coupon', async () => {
    renderBuilder([tag('RRR1'), tag('CCC2')]);
    await waitFor(() => {
      expect(screen.getByTestId('kit-slot-coupon')).toHaveTextContent('CCC2');
    });
    fireEvent.click(screen.getByTestId('kit-pair-swap'));
    expect(screen.getByTestId('kit-slot-router')).toHaveTextContent('CCC2');
    expect(screen.getByTestId('kit-slot-coupon')).toHaveTextContent('RRR1');
  });

  it('assigning a tag to a slot evicts it from the other slot', async () => {
    renderBuilder([tag('RRR1'), tag('CCC2')]);
    await waitFor(() => {
      expect(screen.getByTestId('kit-slot-router')).toHaveTextContent('RRR1');
    });
    fireEvent.click(screen.getByTestId('kit-assign-coupon-RRR1'));
    expect(screen.getByTestId('kit-slot-coupon')).toHaveTextContent('RRR1');
    expect(screen.getByTestId('kit-slot-router')).not.toHaveTextContent('RRR1');
  });

  it('saves the pair with router/coupon roles and QA fields, then notifies', async () => {
    const onSaved = vi.fn();
    renderBuilder([tag('RRR1'), tag('CCC2')], onSaved);
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
    await waitFor(() => {
      expect(onSaved).toHaveBeenCalled();
    });
  });

  it('releases a slot when its tag leaves the bucket', async () => {
    const { rerender } = renderBuilder([tag('RRR1'), tag('CCC2')]);
    await waitFor(() => {
      expect(screen.getByTestId('kit-slot-router')).toHaveTextContent('RRR1');
    });
    const client = new QueryClient();
    rerender(
      <QueryClientProvider client={client}>
        <PairBuilder tags={[tag('CCC2')]} />
      </QueryClientProvider>
    );
    await waitFor(() => {
      expect(screen.getByTestId('kit-slot-router')).not.toHaveTextContent('RRR1');
    });
    expect(screen.getByTestId('kit-save')).toBeDisabled();
  });
});
