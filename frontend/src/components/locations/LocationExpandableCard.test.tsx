import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import { LocationExpandableCard } from './LocationExpandableCard';
import { useLocationStore } from '@/stores/locations/locationStore';
import type { Location } from '@/types/locations';

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

describe('LocationExpandableCard', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
  });

  afterEach(() => {
    cleanup();
  });

  it('should show identifier and name when collapsed', () => {
    const location = createMockLocation(1, { identifier: 'warehouse-a', name: 'Main Warehouse' });
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationExpandableCard
        location={location}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('warehouse-a')).toBeInTheDocument();
    expect(screen.getByText('Main Warehouse')).toBeInTheDocument();
  });

  it('should show active status badge', () => {
    const location = createMockLocation(1, { is_active: true });
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationExpandableCard
        location={location}
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
      <LocationExpandableCard
        location={location}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('Inactive')).toBeInTheDocument();
  });

  it('should expand on header click', () => {
    const location = createMockLocation(1, { description: 'A warehouse description' });
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationExpandableCard
        location={location}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    // Initially collapsed - description not visible
    expect(screen.queryByText('A warehouse description')).not.toBeInTheDocument();

    // Click to expand
    const header = screen.getByRole('button');
    fireEvent.click(header);

    // Now description should be visible
    expect(screen.getByText('A warehouse description')).toBeInTheDocument();
  });

  it('should show action buttons when expanded', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);
    // Expand the card
    useLocationStore.getState().toggleCardExpanded(1);

    render(
      <LocationExpandableCard
        location={location}
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
    useLocationStore.getState().toggleCardExpanded(1);

    const onEdit = vi.fn();
    render(
      <LocationExpandableCard
        location={location}
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
    useLocationStore.getState().toggleCardExpanded(1);

    const onMove = vi.fn();
    render(
      <LocationExpandableCard
        location={location}
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
    useLocationStore.getState().toggleCardExpanded(1);

    const onDelete = vi.fn();
    render(
      <LocationExpandableCard
        location={location}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={onDelete}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    expect(onDelete).toHaveBeenCalledWith(1);
  });

  it('should show children when expanded', () => {
    const root = createMockLocation(1, { identifier: 'root' });
    const child = createMockLocation(2, { identifier: 'child', parent_location_id: 1 });
    useLocationStore.getState().setLocations([root, child]);
    useLocationStore.getState().toggleCardExpanded(1);

    render(
      <LocationExpandableCard
        location={root}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    // Should show children section
    expect(screen.getByText('Children (1)')).toBeInTheDocument();
    expect(screen.getByText('child')).toBeInTheDocument();
  });

  it('should NOT show Type or Children info grid', () => {
    const root = createMockLocation(1);
    const child = createMockLocation(2, { parent_location_id: 1 });
    useLocationStore.getState().setLocations([root, child]);
    useLocationStore.getState().toggleCardExpanded(1);

    render(
      <LocationExpandableCard
        location={root}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.queryByText('Root Location')).not.toBeInTheDocument();
    expect(screen.queryByText('Subsidiary')).not.toBeInTheDocument();
    expect(screen.queryByText(/direct \/ .* total/i)).not.toBeInTheDocument();
  });

  it('should render Add button when onAddChild provided', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);
    useLocationStore.getState().toggleCardExpanded(1);

    const onAddChild = vi.fn();
    render(
      <LocationExpandableCard
        location={location}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
        onAddChild={onAddChild}
      />
    );

    expect(screen.getByRole('button', { name: 'Add' })).toBeInTheDocument();
  });

  it('should call onAddChild with location.id when Add clicked', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);
    useLocationStore.getState().toggleCardExpanded(1);

    const onAddChild = vi.fn();
    render(
      <LocationExpandableCard
        location={location}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
        onAddChild={onAddChild}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    expect(onAddChild).toHaveBeenCalledWith(1);
  });

  it('should NOT render Add button when onAddChild not provided', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);
    useLocationStore.getState().toggleCardExpanded(1);

    render(
      <LocationExpandableCard
        location={location}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.queryByRole('button', { name: 'Add' })).not.toBeInTheDocument();
  });

  it('should collapse when expanded and header clicked again', () => {
    const location = createMockLocation(1, { description: 'Test description' });
    useLocationStore.getState().setLocations([location]);
    useLocationStore.getState().toggleCardExpanded(1);

    render(
      <LocationExpandableCard
        location={location}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    // Verify expanded
    expect(screen.getByText('Test description')).toBeInTheDocument();

    // Click to collapse
    const header = screen.getByRole('button', { expanded: true });
    fireEvent.click(header);

    // Description should no longer be visible
    expect(screen.queryByText('Test description')).not.toBeInTheDocument();
  });

  it('should filter location by search term', () => {
    const location = createMockLocation(1, { identifier: 'warehouse' });
    useLocationStore.getState().setLocations([location]);

    const { rerender } = render(
      <LocationExpandableCard
        location={location}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
        searchTerm="warehouse"
      />
    );

    // Should be visible - matches search
    expect(screen.getByText('warehouse')).toBeInTheDocument();

    // Rerender with non-matching search
    rerender(
      <LocationExpandableCard
        location={location}
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
        searchTerm="nonexistent"
      />
    );

    // Should not be visible
    expect(screen.queryByText('warehouse')).not.toBeInTheDocument();
  });
});
