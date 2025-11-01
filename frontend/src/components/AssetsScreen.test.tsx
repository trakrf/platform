import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import AssetsScreen from './AssetsScreen';
import { useAssets, useAssetMutations } from '@/hooks/assets';
import { useAssetStore } from '@/stores';
import type { Asset } from '@/types/assets';

vi.mock('@/hooks/assets');
vi.mock('@/stores');

describe('AssetsScreen', () => {
  afterEach(() => {
    cleanup();
  });

  const mockAssets: Asset[] = [
    {
      id: 1,
      org_id: 1,
      identifier: 'LAP-001',
      name: 'Laptop 1',
      type: 'device',
      description: '',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();

    (useAssets as any).mockReturnValue({
      data: { data: mockAssets },
      isLoading: false,
    });

    (useAssetMutations as any).mockReturnValue({
      deleteAsset: { mutateAsync: vi.fn() },
    });

    const mockByIdMap = new Map(mockAssets.map((asset) => [asset.id, asset]));
    const mockStore = {
      cache: { byId: mockByIdMap },
      getFilteredAssets: vi.fn(() => mockAssets),
      filters: { type: 'all', is_active: 'all', search: '' },
      setFilters: vi.fn(),
      sort: { field: 'identifier', direction: 'asc' as const },
      setSort: vi.fn(),
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));
  });

  it('renders stats dashboard', () => {
    render(<AssetsScreen />);

    expect(screen.getByText('Total Assets')).toBeInTheDocument();
  });

  it('renders asset table on desktop', () => {
    render(<AssetsScreen />);

    // Asset appears in both desktop table and mobile cards
    const assetIdentifiers = screen.getAllByText('LAP-001');
    expect(assetIdentifiers.length).toBeGreaterThan(0);
  });

  it('renders floating action button', () => {
    render(<AssetsScreen />);

    const fab = screen.getByLabelText('Create new asset');
    expect(fab).toBeInTheDocument();
  });

  it('opens create modal when FAB is clicked', () => {
    render(<AssetsScreen />);

    const fab = screen.getByLabelText('Create new asset');
    fireEvent.click(fab);

    expect(screen.getByText('Create New Asset')).toBeInTheDocument();
  });

  it('shows empty state when no assets', () => {
    const mockStore = {
      cache: { byId: new Map() },
      getFilteredAssets: vi.fn(() => []),
      filters: { type: 'all', is_active: 'all', search: '' },
      setFilters: vi.fn(),
      sort: { field: 'identifier', direction: 'asc' as const },
      setSort: vi.fn(),
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetsScreen />);

    expect(screen.getByText('No assets yet')).toBeInTheDocument();
  });

  it('shows no results when filters are active but no matches', () => {
    const mockStore = {
      cache: { byId: new Map() },
      getFilteredAssets: vi.fn(() => []),
      filters: { type: 'device', is_active: 'all', search: '' },
      setFilters: vi.fn(),
      sort: { field: 'identifier', direction: 'asc' as const },
      setSort: vi.fn(),
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetsScreen />);

    expect(screen.getByText('No Results Found')).toBeInTheDocument();
  });

  it('renders search and sort controls', () => {
    render(<AssetsScreen />);

    expect(screen.getByPlaceholderText('Search assets...')).toBeInTheDocument();
    expect(screen.getByText('Sort by:')).toBeInTheDocument();
  });
});
