import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { LocationBar } from '../LocationBar';
import type { Location } from '@/types/locations';

afterEach(cleanup);

function makeLocation(overrides: Partial<Location> & { id: number; name: string; path: string; depth: number }): Location {
  return {
    org_id: 1,
    identifier: `loc-${overrides.id}`,
    description: '',
    parent_location_id: null,
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
  makeLocation({ id: 1, name: 'Warehouse A', path: 'warehouse-a', depth: 0 }),
  makeLocation({ id: 2, name: 'Shelf B', path: 'warehouse-a/shelf-b', depth: 1 }),
  makeLocation({ id: 3, name: 'Office C', path: 'office-c', depth: 0 }),
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

  it('sorts locations by path in dropdown', async () => {
    // Provide locations in non-sorted order to verify sorting
    const unsortedLocations: Location[] = [
      makeLocation({ id: 3, name: 'Office C', path: 'office-c', depth: 0 }),
      makeLocation({ id: 1, name: 'Warehouse A', path: 'warehouse-a', depth: 0 }),
      makeLocation({ id: 2, name: 'Shelf B', path: 'warehouse-a/shelf-b', depth: 1 }),
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

    // Sorted by path: office-c, warehouse-a, warehouse-a/shelf-b
    expect(menuItems[0]).toHaveTextContent('Office C');
    expect(menuItems[1]).toHaveTextContent('Warehouse A');
    expect(menuItems[2]).toHaveTextContent('Shelf B');
  });
});
