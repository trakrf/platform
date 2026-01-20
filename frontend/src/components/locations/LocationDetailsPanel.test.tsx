import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import { LocationDetailsPanel } from './LocationDetailsPanel';
import { useLocationStore } from '@/stores/locations/locationStore';
import type { Location } from '@/types/locations';

// Mock the TagIdentifierList component
vi.mock('@/components/assets', () => ({
  TagIdentifierList: () => <div data-testid="tag-identifier-list">Tag Identifiers</div>,
}));

// Mock the LocationBreadcrumb component
vi.mock('./LocationBreadcrumb', () => ({
  LocationBreadcrumb: ({ locationId }: { locationId: number }) => (
    <div data-testid="location-breadcrumb">Breadcrumb for {locationId}</div>
  ),
}));

const createMockLocation = (id: number, overrides = {}): Location => ({
  id,
  org_id: 1,
  identifier: `loc_${id}`,
  name: `Location ${id}`,
  description: '',
  parent_location_id: null,
  path: `loc_${id}`,
  depth: 1,
  valid_from: '2024-01-01',
  valid_to: null,
  is_active: true,
  metadata: {},
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides,
});

describe('LocationDetailsPanel', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
  });

  afterEach(() => {
    cleanup();
  });

  it('should show "Select a location" when none selected', () => {
    render(
      <LocationDetailsPanel
        locationId={null}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('Select a location')).toBeInTheDocument();
  });

  it('should show stats summary in empty state', () => {
    const root = createMockLocation(1);
    useLocationStore.getState().setLocations([root]);

    render(
      <LocationDetailsPanel
        locationId={null}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText(/1 root location/)).toBeInTheDocument();
    expect(screen.getByText(/1 total/)).toBeInTheDocument();
  });

  it('should show location identifier and name', () => {
    const location = createMockLocation(1, { identifier: 'warehouse-a', name: 'Main Warehouse' });
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    // Identifier and name appear in both header and info section
    expect(screen.getAllByText('warehouse-a').length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText('Main Warehouse').length).toBeGreaterThanOrEqual(1);
  });

  it('should show description when provided', () => {
    const location = createMockLocation(1, { description: 'A large warehouse for storage' });
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('A large warehouse for storage')).toBeInTheDocument();
  });

  it('should show active status badge', () => {
    const location = createMockLocation(1, { is_active: true });
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('Active')).toBeInTheDocument();
  });

  it('should show inactive status badge', () => {
    const location = createMockLocation(1, { is_active: false });
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('Inactive')).toBeInTheDocument();
  });

  it('should show breadcrumb path', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByTestId('location-breadcrumb')).toBeInTheDocument();
  });

  it('should show hierarchy info with children count', () => {
    const root = createMockLocation(1);
    const child1 = createMockLocation(2, { parent_location_id: 1 });
    const child2 = createMockLocation(3, { parent_location_id: 1 });
    useLocationStore.getState().setLocations([root, child1, child2]);

    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('Direct Children')).toBeInTheDocument();
    // Look for the children count within the hierarchy section
    const directChildrenSection = screen.getByText('Direct Children').closest('div');
    expect(directChildrenSection?.parentElement).toHaveTextContent('2');
  });

  it('should show descendants count', () => {
    const root = createMockLocation(1);
    const child = createMockLocation(2, { parent_location_id: 1 });
    const grandchild = createMockLocation(3, { parent_location_id: 2 });
    useLocationStore.getState().setLocations([root, child, grandchild]);

    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('Total Descendants')).toBeInTheDocument();
  });

  it('should list direct children with click navigation', () => {
    const root = createMockLocation(1);
    const child = createMockLocation(2, { identifier: 'child-loc', parent_location_id: 1 });
    useLocationStore.getState().setLocations([root, child]);

    const onChildClick = vi.fn();
    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
        onChildClick={onChildClick}
      />
    );

    expect(screen.getByText('child-loc')).toBeInTheDocument();
  });

  it('should call onChildClick when child is clicked', () => {
    const root = createMockLocation(1);
    const child = createMockLocation(2, { identifier: 'child-loc', parent_location_id: 1 });
    useLocationStore.getState().setLocations([root, child]);

    const onChildClick = vi.fn();
    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
        onChildClick={onChildClick}
      />
    );

    fireEvent.click(screen.getByText('child-loc'));
    expect(onChildClick).toHaveBeenCalledWith(2);
  });

  it('should show Edit, Move, Delete buttons', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Move' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
  });

  it('should call onEdit when Edit clicked', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);

    const onEdit = vi.fn();
    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={onEdit}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    expect(onEdit).toHaveBeenCalledWith(1);
  });

  it('should call onMove when Move clicked', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);

    const onMove = vi.fn();
    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={onMove}
        onDelete={vi.fn()}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Move' }));
    expect(onMove).toHaveBeenCalledWith(1);
  });

  it('should call onDelete when Delete clicked', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);

    const onDelete = vi.fn();
    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={onDelete}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    expect(onDelete).toHaveBeenCalledWith(1);
  });

  it('should show Root Location type for root locations', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationDetailsPanel
        locationId={1}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('Root Location')).toBeInTheDocument();
  });

  it('should show Subsidiary Location type for child locations', () => {
    const root = createMockLocation(1);
    const child = createMockLocation(2, { parent_location_id: 1 });
    useLocationStore.getState().setLocations([root, child]);

    render(
      <LocationDetailsPanel
        locationId={2}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('Subsidiary Location')).toBeInTheDocument();
  });
});
