import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import type { MapPresence } from '@/types/locationMaps';

const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  getImageBlob: vi.fn(),
  putImage: vi.fn(),
  putPins: vi.fn(),
  remove: vi.fn(),
  presence: vi.fn(),
}));

vi.mock('@/lib/api/locationMaps', async (orig) => ({
  ...(await orig<typeof import('@/lib/api/locationMaps')>()),
  locationMapsApi: api,
}));

const role = vi.hoisted(() => ({ current: 'viewer' as string }));
vi.mock('@/stores', () => ({
  useOrgStore: (sel: (s: { currentRole: string }) => unknown) => sel({ currentRole: role.current }),
}));

vi.mock('@/hooks/locations/useLocations', () => ({
  useLocations: () => ({
    locations: [
      { id: 1, name: 'Warehouse', parent_id: null, external_key: 'WH' },
      { id: 3, name: 'Office', parent_id: 1, external_key: 'OFF' },
      { id: 4, name: 'Dock', parent_id: 1, external_key: 'DOCK' },
    ],
  }),
}));

const presence: MapPresence = {
  map_location_id: 1,
  generated_at: new Date().toISOString(),
  zones: [
    {
      location_id: 3,
      name: 'Office',
      x_pct: 20,
      y_pct: 30,
      occupants: [
        { asset_id: 10, name: 'Alice', external_key: 'B1', is_person: true, location_id: 3, location_name: 'Office', last_seen: new Date().toISOString() },
        { asset_id: 11, name: 'Laptop cart', external_key: 'A1', is_person: false, location_id: 3, location_name: 'Office', last_seen: new Date().toISOString() },
      ],
    },
    { location_id: 4, name: 'Dock', x_pct: 70, y_pct: 60, occupants: [] },
  ],
  unplaced: [],
};

import { LocationMapTab } from './LocationMapTab';

beforeEach(() => {
  vi.clearAllMocks();
  role.current = 'viewer';
  globalThis.URL.createObjectURL = vi.fn(() => 'blob:map');
  globalThis.URL.revokeObjectURL = vi.fn();
  api.list.mockResolvedValue({ data: { data: [{ location_id: 1, location_name: 'Warehouse', pin_count: 2, updated_at: '' }] } });
  api.get.mockResolvedValue({
    data: {
      data: {
        location_id: 1, location_name: 'Warehouse', content_type: 'image/png', byte_size: 10, sha256: 'abc', updated_at: '',
        pins: [{ location_id: 3, x_pct: 20, y_pct: 30 }, { location_id: 4, x_pct: 70, y_pct: 60 }],
      },
    },
  });
  api.getImageBlob.mockResolvedValue({ data: new Blob(['x'], { type: 'image/png' }) });
  api.presence.mockResolvedValue({ data: { data: presence } });
});

describe('LocationMapTab', () => {
  it('opens the first map and draws live counts at each pinned zone', async () => {
    render(<LocationMapTab />);
    await waitFor(() => expect(screen.getByTestId('location-map-pin-3-people').textContent).toBe('1'));
    expect(screen.getByTestId('location-map-pin-3-assets').textContent).toBe('1');
    expect(screen.getByTestId('location-map-pin-4-people').textContent).toBe('0');
    expect(api.presence).toHaveBeenCalledWith(1, 3600);
    expect(screen.getByTestId('location-map-occupant-10').textContent).toContain('Alice');
    expect(screen.queryByTestId('location-map-edit')).toBeNull();
  });

  it('filters to people only', async () => {
    render(<LocationMapTab />);
    await waitFor(() => screen.getByTestId('location-map-occupant-11'));
    fireEvent.change(screen.getByLabelText('Show'), { target: { value: 'people' } });
    expect(screen.queryByTestId('location-map-occupant-11')).toBeNull();
    expect(screen.getByTestId('location-map-pin-3-assets').textContent).toBe('0');
  });

  it('lets an admin place a pin and save', async () => {
    role.current = 'admin';
    api.putPins.mockResolvedValue({ data: { data: { location_id: 1, pins: [] } } });
    render(<LocationMapTab />);
    fireEvent.click(await screen.findByTestId('location-map-edit'));
    fireEvent.change(screen.getByTestId('location-map-place-select'), { target: { value: '4' } });
    const img = screen.getByTestId('location-map-image');
    img.getBoundingClientRect = () => ({ left: 0, top: 0, width: 200, height: 100 }) as DOMRect;
    fireEvent.click(img, { clientX: 50, clientY: 50 });
    fireEvent.click(screen.getByTestId('location-map-save'));
    await waitFor(() =>
      expect(api.putPins).toHaveBeenCalledWith(1, [
        { location_id: 3, x_pct: 20, y_pct: 30 },
        { location_id: 4, x_pct: 25, y_pct: 50 },
      ]),
    );
  });

  it('with no maps yet, lets an admin pick a location and upload from the empty state', async () => {
    role.current = 'admin';
    api.list.mockResolvedValue({ data: { data: [] } });
    api.putImage.mockResolvedValue({
      data: { data: { location_id: 1, location_name: 'Warehouse', content_type: 'image/png', byte_size: 1, sha256: 'x', updated_at: '', pins: [] } },
    });
    render(<LocationMapTab />);
    fireEvent.change(await screen.findByTestId('location-map-new-empty'), { target: { value: '1' } });
    expect(screen.getByTestId('location-map-upload-empty')).toBeTruthy();
    const file = new File(['\x89PNG'], 'plan.png', { type: 'image/png' });
    fireEvent.change(screen.getByTestId('location-map-file'), { target: { files: [file] } });
    await waitFor(() => expect(api.putImage).toHaveBeenCalledWith(1, file));
  });

  it('shows a viewer no upload controls', async () => {
    api.list.mockResolvedValue({ data: { data: [] } });
    render(<LocationMapTab />);
    await screen.findByText('No maps have been set up for this organization.');
    expect(screen.queryByTestId('location-map-new-empty')).toBeNull();
    expect(screen.queryByTestId('location-map-file')).toBeNull();
  });
});
