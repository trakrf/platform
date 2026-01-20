import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { LocationTreePanel } from './LocationTreePanel';
import { useLocationStore } from '@/stores/locations/locationStore';
import type { Location } from '@/types/locations';

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

describe('LocationTreePanel', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
  });

  afterEach(() => {
    cleanup();
  });

  it('should render root locations at top level', () => {
    const root1 = createMockLocation(1, { identifier: 'warehouse-a', name: 'Warehouse A' });
    const root2 = createMockLocation(2, { identifier: 'warehouse-b', name: 'Warehouse B' });
    useLocationStore.getState().setLocations([root1, root2]);

    render(<LocationTreePanel onSelect={vi.fn()} selectedId={null} />);

    expect(screen.getByText('warehouse-a')).toBeInTheDocument();
    expect(screen.getByText('warehouse-b')).toBeInTheDocument();
  });

  it('should render children indented under parents', () => {
    const root = createMockLocation(1, { identifier: 'root' });
    const child = createMockLocation(2, { identifier: 'child', parent_location_id: 1 });
    useLocationStore.getState().setLocations([root, child]);

    // Expand root to see child
    useLocationStore.getState().toggleNodeExpanded(1);

    render(<LocationTreePanel onSelect={vi.fn()} selectedId={null} />);

    expect(screen.getByText('root')).toBeInTheDocument();
    expect(screen.getByText('child')).toBeInTheDocument();
  });

  it('should call onSelect when location clicked', () => {
    const root = createMockLocation(1, { identifier: 'root' });
    useLocationStore.getState().setLocations([root]);

    const onSelect = vi.fn();
    render(<LocationTreePanel onSelect={onSelect} selectedId={null} />);

    const locationElement = screen.getByText('root').closest('[data-location-id]');
    fireEvent.click(locationElement!);

    expect(onSelect).toHaveBeenCalledWith(1);
  });

  it('should highlight selected location with blue background', () => {
    const root = createMockLocation(1, { identifier: 'root' });
    useLocationStore.getState().setLocations([root]);

    render(<LocationTreePanel onSelect={vi.fn()} selectedId={1} />);

    const locationElement = screen.getByText('root').closest('[data-location-id]');
    expect(locationElement).toHaveClass('bg-blue-100');
  });

  it('should toggle expansion on chevron click', () => {
    const root = createMockLocation(1, { identifier: 'root' });
    const child = createMockLocation(2, { identifier: 'child', parent_location_id: 1 });
    useLocationStore.getState().setLocations([root, child]);

    render(<LocationTreePanel onSelect={vi.fn()} selectedId={null} />);

    // Initially child should not be visible (collapsed by default)
    expect(screen.queryByText('child')).not.toBeInTheDocument();

    // Click expand button
    const expandButton = screen.getByLabelText('Expand');
    fireEvent.click(expandButton);

    // Now child should be visible
    expect(screen.getByText('child')).toBeInTheDocument();
  });

  it('should show chevron-down for expanded nodes', () => {
    const root = createMockLocation(1, { identifier: 'root' });
    const child = createMockLocation(2, { identifier: 'child', parent_location_id: 1 });
    useLocationStore.getState().setLocations([root, child]);
    useLocationStore.getState().toggleNodeExpanded(1);

    render(<LocationTreePanel onSelect={vi.fn()} selectedId={null} />);

    expect(screen.getByLabelText('Collapse')).toBeInTheDocument();
  });

  it('should show active/inactive status badge', () => {
    const active = createMockLocation(1, { identifier: 'active-loc', is_active: true });
    const inactive = createMockLocation(2, { identifier: 'inactive-loc', is_active: false });
    useLocationStore.getState().setLocations([active, inactive]);

    render(<LocationTreePanel onSelect={vi.fn()} selectedId={null} />);

    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.getByText('Inactive')).toBeInTheDocument();
  });

  it('should filter locations by search term', () => {
    const loc1 = createMockLocation(1, { identifier: 'warehouse', name: 'Main Warehouse' });
    const loc2 = createMockLocation(2, { identifier: 'office', name: 'Office Building' });
    useLocationStore.getState().setLocations([loc1, loc2]);

    render(<LocationTreePanel onSelect={vi.fn()} selectedId={null} searchTerm="warehouse" />);

    expect(screen.getByText('warehouse')).toBeInTheDocument();
    expect(screen.queryByText('office')).not.toBeInTheDocument();
  });

  it('should show empty state when no locations exist', () => {
    render(<LocationTreePanel onSelect={vi.fn()} selectedId={null} />);

    expect(screen.getByText('No locations found.')).toBeInTheDocument();
  });

  it('should show no matches state when search returns no results', () => {
    const loc = createMockLocation(1, { identifier: 'warehouse' });
    useLocationStore.getState().setLocations([loc]);

    render(<LocationTreePanel onSelect={vi.fn()} selectedId={null} searchTerm="nonexistent" />);

    expect(screen.getByText('No matching locations.')).toBeInTheDocument();
  });

  it('should show ancestors when filtering', () => {
    const root = createMockLocation(1, { identifier: 'root' });
    const child = createMockLocation(2, { identifier: 'child', parent_location_id: 1 });
    const grandchild = createMockLocation(3, { identifier: 'match-this', parent_location_id: 2 });
    useLocationStore.getState().setLocations([root, child, grandchild]);
    useLocationStore.getState().toggleNodeExpanded(1);
    useLocationStore.getState().toggleNodeExpanded(2);

    render(<LocationTreePanel onSelect={vi.fn()} selectedId={null} searchTerm="match" />);

    // Should show root and child as ancestors of matched grandchild
    expect(screen.getByText('root')).toBeInTheDocument();
    expect(screen.getByText('child')).toBeInTheDocument();
    expect(screen.getByText('match-this')).toBeInTheDocument();
  });

  describe('keyboard navigation', () => {
    beforeEach(() => {
      const root = createMockLocation(1, { identifier: 'root' });
      const child1 = createMockLocation(2, { identifier: 'child1', parent_location_id: 1 });
      const child2 = createMockLocation(3, { identifier: 'child2', parent_location_id: 1 });
      useLocationStore.getState().setLocations([root, child1, child2]);
      useLocationStore.getState().toggleNodeExpanded(1);
    });

    it('should select on Enter key', () => {
      const onSelect = vi.fn();
      render(<LocationTreePanel onSelect={onSelect} selectedId={1} />);

      const rootElement = screen.getByText('root').closest('[data-location-id]');
      rootElement?.focus();

      fireEvent.keyDown(rootElement!, { key: 'Enter' });

      expect(onSelect).toHaveBeenCalledWith(1);
    });

    it('should select on Space key', () => {
      const onSelect = vi.fn();
      render(<LocationTreePanel onSelect={onSelect} selectedId={1} />);

      const rootElement = screen.getByText('root').closest('[data-location-id]');
      rootElement?.focus();

      fireEvent.keyDown(rootElement!, { key: ' ' });

      expect(onSelect).toHaveBeenCalledWith(1);
    });
  });
});
