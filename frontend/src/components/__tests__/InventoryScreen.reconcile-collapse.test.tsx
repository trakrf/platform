import React, { type ReactNode } from 'react';
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import InventoryScreen from '@/components/InventoryScreen';
import { useTagStore } from '@/stores';
import type { TagInfo } from '@/stores/tagStore';

const renderScreen = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) =>
    React.createElement(QueryClientProvider, { client: queryClient }, children);
  return render(<InventoryScreen />, { wrapper: Wrapper });
};

const scannedTag = (i: number, extra: Partial<TagInfo> = {}): TagInfo => ({
  epc: `E280689400000000000000${i.toString().padStart(2, '0')}`,
  displayEpc: `E280689400000000000000${i.toString().padStart(2, '0')}`,
  rssi: -40,
  count: 1,
  timestamp: Date.now(),
  source: 'scan',
  type: 'unknown',
  ...extra,
});

describe('InventoryScreen reconcile collapse (TRA-1036)', () => {
  beforeEach(() => {
    localStorage.removeItem('inventory-status-filters');
    useTagStore.getState().clearTags();
    useTagStore.getState().setCurrentPage(1);
    useTagStore.getState().setPageSize(10);
    useTagStore.getState().setSortConfig('timestamp', 'desc');
  });

  afterEach(() => cleanup());

  it('hides Status header and badges when no list is uploaded', async () => {
    useTagStore.getState().setTags([scannedTag(1), scannedTag(2)]);
    renderScreen();
    await waitFor(() => {
      expect(screen.getAllByText(/E2806894/).length).toBeGreaterThan(0);
    });
    expect(screen.queryByText('Status')).not.toBeInTheDocument();
    expect(screen.queryByText('Extra')).not.toBeInTheDocument();
    expect(screen.queryByText('Not Listed')).not.toBeInTheDocument();
  });

  it('shows the full reconcile surface when a list is present', async () => {
    useTagStore.getState().setTags([
      scannedTag(1, { reconciled: true }),
      scannedTag(2, { reconciled: false }),
      scannedTag(3),
    ]);
    renderScreen();
    await waitFor(() => {
      expect(screen.getByText('Status')).toBeInTheDocument();
    });
    expect(screen.getAllByText('Found').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Missing').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Extra').length).toBeGreaterThan(0);
  });

  it('keeps the reconcile surface when a filter excludes all reconciled rows', async () => {
    // hasReconciliation must derive from the UNFILTERED set: filter to
    // Assets-only (the reconciled row is not an asset) and the Status
    // column must survive.
    useTagStore.getState().setTags([
      scannedTag(1, { reconciled: true }),
      scannedTag(2, { type: 'asset', assetId: 1, assetIdentifier: 'A-1' }),
    ]);
    renderScreen();
    await waitFor(() => {
      expect(screen.getByText('Status')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('button', { name: /^Assets/ }));
    await waitFor(() => {
      expect(screen.getByText('Status')).toBeInTheDocument();
    });
  });

  it('filters to recognized assets when the Assets tile is clicked', async () => {
    useTagStore.getState().setTags([
      scannedTag(1, { type: 'asset', assetId: 1, assetIdentifier: 'A-1' }),
      scannedTag(2),
    ]);
    renderScreen();
    await waitFor(() => {
      expect(screen.getAllByText(/E2806894/).length).toBeGreaterThan(0);
    });
    fireEvent.click(screen.getByRole('button', { name: /^Assets/ }));
    await waitFor(() => {
      expect(screen.queryAllByText(/E28068940000000000000002/).length).toBe(0);
      expect(screen.getAllByText(/E28068940000000000000001/).length).toBeGreaterThan(0);
    });
    // Toggle off restores show-all
    fireEvent.click(screen.getByRole('button', { name: /^Assets/ }));
    await waitFor(() => {
      expect(screen.getAllByText(/E28068940000000000000002/).length).toBeGreaterThan(0);
    });
  });

  it('resets sort off reconciled when the list goes away', async () => {
    useTagStore.getState().setTags([scannedTag(1, { reconciled: true })]);
    useTagStore.getState().setSortConfig('reconciled', 'asc');
    renderScreen();
    await waitFor(() => {
      expect(screen.getByText('Status')).toBeInTheDocument();
    });
    useTagStore.getState().setTags([scannedTag(2)]);
    await waitFor(() => {
      expect(useTagStore.getState().sortColumn).toBe('timestamp');
      expect(useTagStore.getState().sortDirection).toBe('desc');
    });
  });

  it('persists the tile filter selection', async () => {
    useTagStore.getState().setTags([scannedTag(1, { type: 'asset', assetId: 1, assetIdentifier: 'A-1' })]);
    renderScreen();
    await waitFor(() => {
      expect(screen.getAllByText(/E2806894/).length).toBeGreaterThan(0);
    });
    fireEvent.click(screen.getByRole('button', { name: /^Assets/ }));
    await waitFor(() => {
      expect(JSON.parse(localStorage.getItem('inventory-status-filters')!)).toEqual(['Assets']);
    });
  });
});
