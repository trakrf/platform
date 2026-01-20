import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { LocationSplitPane } from './LocationSplitPane';
import { useLocationStore } from '@/stores/locations/locationStore';
import type { Location } from '@/types/locations';

// Mock react-split-pane
vi.mock('react-split-pane', () => ({
  SplitPane: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="split-pane">{children}</div>
  ),
  Pane: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="pane">{children}</div>
  ),
}));

// Mock the child components
vi.mock('./LocationTreePanel', () => ({
  LocationTreePanel: ({ selectedId, searchTerm }: { selectedId: number | null; searchTerm: string }) => (
    <div data-testid="tree-panel">
      Tree Panel - Selected: {selectedId ?? 'none'}, Search: {searchTerm || 'none'}
    </div>
  ),
}));

vi.mock('./LocationDetailsPanel', () => ({
  LocationDetailsPanel: ({ locationId }: { locationId: number | null }) => (
    <div data-testid="details-panel">
      Details Panel - Location: {locationId ?? 'none'}
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

describe('LocationSplitPane', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
  });

  afterEach(() => {
    cleanup();
  });

  it('should render split pane container', () => {
    render(
      <LocationSplitPane
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByTestId('split-pane')).toBeInTheDocument();
  });

  it('should render tree panel on left', () => {
    render(
      <LocationSplitPane
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByTestId('tree-panel')).toBeInTheDocument();
  });

  it('should render details panel on right', () => {
    render(
      <LocationSplitPane
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByTestId('details-panel')).toBeInTheDocument();
  });

  it('should render two panes', () => {
    render(
      <LocationSplitPane
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    const panes = screen.getAllByTestId('pane');
    expect(panes).toHaveLength(2);
  });

  it('should pass searchTerm to tree panel', () => {
    render(
      <LocationSplitPane
        searchTerm="test-search"
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    expect(screen.getByText(/Search: test-search/)).toBeInTheDocument();
  });

  it('should pass selectedLocationId to both panels', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);
    useLocationStore.getState().setSelectedLocation(1);

    render(
      <LocationSplitPane
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    const treePanel = screen.getByTestId('tree-panel');
    const detailsPanel = screen.getByTestId('details-panel');

    expect(treePanel).toHaveTextContent('Selected: 1');
    expect(detailsPanel).toHaveTextContent('Location: 1');
  });

  it('should show no selection when no location selected', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationSplitPane
        onEdit={vi.fn()}
        onMove={vi.fn()}
        onDelete={vi.fn()}
      />
    );

    const treePanel = screen.getByTestId('tree-panel');
    const detailsPanel = screen.getByTestId('details-panel');

    expect(treePanel).toHaveTextContent('Selected: none');
    expect(detailsPanel).toHaveTextContent('Location: none');
  });
});
