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
import { AlarmDeviceFormModal } from './AlarmDeviceFormModal';
import { alarmDevicesApi } from '@/lib/api/alarmdevices';
import type { AlarmDevice } from '@/types/alarmdevices';

vi.mock('@/lib/api/alarmdevices');
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn().mockResolvedValue(undefined),
}));
// The form fetches locations for its dropdown; stub the hook so the test
// doesn't hit the network.
vi.mock('@/hooks/locations', () => ({
  useLocations: () => ({ locations: [{ id: 5, name: 'Dock 1' }], isLoading: false }),
}));

const wrapper = ({ children }: { children: ReactNode }) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
};
const render = (ui: ReactElement, options?: Omit<RenderOptions, 'wrapper'>) =>
  rtlRender(ui, { wrapper, ...options });

describe('AlarmDeviceFormModal', () => {
  const mockOnClose = vi.fn();

  const mockDevice: AlarmDevice = {
    id: 1,
    org_id: 1,
    name: 'Dock Strobe',
    type: 'shelly_gen4',
    transport: 'http',
    base_url: 'http://192.168.50.66',
    command_topic: null,
    switch_id: 0,
    location_id: null,
    is_active: true,
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    deleted_at: null,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    (alarmDevicesApi.create as any).mockResolvedValue({ data: { data: mockDevice } });
    (alarmDevicesApi.update as any).mockResolvedValue({ data: { data: mockDevice } });
  });

  afterEach(() => {
    cleanup();
  });

  it('does not render when isOpen is false', () => {
    const { container } = render(
      <AlarmDeviceFormModal isOpen={false} mode="create" onClose={mockOnClose} />
    );
    expect(container.firstChild).toBeNull();
  });

  it('renders create modal when isOpen is true', () => {
    render(<AlarmDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    expect(screen.getByText('Create New Alarm Device')).toBeInTheDocument();
  });

  it('renders edit modal with device name', () => {
    render(
      <AlarmDeviceFormModal isOpen={true} mode="edit" device={mockDevice} onClose={mockOnClose} />
    );
    expect(screen.getByText(`Edit Alarm Device: ${mockDevice.name}`)).toBeInTheDocument();
  });

  it('closes modal when close button is clicked', () => {
    render(<AlarmDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    fireEvent.click(screen.getByLabelText('Close modal'));
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it('submits create with filled fields, calls API and fires onClose', async () => {
    render(<AlarmDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
    fireEvent.change(screen.getByLabelText(/Base URL/), {
      target: { value: 'http://192.168.50.66' },
    });

    fireEvent.click(screen.getByRole('button', { name: /Create Alarm Device/i }));

    await waitFor(() => {
      expect(alarmDevicesApi.create).toHaveBeenCalledTimes(1);
    });
    expect(alarmDevicesApi.create).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Dock Strobe',
        base_url: 'http://192.168.50.66',
        type: 'shelly_gen4',
        transport: 'http',
        switch_id: 0,
        location_id: null,
      })
    );
    await waitFor(() => {
      expect(mockOnClose).toHaveBeenCalledTimes(1);
    });
  });

  it('submits an MQTT device with command_topic (no base_url)', async () => {
    render(<AlarmDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
    fireEvent.change(screen.getByLabelText(/Transport/), { target: { value: 'mqtt' } });
    fireEvent.change(screen.getByLabelText(/Command Topic/), {
      target: { value: 'trakrf.id/dock-strobe' },
    });

    fireEvent.click(screen.getByRole('button', { name: /Create Alarm Device/i }));

    await waitFor(() => {
      expect(alarmDevicesApi.create).toHaveBeenCalledTimes(1);
    });
    expect(alarmDevicesApi.create).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Dock Strobe',
        transport: 'mqtt',
        command_topic: 'trakrf.id/dock-strobe',
      })
    );
  });

  it('requires a command topic for MQTT transport', async () => {
    render(<AlarmDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
    fireEvent.change(screen.getByLabelText(/Transport/), { target: { value: 'mqtt' } });
    fireEvent.click(screen.getByRole('button', { name: /Create Alarm Device/i }));

    await waitFor(() => {
      expect(screen.getByText('Command topic is required for MQTT transport')).toBeInTheDocument();
    });
    expect(alarmDevicesApi.create).not.toHaveBeenCalled();
  });

  it('rejects a missing base URL', async () => {
    render(<AlarmDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
    fireEvent.click(screen.getByRole('button', { name: /Create Alarm Device/i }));

    await waitFor(() => {
      expect(screen.getByText('Base URL is required')).toBeInTheDocument();
    });
    expect(alarmDevicesApi.create).not.toHaveBeenCalled();
    expect(mockOnClose).not.toHaveBeenCalled();
  });
});
