import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  render as rtlRender,
  screen,
  fireEvent,
  cleanup,
  waitFor,
  type RenderOptions,
} from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactElement, ReactNode } from 'react';
import { OutputDeviceFormModal } from './OutputDeviceFormModal';
import { outputDevicesApi } from '@/lib/api/outputdevices';

vi.mock('@/lib/api/outputdevices');
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn().mockResolvedValue(undefined),
}));
vi.mock('@/hooks/locations', () => ({
  useLocations: () => ({ locations: [], isLoading: false }),
}));
vi.mock('@/hooks/scandevices', () => ({
  useScanDevices: () => ({ scanDevices: [], isLoading: false }),
}));
const geofence = vi.hoisted(() => ({ orgAutoOff: null as number | null }));
vi.mock('@/hooks/orgs/useGeofenceDefaults', () => ({
  useGeofenceDefaults: () => ({
    defaults: { auto_off_seconds: geofence.orgAutoOff },
    system_defaults: { rssi_threshold: -65, age_out_seconds: 60, auto_off_seconds: 0, mode: 'egress' },
  }),
}));

const wrapper = ({ children }: { children: ReactNode }) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
};
const render = (ui: ReactElement, options?: Omit<RenderOptions, 'wrapper'>) =>
  rtlRender(ui, { wrapper, ...options });

const mockOnClose = vi.fn();
// An egress create needs an off edge (TRA-1273); tests about other fields give it one.
const setAutoOff = (value = '5') =>
  fireEvent.change(screen.getByLabelText(/Auto-off/i), { target: { value } });

beforeEach(() => {
  vi.clearAllMocks();
  geofence.orgAutoOff = null;
  (outputDevicesApi.create as any).mockResolvedValue({ data: { data: { id: 1 } } });
});
afterEach(() => cleanup());

describe('rule config fields (TRA-943/935)', () => {
  it('renders the rule-config fields', () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    expect(screen.getByLabelText(/Mode/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Age-out/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Auto-off/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/RSSI/i)).toBeInTheDocument();
  });

  it('disables auto-off when mode is presence', () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    fireEvent.change(screen.getByLabelText(/Mode/i), { target: { value: 'presence' } });
    expect(screen.getByLabelText(/Auto-off/i)).toBeDisabled();
  });

  it('submits metadata with the rule fields', async () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
    fireEvent.change(screen.getByLabelText(/Base URL/), {
      target: { value: 'http://192.168.50.66' },
    });
    fireEvent.change(screen.getByLabelText(/Age-out/i), { target: { value: '30' } });
    fireEvent.change(screen.getByLabelText(/RSSI/i), { target: { value: '-60' } });

    setAutoOff();
    fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

    await waitFor(() => {
      expect(outputDevicesApi.create).toHaveBeenCalledTimes(1);
    });
    expect(outputDevicesApi.create).toHaveBeenCalledWith(
      expect.objectContaining({
        metadata: expect.objectContaining({
          mode: 'egress',
          age_out_seconds: 30,
          rssi_threshold: -60,
        }),
      })
    );
  });

  it('omits auto_off_seconds from metadata in presence mode', async () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Light' } });
    fireEvent.change(screen.getByLabelText(/Base URL/), {
      target: { value: 'http://192.168.50.66' },
    });
    fireEvent.change(screen.getByLabelText(/Mode/i), { target: { value: 'presence' } });

    fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

    await waitFor(() => {
      expect(outputDevicesApi.create).toHaveBeenCalledTimes(1);
    });
    const payload = (outputDevicesApi.create as any).mock.calls[0][0];
    expect(payload.metadata.mode).toBe('presence');
    expect(payload.metadata).not.toHaveProperty('auto_off_seconds');
  });
});

// TRA-1273: egress only ever sends ON; the device-side auto-off is its only
// OFF edge. A blank Auto-off with no org default used to save an output that
// latched on forever.
describe('egress auto-off is required (TRA-1273)', () => {
  const fillShelly = () => {
    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
    fireEvent.change(screen.getByLabelText(/Base URL/), {
      target: { value: 'http://192.168.50.66' },
    });
  };
  const submit = () =>
    fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

  it('blocks a blank auto-off when the org default supplies none', async () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    fillShelly();
    submit();
    expect(await screen.findByText(/Required for egress/i)).toBeInTheDocument();
    expect(outputDevicesApi.create).not.toHaveBeenCalled();
  });

  it('allows a blank auto-off when the org default supplies one, and says so', async () => {
    geofence.orgAutoOff = 30;
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    expect(screen.getByLabelText(/Auto-off/i)).toHaveAttribute('placeholder', 'Org default (30)');
    fillShelly();
    submit();
    await waitFor(() => {
      expect(outputDevicesApi.create).toHaveBeenCalledTimes(1);
    });
    const payload = (outputDevicesApi.create as any).mock.calls[0][0];
    expect(payload.metadata).not.toHaveProperty('auto_off_seconds');
  });

  it.each(['0', '61'])('refuses %s seconds', async (value) => {
    geofence.orgAutoOff = 30;
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    fillShelly();
    setAutoOff(value);
    submit();
    expect(await screen.findByText(/between 1 and 60 seconds/i)).toBeInTheDocument();
    expect(outputDevicesApi.create).not.toHaveBeenCalled();
  });

  it('submits 60, the ceiling', async () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    fillShelly();
    setAutoOff('60');
    submit();
    await waitFor(() => {
      expect(outputDevicesApi.create).toHaveBeenCalledTimes(1);
    });
    expect((outputDevicesApi.create as any).mock.calls[0][0].metadata.auto_off_seconds).toBe(60);
  });

  it('does not require auto-off in presence mode', async () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    fillShelly();
    fireEvent.change(screen.getByLabelText(/Mode/i), { target: { value: 'presence' } });
    submit();
    await waitFor(() => {
      expect(outputDevicesApi.create).toHaveBeenCalledTimes(1);
    });
  });
});
