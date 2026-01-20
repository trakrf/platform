import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { LocationMobileView } from './LocationMobileView';
import { useLocationStore } from '@/stores/locations/locationStore';
import type { Location } from '@/types/locations';

// Mock the LocationExpandableCard component
vi.mock('./LocationExpandableCard', () => ({
  LocationExpandableCard: ({ location, searchTerm }: { location: Location; searchTerm: string }) => (
    <div data-testid="expandable-card" data-location-id={location.id} data-search={searchTerm}>
      Card: {location.identifier}
    </div>
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

describe('LocationMobileView', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
  });

  afterEach(() => {
    cleanup();
  });

  it('should show empty state when no locations exist', () => {
    render(
      <LocationMobileView
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('No locations yet')).toBeInTheDocument();
    expect(screen.getByText('Tap the + button to create your first location')).toBeInTheDocument();
  });

  it('should render root locations as expandable cards', () => {
    const root1 = createMockLocation(1, { identifier: 'warehouse-a' });
    const root2 = createMockLocation(2, { identifier: 'warehouse-b' });
    useLocationStore.getState().setLocations([root1, root2]);

    render(
      <LocationMobileView
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('Card: warehouse-a')).toBeInTheDocument();
    expect(screen.getByText('Card: warehouse-b')).toBeInTheDocument();
  });

  it('should have data-testid for mobile view', () => {
    const root = createMockLocation(1);
    useLocationStore.getState().setLocations([root]);

    render(
      <LocationMobileView
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByTestId('location-mobile-view')).toBeInTheDocument();
  });

  it('should pass searchTerm to expandable cards', () => {
    const root = createMockLocation(1, { identifier: 'warehouse' });
    useLocationStore.getState().setLocations([root]);

    render(
      <LocationMobileView
        searchTerm="warehouse"
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    const card = screen.getByTestId('expandable-card');
    expect(card).toHaveAttribute('data-search', 'warehouse');
  });

  it('should show no matches state when filtering returns no results', () => {
    const root = createMockLocation(1, { identifier: 'warehouse' });
    useLocationStore.getState().setLocations([root]);

    render(
      <LocationMobileView
        searchTerm="nonexistent"
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('No matching locations')).toBeInTheDocument();
    expect(screen.getByText('Try a different search term')).toBeInTheDocument();
  });

  it('should show matching locations when filtering', () => {
    const root1 = createMockLocation(1, { identifier: 'warehouse', name: 'Main Warehouse' });
    const root2 = createMockLocation(2, { identifier: 'office', name: 'Office Building' });
    useLocationStore.getState().setLocations([root1, root2]);

    render(
      <LocationMobileView
        searchTerm="warehouse"
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText('Card: warehouse')).toBeInTheDocument();
    expect(screen.queryByText('Card: office')).not.toBeInTheDocument();
  });

  it('should show root with matching descendant', () => {
    const root = createMockLocation(1, { identifier: 'root' });
    const child = createMockLocation(2, { identifier: 'matching-child', parent_location_id: 1 });
    useLocationStore.getState().setLocations([root, child]);

    render(
      <LocationMobileView
        searchTerm="matching"
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    // Root should be shown because it has a matching descendant
    expect(screen.getByText('Card: root')).toBeInTheDocument();
  });
});
