import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { ScanPointsPanel } from './ScanPointsPanel';
import { useScanPoints, useScanPointMutations } from '@/hooks/scandevices';
import { useLocations } from '@/hooks/locations/useLocations';
import type { ScanPoint } from '@/types/scandevices';
import type { Location } from '@/types/locations';

vi.mock('@/hooks/scandevices');
vi.mock('@/hooks/locations/useLocations');

const point = (over: Partial<ScanPoint>): ScanPoint => ({
  id: 1,
  org_id: 1,
  scan_device_id: 10,
  location_id: null,
  external_key: 'dock_1_port_1',
  name: 'Dock 1 Port 1',
  antenna_port: 1,
  description: '',
  metadata: {},
  valid_from: '2024-01-01T00:00:00Z',
  valid_to: null,
  is_active: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: null,
  deleted_at: null,
  ...over,
});

const location = (over: Partial<Location>): Location =>
  ({
    id: 100,
    external_key: 'zone_a',
    name: 'Zone A',
    ...over,
  }) as Location;

function mockPoints(points: ScanPoint[]) {
  (useScanPoints as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    scanPoints: points,
    isLoading: false,
    isRefetching: false,
    error: null,
    refetch: vi.fn(),
  });
  (useScanPointMutations as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
  });
}

describe('ScanPointsPanel location column', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (useLocations as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      locations: [location({ id: 100, name: 'Zone A', external_key: 'zone_a' })],
      isLoading: false,
    });
  });
  afterEach(() => cleanup());

  it('has a Location column header', () => {
    mockPoints([point({})]);
    render(<ScanPointsPanel deviceId={10} />);
    expect(screen.getByRole('columnheader', { name: 'Location' })).toBeInTheDocument();
  });

  it('shows the assigned location name for a point with a location_id', () => {
    mockPoints([point({ location_id: 100 })]);
    render(<ScanPointsPanel deviceId={10} />);
    expect(screen.getByText('Zone A')).toBeInTheDocument();
  });

  it('shows an em dash for a point with no location', () => {
    mockPoints([point({ location_id: null })]);
    const { container } = render(<ScanPointsPanel deviceId={10} />);
    // The location cell renders the placeholder when unassigned.
    expect(container.textContent).toContain('—');
  });
});
