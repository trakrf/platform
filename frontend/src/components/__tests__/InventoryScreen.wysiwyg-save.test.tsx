import React, { type ReactNode } from 'react';
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import InventoryScreen from '@/components/InventoryScreen';
import { useTagStore, useAuthStore } from '@/stores';
import { useOrgStore } from '@/stores/orgStore';
import type { TagInfo } from '@/stores/tagStore';

const save = vi.fn().mockResolvedValue({ count: 0, location_name: 'Dock A' });

vi.mock('@/hooks/inventory/useInventorySave', () => ({
  useInventorySave: () => ({ save, isSaving: false, saveError: null }),
}));

// The screen mounts useAssets()/useLocations() on render; stub both so the
// suite never issues real requests (TRA-1052) and the location the save
// payload resolves against is deterministic.
vi.mock('@/hooks/assets', () => ({
  useAssets: () => ({ assets: [], isLoading: false }),
}));
vi.mock('@/hooks/locations', () => ({
  useLocations: () => ({
    locations: [{ id: 7, name: 'Dock A', external_key: 'DOCK-A' }],
    isLoading: false,
  }),
}));

const renderScreen = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) =>
    React.createElement(QueryClientProvider, { client: queryClient }, children);
  return render(<InventoryScreen />, { wrapper: Wrapper });
};

// A scanned location tag resolves the save target without touching the picker.
const locationTag = (): TagInfo => ({
  epc: 'LOC00000007',
  displayEpc: 'LOC00000007',
  rssi: -30,
  count: 1,
  timestamp: Date.now(),
  source: 'scan',
  type: 'location',
  locationId: 7,
  locationName: 'Dock A',
});

const assetTag = (n: number, extra: Partial<TagInfo> = {}): TagInfo => ({
  epc: `E28068940000000000000${n}`,
  displayEpc: `E28068940000000000000${n}`,
  rssi: -40,
  count: 1,
  timestamp: Date.now(),
  source: 'scan',
  type: 'asset',
  assetId: n,
  assetIdentifier: `ASSET-${n}`,
  ...extra,
});

// A CSV row that was never physically scanned. Deliberately typed as an
// asset: the invariant must hold on `source`, not on the classification
// accident that recon stubs are currently created as 'unknown'.
const reconOnlyTag = (n: number): TagInfo => ({
  epc: `E28068940000000000000${n}`,
  displayEpc: `E28068940000000000000${n}`,
  count: 0,
  reconciled: false,
  source: 'reconciliation',
  type: 'asset',
  assetId: n,
  assetIdentifier: `ASSET-${n}`,
});

const clickSave = () => {
  const buttons = screen.getAllByTitle(/^Save \d+ assets$/);
  fireEvent.click(buttons[buttons.length - 1]);
};

const assetsTile = () => screen.getByRole('button', { name: /^Assets/ });

describe('InventoryScreen WYSIWYG save (TRA-1038)', () => {
  beforeEach(() => {
    save.mockClear();
    localStorage.removeItem('inventory-status-filters');
    useTagStore.getState().clearTags();
    useTagStore.getState().setCurrentPage(1);
    useTagStore.getState().setPageSize(10);
    useTagStore.getState().setSortConfig('timestamp', 'desc');
    useAuthStore.setState({
      isAuthenticated: true,
      token: 'test-token',
      user: { id: 1, email: 't@e.st' } as never,
    });
    useOrgStore.setState({ currentOrg: null } as never);
  });

  afterEach(() => cleanup());

  it('saves every recognized asset when no filter is active', async () => {
    useTagStore.getState().setTags([locationTag(), assetTag(1), assetTag(2), assetTag(3)]);
    renderScreen();

    await waitFor(() => expect(screen.getAllByTitle(/^Save 3 assets$/).length).toBeGreaterThan(0));
    clickSave();

    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    const payload = save.mock.calls[0][0];
    expect(payload.location_identifier).toBe('DOCK-A');
    expect([...payload.asset_identifiers].sort()).toEqual(['ASSET-1', 'ASSET-2', 'ASSET-3']);
  });

  it('saves only the assets left visible by the search box', async () => {
    useTagStore.getState().setTags([locationTag(), assetTag(1), assetTag(2), assetTag(3)]);
    renderScreen();

    const search = screen.getAllByPlaceholderText(/Search for an item/i)[0];
    fireEvent.change(search, { target: { value: '00002' } });

    await waitFor(() => expect(screen.getAllByTitle(/^Save 1 assets$/).length).toBeGreaterThan(0));
    clickSave();

    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save.mock.calls[0][0].asset_identifiers).toEqual(['ASSET-2']);
  });

  it('saves only the assets left visible by a tile filter', async () => {
    useTagStore.getState().setTags([
      locationTag(),
      assetTag(1, { reconciled: true }),
      assetTag(2, { reconciled: null }),
    ]);
    renderScreen();

    fireEvent.click(screen.getByRole('button', { name: /^Found/ }));

    await waitFor(() => expect(screen.getAllByTitle(/^Save 1 assets$/).length).toBeGreaterThan(0));
    clickSave();

    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save.mock.calls[0][0].asset_identifiers).toEqual(['ASSET-1']);
  });

  it('never saves a reconciliation-only row, even when classified as an asset', async () => {
    useTagStore.getState().setTags([locationTag(), assetTag(1, { reconciled: true }), reconOnlyTag(2)]);
    renderScreen();

    await waitFor(() => expect(screen.getAllByTitle(/^Save 1 assets$/).length).toBeGreaterThan(0));
    clickSave();

    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save.mock.calls[0][0].asset_identifiers).toEqual(['ASSET-1']);
  });

  it('disables Save when the Missing filter leaves only recon-only rows', async () => {
    useTagStore.getState().setTags([locationTag(), assetTag(1, { reconciled: true }), reconOnlyTag(2)]);
    renderScreen();

    fireEvent.click(screen.getByRole('button', { name: /^Missing/ }));

    await waitFor(() => {
      const buttons = screen.getAllByTitle('Nothing in view to save');
      expect(buttons.length).toBeGreaterThan(0);
      expect(buttons.every(b => (b as HTMLButtonElement).disabled)).toBe(true);
    });
  });

  it('counts the Assets tile as the save manifest under an active filter', async () => {
    useTagStore.getState().setTags([locationTag(), assetTag(1), assetTag(2), assetTag(3)]);
    renderScreen();

    await waitFor(() => expect(within(assetsTile()).getByText('3')).toBeInTheDocument());

    const search = screen.getAllByPlaceholderText(/Search for an item/i)[0];
    fireEvent.change(search, { target: { value: '00002' } });

    await waitFor(() => expect(within(assetsTile()).getByText('1')).toBeInTheDocument());
  });

  it('dedups multi-tag assets in the save payload', async () => {
    useTagStore.getState().setTags([
      locationTag(),
      assetTag(1),
      { ...assetTag(1), epc: 'E280689400000000000011', displayEpc: 'E280689400000000000011' },
    ]);
    renderScreen();

    await waitFor(() => expect(screen.getAllByTitle(/^Save 1 assets$/).length).toBeGreaterThan(0));
    clickSave();

    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save.mock.calls[0][0].asset_identifiers).toEqual(['ASSET-1']);
  });
});
