import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { SinglePointLocationField } from './SinglePointLocationField';
import { useScanPoints, useScanPointMutations } from '@/hooks/scandevices';
import { useLocations } from '@/hooks/locations/useLocations';
import type { ScanDevice, ScanPoint } from '@/types/scandevices';
import type { Location } from '@/types/locations';

vi.mock('@/hooks/scandevices');
vi.mock('@/hooks/locations/useLocations');
vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

const device = (over: Partial<ScanDevice> = {}): ScanDevice => ({
  id: 10,
  org_id: 1,
  external_key: 'gateway_1',
  name: 'Gateway 1',
  type: 'gl_s10',
  transport: 'mqtt',
  publish_topic: null,
  serial_number: null,
  model: null,
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

const point = (over: Partial<ScanPoint>): ScanPoint => ({
  id: 1,
  org_id: 1,
  scan_device_id: 10,
  location_id: null,
  external_key: 'gateway_1-point-1',
  name: 'Gateway 1',
  antenna_port: null,
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

const location = (id: number, name: string): Location =>
  ({ id, external_key: name.toLowerCase().replace(/\s/g, '_'), name }) as Location;

const create = vi.fn();
const update = vi.fn();

function setup(points: ScanPoint[]) {
  (useScanPoints as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    scanPoints: points,
    isLoading: false,
  });
  (useScanPointMutations as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    create,
    update,
  });
  (useLocations as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    locations: [location(100, 'Zone A'), location(200, 'Zone B')],
    isLoading: false,
  });
}

describe('SinglePointLocationField', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    create.mockResolvedValue(point({ id: 5 }));
    update.mockResolvedValue(point({ id: 1 }));
  });
  afterEach(() => cleanup());

  it('preselects the existing scan point location', () => {
    setup([point({ id: 1, location_id: 200 })]);
    render(<SinglePointLocationField device={device()} />);
    const select = screen.getByLabelText(/Location/) as HTMLSelectElement;
    expect(select.value).toBe('200');
  });

  it('updates the existing scan point when a location is chosen and saved', async () => {
    setup([point({ id: 1, location_id: null })]);
    render(<SinglePointLocationField device={device()} />);

    fireEvent.change(screen.getByLabelText(/Location/), { target: { value: '100' } });
    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
    expect(update).toHaveBeenCalledWith({ id: 1, updates: { location_id: 100 } });
    expect(create).not.toHaveBeenCalled();
  });

  it('creates scan point 1 when none exists, writing location onto the point', async () => {
    setup([]);
    render(<SinglePointLocationField device={device()} />);

    fireEvent.change(screen.getByLabelText(/Location/), { target: { value: '200' } });
    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({
        external_key: 'gateway_1-point-1',
        name: 'Gateway 1',
        location_id: 200,
      })
    );
    expect(update).not.toHaveBeenCalled();
  });

  it('clears the location by saving — None — onto the existing point', async () => {
    setup([point({ id: 1, location_id: 100 })]);
    render(<SinglePointLocationField device={device()} />);

    fireEvent.change(screen.getByLabelText(/Location/), { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
    expect(update).toHaveBeenCalledWith({ id: 1, updates: { location_id: null } });
  });
});
