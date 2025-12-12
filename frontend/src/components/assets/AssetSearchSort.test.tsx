import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { AssetSearchSort } from './AssetSearchSort';
import { useAssetStore, useLocationStore } from '@/stores';

vi.mock('@/stores');
vi.mock('@/hooks/locations', () => ({
  useLocations: vi.fn(() => ({
    locations: [],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  })),
}));

describe('AssetSearchSort', () => {
  afterEach(() => {
    cleanup();
  });

  const mockSetSearchTerm = vi.fn();
  const mockSetSort = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    const mockAssetStore = {
      filters: { search: '', location_id: 'all' },
      setSearchTerm: mockSetSearchTerm,
      setFilters: vi.fn(),
      sort: { field: 'identifier', direction: 'asc' as const },
      setSort: mockSetSort,
      getFilteredAssets: vi.fn(() => [{ id: 1 }, { id: 2 }]),
      cache: {
        byId: new Map(),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set(),
        allIds: [],
        lastFetched: 0,
        ttl: 60 * 60 * 1000,
      },
    };
    const mockLocationStore = {
      cache: {
        byId: new Map(),
      },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockAssetStore));
    (useAssetStore as any).getState = vi.fn(() => mockAssetStore);
    (useLocationStore as any).mockImplementation((selector: any) => selector(mockLocationStore));
  });

  it('renders search input', () => {
    render(<AssetSearchSort />);

    expect(screen.getByPlaceholderText('Search assets...')).toBeInTheDocument();
  });

  it('renders sort controls', () => {
    render(<AssetSearchSort />);

    expect(screen.getByText('Sort by:')).toBeInTheDocument();
    expect(screen.getByRole('combobox')).toBeInTheDocument();
  });

  it('displays results count', () => {
    render(<AssetSearchSort />);

    expect(screen.getByText('2 results')).toBeInTheDocument();
  });

  it('calls setSearchTerm after debounce delay', async () => {
    render(<AssetSearchSort />);

    const searchInput = screen.getByPlaceholderText('Search assets...');
    fireEvent.change(searchInput, { target: { value: 'test' } });

    // Should not call immediately
    expect(mockSetSearchTerm).not.toHaveBeenCalled();

    // Should call after 300ms debounce
    await waitFor(
      () => {
        expect(mockSetSearchTerm).toHaveBeenCalledWith('test');
      },
      { timeout: 500 }
    );
  });

  it('shows clear button when search has value', () => {
    const mockStore = {
      filters: { search: 'test' },
      setSearchTerm: mockSetSearchTerm,
      sort: { field: 'identifier', direction: 'asc' as const },
      setSort: mockSetSort,
      getFilteredAssets: vi.fn(() => []),
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetSearchSort />);

    const clearButton = document.querySelector('button[class*="absolute"]');
    expect(clearButton).toBeInTheDocument();
  });

  it('clears search when clear button is clicked', () => {
    const mockStore = {
      filters: { search: 'test' },
      setSearchTerm: mockSetSearchTerm,
      sort: { field: 'identifier', direction: 'asc' as const },
      setSort: mockSetSort,
      getFilteredAssets: vi.fn(() => []),
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetSearchSort />);

    const clearButton = document.querySelector('button[class*="absolute"]');
    fireEvent.click(clearButton!);

    expect(mockSetSearchTerm).toHaveBeenCalledWith('');
  });

  it('calls setSort when sort field changes', () => {
    render(<AssetSearchSort />);

    const sortSelect = screen.getByRole('combobox');
    fireEvent.change(sortSelect, { target: { value: 'name' } });

    expect(mockSetSort).toHaveBeenCalledWith('name', 'asc');
  });

  it('toggles sort direction when direction button is clicked', () => {
    render(<AssetSearchSort />);

    const directionButton = document.querySelector('button[title*="Ascending"]');
    fireEvent.click(directionButton!);

    expect(mockSetSort).toHaveBeenCalledWith('identifier', 'desc');
  });

  it('displays singular "result" when count is 1', () => {
    const mockStore = {
      filters: { search: '' },
      setSearchTerm: mockSetSearchTerm,
      sort: { field: 'identifier', direction: 'asc' as const },
      setSort: mockSetSort,
      getFilteredAssets: vi.fn(() => [{ id: 1 }]),
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetSearchSort />);

    expect(screen.getByText('1 result')).toBeInTheDocument();
  });

  it('applies custom className', () => {
    const { container } = render(<AssetSearchSort className="custom-search-class" />);
    const searchDiv = container.firstChild as HTMLElement;

    expect(searchDiv.className).toContain('custom-search-class');
  });
});
