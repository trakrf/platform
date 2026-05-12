import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { LocationBar } from '../LocationBar';
import type { Location } from '@/types/locations';

afterEach(cleanup);

function makeLocation(overrides: Partial<Location> & { id: number; name: string; external_key: string }): Location {
  return {
    description: '',
    parent_id: null,
    parent_external_key: null,
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    is_active: true,
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  };
}

const locations: Location[] = [
  makeLocation({ id: 1, name: 'Warehouse A', external_key: 'warehouse-a' }),
  makeLocation({ id: 2, name: 'Shelf B', external_key: 'shelf-b', parent_id: 1 }),
  makeLocation({ id: 3, name: 'Office C', external_key: 'office-c' }),
];

const defaultProps = {
  detectedLocation: null as { id: number; name: string } | null,
  detectionMethod: null as 'tag' | 'manual' | null,
  selectedLocationId: null as number | null,
  onLocationChange: vi.fn(),
  locations,
  isAuthenticated: true,
};

describe('LocationBar', () => {
  beforeEach(() => {
    defaultProps.onLocationChange.mockClear();
  });

  it('renders detected location name with "via location tag (strongest signal)" subtext', () => {
    render(
      <LocationBar
        {...defaultProps}
        detectedLocation={{ id: 1, name: 'Warehouse A' }}
        detectionMethod="tag"
      />,
    );

    expect(screen.getByText('Warehouse A')).toBeInTheDocument();
    expect(screen.getByText('via location tag (strongest signal)')).toBeInTheDocument();
  });

  it('renders "No location tag detected" when no detection', () => {
    render(<LocationBar {...defaultProps} />);

    expect(screen.getByText('No location tag detected')).toBeInTheDocument();
  });

  it('shows "manually selected" subtext for manual override', () => {
    render(
      <LocationBar
        {...defaultProps}
        detectedLocation={{ id: 1, name: 'Warehouse A' }}
        detectionMethod="manual"
        selectedLocationId={1}
      />,
    );

    expect(screen.getByText('manually selected')).toBeInTheDocument();
  });

  it('shows "Change" button when location detected, "Select" when not', () => {
    // First: with detected location => "Change"
    const { unmount } = render(
      <LocationBar
        {...defaultProps}
        detectedLocation={{ id: 1, name: 'Warehouse A' }}
        detectionMethod="tag"
      />,
    );

    expect(screen.getByRole('button', { name: /Change/i })).toBeInTheDocument();

    unmount();

    // Second: no location => "Select"
    render(<LocationBar {...defaultProps} />);

    expect(screen.getByRole('button', { name: /Select/i })).toBeInTheDocument();
  });

  it('hides dropdown for unauthenticated users', () => {
    render(
      <LocationBar
        {...defaultProps}
        isAuthenticated={false}
        detectedLocation={{ id: 1, name: 'Warehouse A' }}
        detectionMethod="tag"
      />,
    );

    expect(screen.getByText('Warehouse A')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Change/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Select/i })).not.toBeInTheDocument();
  });

  it('shows "Use detected" revert option when manual override differs from detected', async () => {
    render(
      <LocationBar
        {...defaultProps}
        detectedLocation={{ id: 1, name: 'Warehouse A' }}
        detectionMethod="manual"
        selectedLocationId={2}
      />,
    );

    // Open the dropdown menu
    fireEvent.click(screen.getByRole('button', { name: /Change/i }));

    // The revert option should appear
    expect(await screen.findByText(/Use detected: Warehouse A/)).toBeInTheDocument();
  });

  it('sorts locations by parent_id-derived tree order in dropdown', async () => {
    // TRA-684: tree_path is gone; depth-first order is derived client-side
    // from the parent_id chain. Provide locations in non-sorted order to
    // verify the walk + sort.
    const unsortedLocations: Location[] = [
      makeLocation({ id: 3, name: 'Office C', external_key: 'office-c' }),
      makeLocation({ id: 1, name: 'Warehouse A', external_key: 'warehouse-a' }),
      makeLocation({ id: 2, name: 'Shelf B', external_key: 'shelf-b', parent_id: 1 }),
    ];

    render(
      <LocationBar
        {...defaultProps}
        locations={unsortedLocations}
        isAuthenticated={true}
      />,
    );

    // Open the dropdown
    fireEvent.click(screen.getByRole('button', { name: /Select/i }));

    // Wait for menu items to appear
    const menuItems = await screen.findAllByRole('menuitem');

    // Depth-first by external_key: office-c (root), warehouse-a (root),
    // then warehouse-a → shelf-b (child).
    expect(menuItems[0]).toHaveTextContent('Office C');
    expect(menuItems[1]).toHaveTextContent('Warehouse A');
    expect(menuItems[2]).toHaveTextContent('Shelf B');
  });
});
