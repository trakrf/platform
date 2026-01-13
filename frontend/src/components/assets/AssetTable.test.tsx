import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { AssetTable } from './AssetTable';
import { useAssetStore } from '@/stores';
import type { Asset } from '@/types/assets';

// Mock the asset store
vi.mock('@/stores');

describe('AssetTable', () => {
  afterEach(() => {
    cleanup();
  });

  const mockAssets: Asset[] = [
    {
      id: 1,
      org_id: 1,
      identifier: 'LAP-001',
      name: 'Engineering Laptop',
      type: 'device',
      description: 'Test laptop',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: { location: 'Building A' },
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
    },
    {
      id: 2,
      org_id: 1,
      identifier: 'LAP-002',
      name: 'Sales Laptop',
      type: 'device',
      description: 'Test laptop',
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
    const mockStore = {
      getFilteredAssets: vi.fn(() => mockAssets),
      sort: { field: 'identifier', direction: 'asc' as const },
      setSort: vi.fn(),
      cache: {
        byId: new Map(),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set(),
        allIds: [],
        lastFetched: 0,
        ttl: 60 * 60 * 1000,
      },
      filters: {
        type: 'all',
        is_active: 'all',
        search: '',
        location_id: 'all',
      },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));
    (useAssetStore as any).getState = vi.fn(() => mockStore);
  });

  it('renders desktop table with assets', () => {
    render(<AssetTable />);

    expect(screen.getByText('LAP-001')).toBeInTheDocument();
    expect(screen.getByText('Engineering Laptop')).toBeInTheDocument();
    expect(screen.getByText('LAP-002')).toBeInTheDocument();
    expect(screen.getByText('Sales Laptop')).toBeInTheDocument();
  });

  it('hides on mobile (has hidden md:block classes)', () => {
    const { container } = render(<AssetTable />);
    const tableWrapper = container.firstChild as HTMLElement;

    expect(tableWrapper.className).toContain('hidden');
    expect(tableWrapper.className).toContain('md:block');
  });

  it('renders sortable column headers', () => {
    render(<AssetTable />);

    expect(screen.getByText('Asset ID')).toBeInTheDocument();
    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('Status')).toBeInTheDocument();
  });

  it('calls setSort when clicking sortable header', () => {
    const mockSetSort = vi.fn();
    const mockStore = {
      getFilteredAssets: vi.fn(() => mockAssets),
      sort: { field: 'identifier', direction: 'asc' as const },
      setSort: mockSetSort,
      cache: {
        byId: new Map(),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set(),
        allIds: [],
        lastFetched: 0,
        ttl: 60 * 60 * 1000,
      },
      filters: {
        type: 'all',
        is_active: 'all',
        search: '',
        location_id: 'all',
      },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));
    (useAssetStore as any).getState = vi.fn(() => mockStore);

    render(<AssetTable />);

    const nameHeader = screen.getByText('Name');
    fireEvent.click(nameHeader);

    expect(mockSetSort).toHaveBeenCalledWith('name', 'asc');
  });

  it('toggles sort direction when clicking same field', () => {
    const mockSetSort = vi.fn();
    const mockStore = {
      getFilteredAssets: vi.fn(() => mockAssets),
      sort: { field: 'name', direction: 'asc' as const },
      setSort: mockSetSort,
      cache: {
        byId: new Map(),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set(),
        allIds: [],
        lastFetched: 0,
        ttl: 60 * 60 * 1000,
      },
      filters: {
        type: 'all',
        is_active: 'all',
        search: '',
        location_id: 'all',
      },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));
    (useAssetStore as any).getState = vi.fn(() => mockStore);

    render(<AssetTable />);

    const nameHeader = screen.getByText('Name');
    fireEvent.click(nameHeader);

    expect(mockSetSort).toHaveBeenCalledWith('name', 'desc');
  });

  it('displays loading skeletons when loading is true', () => {
    render(<AssetTable loading={true} />);

    const skeletons = document.querySelectorAll('.animate-pulse');
    expect(skeletons.length).toBeGreaterThan(0);

    // Assets should not be rendered
    expect(screen.queryByText('LAP-001')).not.toBeInTheDocument();
  });

  it('displays empty state when no assets', () => {
    const mockStore = {
      getFilteredAssets: vi.fn(() => []),
      sort: { field: 'identifier', direction: 'asc' as const },
      setSort: vi.fn(),
      cache: {
        byId: new Map(),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set(),
        allIds: [],
        lastFetched: 0,
        ttl: 60 * 60 * 1000,
      },
      filters: {
        type: 'all',
        is_active: 'all',
        search: '',
        location_id: 'all',
      },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));
    (useAssetStore as any).getState = vi.fn(() => mockStore);

    render(<AssetTable />);

    expect(screen.getByText('No Assets Found')).toBeInTheDocument();
  });

  it('calls onAssetClick when row is clicked', () => {
    const handleAssetClick = vi.fn();
    render(<AssetTable onAssetClick={handleAssetClick} />);

    const row = screen.getByText('LAP-001').closest('tr');
    fireEvent.click(row!);

    expect(handleAssetClick).toHaveBeenCalledWith(mockAssets[0]);
  });

  it('calls onEdit when edit button is clicked', () => {
    const handleEdit = vi.fn();
    render(<AssetTable onEdit={handleEdit} />);

    const editButtons = screen.getAllByLabelText(/Edit/);
    fireEvent.click(editButtons[0]);

    expect(handleEdit).toHaveBeenCalledWith(mockAssets[0]);
  });

  it('calls onDelete when delete button is clicked', () => {
    const handleDelete = vi.fn();
    render(<AssetTable onDelete={handleDelete} />);

    const deleteButtons = screen.getAllByLabelText(/Delete/);
    fireEvent.click(deleteButtons[0]);

    expect(handleDelete).toHaveBeenCalledWith(mockAssets[0]);
  });

  it('applies striping to alternating rows', () => {
    const { container } = render(<AssetTable />);
    const rows = container.querySelectorAll('tbody tr');

    expect(rows[0].className).toContain('bg-white');
    expect(rows[1].className).toContain('bg-gray-50');
  });

  it('applies custom className', () => {
    const { container } = render(<AssetTable className="custom-table-class" />);
    const tableWrapper = container.firstChild as HTMLElement;

    expect(tableWrapper.className).toContain('custom-table-class');
  });
});
