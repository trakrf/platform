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
import { OutputDeviceFormModal, OutputDeviceEditPanel } from './OutputDeviceFormModal';
import { outputDevicesApi } from '@/lib/api/outputdevices';
import type { OutputDevice } from '@/types/outputdevices';

vi.mock('@/lib/api/outputdevices');
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn().mockResolvedValue(undefined),
}));
// The form fetches locations for its dropdown; stub the hook so the test
// doesn't hit the network.
vi.mock('@/hooks/locations', () => ({
  useLocations: () => ({ locations: [{ id: 5, name: 'Dock 1' }], isLoading: false }),
}));
// The GPO reader picker fetches scan devices; stub the hook so the test doesn't
// hit the network. Includes one BLE gateway to prove the picker filters it out.
vi.mock('@/hooks/scandevices', () => ({
  useScanDevices: () => ({
    scanDevices: [
      { id: 42, name: 'cs463-212', type: 'csl_cs463' },
      { id: 43, name: 'cs463-213', type: 'csl_cs463' },
      { id: 99, name: 'ble-gateway-1', type: 'gl_s10' },
    ],
    isLoading: false,
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

describe('OutputDeviceFormModal', () => {
  const mockOnClose = vi.fn();

  const mockDevice: OutputDevice = {
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
    (outputDevicesApi.create as any).mockResolvedValue({ data: { data: mockDevice } });
    (outputDevicesApi.update as any).mockResolvedValue({ data: { data: mockDevice } });
    (outputDevicesApi.test as any).mockResolvedValue({ data: { status: 'ok' } });
    (outputDevicesApi.reset as any).mockResolvedValue({ data: { status: 'ok' } });
  });

  afterEach(() => {
    cleanup();
  });

  it('does not render when isOpen is false', () => {
    const { container } = render(
      <OutputDeviceFormModal isOpen={false} mode="create" onClose={mockOnClose} />
    );
    expect(container.firstChild).toBeNull();
  });

  it('renders create modal when isOpen is true', () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    expect(screen.getByText('Create New Output Device')).toBeInTheDocument();
  });

  it('renders edit modal with device name', () => {
    render(
      <OutputDeviceFormModal isOpen={true} mode="edit" device={mockDevice} onClose={mockOnClose} />
    );
    expect(screen.getByText(`Edit Output Device: ${mockDevice.name}`)).toBeInTheDocument();
  });

  it('closes modal when close button is clicked', () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
    fireEvent.click(screen.getByLabelText('Close modal'));
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it('submits create with filled fields, calls API and fires onClose', async () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
    fireEvent.change(screen.getByLabelText(/Base URL/), {
      target: { value: 'http://192.168.50.66' },
    });

    fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

    await waitFor(() => {
      expect(outputDevicesApi.create).toHaveBeenCalledTimes(1);
    });
    expect(outputDevicesApi.create).toHaveBeenCalledWith(
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
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
    fireEvent.change(screen.getByLabelText(/Transport/), { target: { value: 'mqtt' } });
    fireEvent.change(screen.getByLabelText(/Command Topic/), {
      target: { value: 'trakrf.id/dock-strobe' },
    });

    fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

    await waitFor(() => {
      expect(outputDevicesApi.create).toHaveBeenCalledTimes(1);
    });
    expect(outputDevicesApi.create).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Dock Strobe',
        transport: 'mqtt',
        command_topic: 'trakrf.id/dock-strobe',
      })
    );
    // TRA-928: base_url is not applicable to mqtt and must not be submitted (an
    // empty base_url is rejected by the backend's url validation envelope).
    const createPayload = (outputDevicesApi.create as any).mock.calls[0][0];
    expect(createPayload).not.toHaveProperty('base_url');
  });

  it('edit: switching an HTTP device to MQTT submits no base_url (TRA-928)', async () => {
    render(
      <OutputDeviceFormModal isOpen={true} mode="edit" device={mockDevice} onClose={mockOnClose} />
    );

    fireEvent.change(screen.getByLabelText(/Transport/), { target: { value: 'mqtt' } });
    fireEvent.change(screen.getByLabelText(/Command Topic/), {
      target: { value: 'trakrf.id/dock-strobe' },
    });

    fireEvent.click(screen.getByRole('button', { name: /Update Output Device/i }));

    await waitFor(() => {
      expect(outputDevicesApi.update).toHaveBeenCalledTimes(1);
    });
    const [, updatePayload] = (outputDevicesApi.update as any).mock.calls[0];
    expect(updatePayload).not.toHaveProperty('base_url');
    expect(updatePayload).toMatchObject({
      transport: 'mqtt',
      command_topic: 'trakrf.id/dock-strobe',
    });
  });

  it('requires a command topic for MQTT transport', async () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
    fireEvent.change(screen.getByLabelText(/Transport/), { target: { value: 'mqtt' } });
    fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

    await waitFor(() => {
      expect(screen.getByText('Command topic is required for MQTT transport')).toBeInTheDocument();
    });
    expect(outputDevicesApi.create).not.toHaveBeenCalled();
  });

  it('rejects a missing base URL', async () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
    fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

    await waitFor(() => {
      expect(screen.getByText('Base URL is required')).toBeInTheDocument();
    });
    expect(outputDevicesApi.create).not.toHaveBeenCalled();
    expect(mockOnClose).not.toHaveBeenCalled();
  });

  it('does not show test-fire / reset controls in create mode', () => {
    render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    expect(screen.queryByRole('button', { name: /Test-fire/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Reset/i })).not.toBeInTheDocument();
  });

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

  describe('inline variant (TRA-938 row expander)', () => {
    it('renders the edit form without modal chrome', () => {
      render(
        <OutputDeviceFormModal
          isOpen={true}
          mode="edit"
          device={mockDevice}
          variant="inline"
          onClose={mockOnClose}
        />
      );

      expect(
        screen.getByRole('button', { name: /Update Output Device/i })
      ).toBeInTheDocument();
      expect(screen.queryByLabelText('Close modal')).not.toBeInTheDocument();
      expect(
        screen.queryByText(`Edit Output Device: ${mockDevice.name}`)
      ).not.toBeInTheDocument();
    });

    it('exposes inline test-fire and reset controls that drive the device', async () => {
      render(
        <OutputDeviceFormModal
          isOpen={true}
          mode="edit"
          device={mockDevice}
          variant="inline"
          onClose={mockOnClose}
        />
      );

      fireEvent.click(
        screen.getByRole('button', { name: `Test-fire output device ${mockDevice.name}` })
      );
      await waitFor(() => {
        expect(outputDevicesApi.test).toHaveBeenCalledWith(mockDevice.id);
      });

      fireEvent.click(
        screen.getByRole('button', { name: `Reset output device ${mockDevice.name}` })
      );
      await waitFor(() => {
        expect(outputDevicesApi.reset).toHaveBeenCalledWith(mockDevice.id);
      });
    });
  });

  describe('de-duplication in edit mode (TRA-940)', () => {
    it('create mode renders name, switch ID, location, and active fields', () => {
      render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);
      expect(screen.getByLabelText(/^name/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/switch id/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/^location/i)).toBeInTheDocument();
      expect(screen.getByLabelText('Active')).toBeInTheDocument();
    });

    it('edit mode hides name, switch ID, location, and active (now inline in the row)', () => {
      render(
        <OutputDeviceFormModal isOpen={true} mode="edit" device={mockDevice} onClose={mockOnClose} />
      );
      expect(screen.queryByLabelText(/^name/i)).not.toBeInTheDocument();
      expect(screen.queryByLabelText(/switch id/i)).not.toBeInTheDocument();
      expect(screen.queryByLabelText(/^location/i)).not.toBeInTheDocument();
      expect(screen.queryByLabelText('Active')).not.toBeInTheDocument();
    });

    it('edit mode keeps the cascade + rule fields', () => {
      render(
        <OutputDeviceFormModal isOpen={true} mode="edit" device={mockDevice} onClose={mockOnClose} />
      );
      expect(screen.getByLabelText(/transport/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/base url/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/^mode/i)).toBeInTheDocument();
    });

    it('edit submit omits name, switch_id, location_id, and is_active', async () => {
      render(
        <OutputDeviceFormModal isOpen={true} mode="edit" device={mockDevice} onClose={mockOnClose} />
      );
      fireEvent.click(screen.getByRole('button', { name: /Update Output Device/i }));
      await waitFor(() => expect(outputDevicesApi.update).toHaveBeenCalledTimes(1));
      const [, payload] = (outputDevicesApi.update as any).mock.calls[0];
      expect(payload).not.toHaveProperty('name');
      expect(payload).not.toHaveProperty('switch_id');
      expect(payload).not.toHaveProperty('location_id');
      expect(payload).not.toHaveProperty('is_active');
      expect(payload).toMatchObject({ type: 'shelly_gen4', transport: 'http' });
    });
  });

  describe('CS463 GPO (TRA-1028)', () => {
    it('locks transport to mqtt when the CS463 GPO type is selected', () => {
      render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

      fireEvent.change(screen.getByLabelText(/Type/), { target: { value: 'csl_cs463_gpo' } });

      const transport = screen.getByLabelText(/Transport/) as HTMLSelectElement;
      expect(transport.value).toBe('mqtt');
      // An http GPO is not a thing; lock it rather than validating after the fact.
      expect(transport).toBeDisabled();
    });

    it('rejects a GPO port outside 1-4', async () => {
      render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

      fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Egress GPO' } });
      fireEvent.change(screen.getByLabelText(/Type/), { target: { value: 'csl_cs463_gpo' } });
      fireEvent.change(screen.getByLabelText(/Reader/), { target: { value: '42' } });
      fireEvent.change(screen.getByLabelText(/GPO port/), { target: { value: '0' } });
      fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

      expect(await screen.findByText(/GPO port must be between 1 and 4/)).toBeInTheDocument();
      expect(outputDevicesApi.create).not.toHaveBeenCalled();
    });

    it('accepts a GPO port of 4', async () => {
      render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

      fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Egress GPO' } });
      fireEvent.change(screen.getByLabelText(/Type/), { target: { value: 'csl_cs463_gpo' } });
      fireEvent.change(screen.getByLabelText(/Reader/), { target: { value: '42' } });
      fireEvent.change(screen.getByLabelText(/GPO port/), { target: { value: '4' } });
      fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

      await waitFor(() => expect(outputDevicesApi.create).toHaveBeenCalled());
      expect((outputDevicesApi.create as any).mock.calls[0][0]).toMatchObject({
        type: 'csl_cs463_gpo',
        transport: 'mqtt',
        scan_device_id: 42,
        switch_id: 4,
      });
      const createPayload = (outputDevicesApi.create as any).mock.calls[0][0];
      expect(createPayload).not.toHaveProperty('command_topic');
    });

    it('shows a reader picker instead of the command-topic input for GPO', () => {
      render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

      fireEvent.change(screen.getByLabelText(/Type/), { target: { value: 'csl_cs463_gpo' } });

      expect(screen.queryByLabelText(/Command Topic/)).not.toBeInTheDocument();
      const readerSelect = screen.getByLabelText(/Reader/) as HTMLSelectElement;
      expect(readerSelect).toBeInTheDocument();
      // Populated from the mocked useScanDevices, filtered to reader-type
      // devices only — the BLE gateway must not appear as an option.
      expect(screen.getByRole('option', { name: 'cs463-212' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'cs463-213' })).toBeInTheDocument();
      expect(screen.queryByRole('option', { name: 'ble-gateway-1' })).not.toBeInTheDocument();
    });

    it('defaults the GPO port to 1 (not 0) when the type is selected', () => {
      render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

      fireEvent.change(screen.getByLabelText(/Type/), { target: { value: 'csl_cs463_gpo' } });

      const switchId = screen.getByLabelText(/GPO port/) as HTMLInputElement;
      expect(switchId.value).toBe('1');
      expect(switchId).toHaveAttribute('min', '1');
    });

    it('requires a reader selection for GPO', async () => {
      render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

      fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Egress GPO' } });
      fireEvent.change(screen.getByLabelText(/Type/), { target: { value: 'csl_cs463_gpo' } });
      fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

      expect(await screen.findByText(/Reader is required/)).toBeInTheDocument();
      expect(outputDevicesApi.create).not.toHaveBeenCalled();
    });

    it('preselects the current scan_device_id on the edit path', () => {
      const gpoDevice: OutputDevice = {
        ...mockDevice,
        type: 'csl_cs463_gpo',
        transport: 'mqtt',
        command_topic: null,
        scan_device_id: 43,
        switch_id: 2,
      };
      render(
        <OutputDeviceFormModal isOpen={true} mode="edit" device={gpoDevice} onClose={mockOnClose} />
      );

      const readerSelect = screen.getByLabelText(/Reader/) as HTMLSelectElement;
      expect(readerSelect.value).toBe('43');
    });

    it('still accepts switch_id 0 for a shelly device', async () => {
      // Regression guard: 0 is a valid relay channel on a single-relay Gen4.
      render(<OutputDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

      fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dock Strobe' } });
      fireEvent.change(screen.getByLabelText(/Base URL/), {
        target: { value: 'http://192.168.50.66' },
      });
      fireEvent.change(screen.getByLabelText(/Switch ID/), { target: { value: '0' } });
      fireEvent.click(screen.getByRole('button', { name: /Create Output Device/i }));

      await waitFor(() => expect(outputDevicesApi.create).toHaveBeenCalled());
    });
  });

  describe('OutputDeviceEditPanel', () => {
    it('renders the inline edit surface with test-fire / reset for the device', () => {
      render(<OutputDeviceEditPanel device={mockDevice} onClose={mockOnClose} />);

      expect(
        screen.getByRole('button', { name: /Update Output Device/i })
      ).toBeInTheDocument();
      expect(
        screen.getByRole('button', { name: `Test-fire output device ${mockDevice.name}` })
      ).toBeInTheDocument();
      expect(screen.queryByLabelText('Close modal')).not.toBeInTheDocument();
    });
  });
});
