import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { AssetFilters } from './AssetFilters';
import { useAssetStore } from '@/stores';
import type { Asset } from '@/types/assets';

vi.mock('@/stores');

describe('AssetFilters', () => {
  afterEach(() => {
    cleanup();
  });

  const mockSetFilters = vi.fn();
  const mockAssets: Asset[] = [
    {
      id: 1,
      org_id: 1,
      identifier: 'DEV-001',
      name: 'Device 1',
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
    {
      id: 2,
      org_id: 1,
      identifier: 'PER-001',
      name: 'Person 1',
      type: 'person',
      description: '',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: false,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
    const mockByIdMap = new Map(mockAssets.map((asset) => [asset.id, asset]));
    const mockStore = {
      filters: { type: 'all', is_active: 'all', search: '' },
      setFilters: mockSetFilters,
      cache: { byId: mockByIdMap },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));
  });

  it('renders filter header', () => {
    render(<AssetFilters />);

    expect(screen.getByText('Filters')).toBeInTheDocument();
  });

  it('renders asset type checkboxes with counts', () => {
    render(<AssetFilters />);

    expect(screen.getByText('Asset Type')).toBeInTheDocument();
    expect(screen.getByText('Devices')).toBeInTheDocument();
    expect(screen.getByText('People')).toBeInTheDocument();

    // Check that counts are rendered (may appear multiple times)
    const counts = screen.getAllByText('(1)');
    expect(counts.length).toBeGreaterThan(0);
  });

  it('renders status radio buttons', () => {
    render(<AssetFilters />);

    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('All')).toBeInTheDocument();
    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.getByText('Inactive')).toBeInTheDocument();
  });

  it('calls setFilters when type checkbox is clicked', () => {
    render(<AssetFilters />);

    const deviceCheckbox = screen.getByLabelText(/Devices/);
    fireEvent.click(deviceCheckbox);

    expect(mockSetFilters).toHaveBeenCalledWith({ type: 'device' });
  });

  it('toggles type filter when clicking same type again', () => {
    const mockStore = {
      filters: { type: 'device', is_active: 'all', search: '' },
      setFilters: mockSetFilters,
      cache: { byId: new Map(mockAssets.map((asset) => [asset.id, asset])) },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetFilters />);

    const deviceCheckbox = screen.getByLabelText(/Devices/);
    fireEvent.click(deviceCheckbox);

    expect(mockSetFilters).toHaveBeenCalledWith({ type: 'all' });
  });

  it('calls setFilters when status radio is clicked', () => {
    render(<AssetFilters />);

    const activeRadio = screen.getByLabelText('Active');
    fireEvent.click(activeRadio);

    expect(mockSetFilters).toHaveBeenCalledWith({ is_active: true });
  });

  it('shows active filter count badge', () => {
    const mockStore = {
      filters: { type: 'device', is_active: true, search: 'test' },
      setFilters: mockSetFilters,
      cache: { byId: new Map(mockAssets.map((asset) => [asset.id, asset])) },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetFilters />);

    expect(screen.getByText('3')).toBeInTheDocument(); // Badge count
  });

  it('shows clear all button when filters are active', () => {
    const mockStore = {
      filters: { type: 'device', is_active: 'all', search: '' },
      setFilters: mockSetFilters,
      cache: { byId: new Map(mockAssets.map((asset) => [asset.id, asset])) },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetFilters />);

    expect(screen.getByText('Clear All')).toBeInTheDocument();
  });

  it('clears all filters when clear all button is clicked', () => {
    const mockStore = {
      filters: { type: 'device', is_active: true, search: 'test' },
      setFilters: mockSetFilters,
      cache: { byId: new Map(mockAssets.map((asset) => [asset.id, asset])) },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetFilters />);

    const clearButton = screen.getByText('Clear All');
    fireEvent.click(clearButton);

    expect(mockSetFilters).toHaveBeenCalledWith({ type: 'all', is_active: 'all', search: '' });
  });

  it('does not render when isOpen is false', () => {
    const { container } = render(<AssetFilters isOpen={false} />);

    expect(container.firstChild).toBeNull();
  });

  it('calls onToggle when close button is clicked', () => {
    const handleToggle = vi.fn();
    render(<AssetFilters onToggle={handleToggle} />);

    const closeButton = document.querySelector('button.md\\:hidden');
    if (closeButton) {
      fireEvent.click(closeButton);
      expect(handleToggle).toHaveBeenCalledTimes(1);
    }
  });

  it('applies custom className', () => {
    const { container } = render(<AssetFilters className="custom-filters-class" />);
    const filtersDiv = container.firstChild as HTMLElement;

    expect(filtersDiv.className).toContain('custom-filters-class');
  });
});
