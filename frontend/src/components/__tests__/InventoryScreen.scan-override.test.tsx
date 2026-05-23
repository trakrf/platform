import React, { type ReactNode } from 'react';
import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import InventoryScreen from '@/components/InventoryScreen';
import { useTagStore, useAuthStore } from '@/stores';
import type { TagInfo } from '@/stores/tagStore';
import type { Location } from '@/types/locations';

// TRA-819: drive manual selection through the dropdown, which requires the
// user to be authenticated AND a populated locations list. Mock both so the
// override behavior can be exercised end-to-end.
vi.mock('@/hooks/assets', () => ({
  useAssets: () => ({ assets: [], isLoading: false, error: null }),
}));

const mockLocations: Location[] = [
  {
    id: 1,
    name: 'Warehouse A',
    external_key: 'warehouse-a',
    description: '',
    parent_id: null,
    parent_external_key: null,
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    is_active: true,
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 2,
    name: 'Office B',
    external_key: 'office-b',
    description: '',
    parent_id: null,
    parent_external_key: null,
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    is_active: true,
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 3,
    name: 'Office C',
    external_key: 'office-c',
    description: '',
    parent_id: null,
    parent_external_key: null,
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    is_active: true,
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];

vi.mock('@/hooks/locations', () => ({
  useLocations: () => ({
    locations: mockLocations,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

const generateLocationTag = (
  locationId: number,
  rssi: number,
  locationName: string,
): TagInfo => ({
  epc: `LOC${locationId.toString().padStart(8, '0')}`,
  displayEpc: `LOC${locationId.toString().padStart(8, '0')}`,
  rssi,
  count: 1,
  timestamp: Date.now(),
  source: 'scan' as const,
  type: 'location' as const,
  locationId,
  locationName,
});

const renderWithQueryClient = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) =>
    React.createElement(QueryClientProvider, { client: queryClient }, children);
  return render(<InventoryScreen />, { wrapper: Wrapper });
};

describe('InventoryScreen — scan overrides manual (TRA-819)', () => {
  beforeEach(() => {
    useTagStore.getState().clearTags();
    useAuthStore.setState({
      isAuthenticated: true,
      token: 'test-token',
      user: { id: 1, email: 't@e.st' } as never,
    });
  });

  afterEach(() => {
    cleanup();
    useAuthStore.setState({ isAuthenticated: false, token: null, user: null });
  });

  it('a new scanned location tag overrides a prior manual selection', async () => {
    // Start with one location tag detected (Warehouse A)
    useTagStore.getState().setTags([generateLocationTag(1, -30, 'Warehouse A')]);

    renderWithQueryClient();

    await waitFor(() => {
      expect(screen.getByText('Warehouse A')).toBeInTheDocument();
    });

    // User manually picks Office C via the dropdown
    fireEvent.click(screen.getByRole('button', { name: /Change/i }));
    const officeCButton = await screen.findByRole('menuitem', { name: /Office C/ });
    fireEvent.click(officeCButton);

    await waitFor(() => {
      expect(screen.getByText('Office C')).toBeInTheDocument();
      expect(screen.getByText('manually selected')).toBeInTheDocument();
    });

    // User moves and scans a new location tag (Office B) with stronger
    // signal — most-recent-signal-wins should clear the manual pick.
    useTagStore.getState().setTags([
      generateLocationTag(1, -60, 'Warehouse A'),
      generateLocationTag(2, -25, 'Office B'),
    ]);

    await waitFor(() => {
      expect(screen.getByText('Office B')).toBeInTheDocument();
      expect(screen.getByText('via location tag (strongest signal)')).toBeInTheDocument();
    });
  });
});
