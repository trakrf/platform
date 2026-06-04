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
import { ScanDeviceFormModal } from './ScanDeviceFormModal';
import { scanDevicesApi } from '@/lib/api/scandevices';
import type { ScanDevice } from '@/types/scandevices';

vi.mock('@/lib/api/scandevices');
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn().mockResolvedValue(undefined),
}));

const wrapper = ({ children }: { children: ReactNode }) => {
  // Fresh client per render so cache state doesn't leak across tests.
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
};
const render = (ui: ReactElement, options?: Omit<RenderOptions, 'wrapper'>) =>
  rtlRender(ui, { wrapper, ...options });

describe('ScanDeviceFormModal', () => {
  const mockOnClose = vi.fn();

  const mockDevice: ScanDevice = {
    id: 1,
    org_id: 1,
    external_key: 'dock_reader_1',
    name: 'Dock Reader 1',
    type: 'csl_cs463',
    transport: 'mqtt',
    publish_topic: 'trakrf.id/dock_reader_1/reads',
    serial_number: null,
    model: null,
    description: 'Test device',
    metadata: {},
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    deleted_at: null,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    (scanDevicesApi.create as any).mockResolvedValue({ data: { data: mockDevice } });
    (scanDevicesApi.update as any).mockResolvedValue({ data: { data: mockDevice } });
  });

  afterEach(() => {
    cleanup();
  });

  it('does not render when isOpen is false', () => {
    const { container } = render(
      <ScanDeviceFormModal isOpen={false} mode="create" onClose={mockOnClose} />
    );

    expect(container.firstChild).toBeNull();
  });

  it('renders create modal when isOpen is true', () => {
    render(<ScanDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    expect(screen.getByText('Create New Scan Device')).toBeInTheDocument();
  });

  it('renders edit modal with device external key', () => {
    render(
      <ScanDeviceFormModal isOpen={true} mode="edit" device={mockDevice} onClose={mockOnClose} />
    );

    expect(screen.getByText(`Edit Scan Device: ${mockDevice.external_key}`)).toBeInTheDocument();
  });

  it('closes modal when close button is clicked', () => {
    render(<ScanDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.click(screen.getByLabelText('Close modal'));

    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it('submits create with filled fields, calls API and fires onClose', async () => {
    render(<ScanDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.change(screen.getByLabelText(/External Key/), {
      target: { value: 'dock_reader_1' },
    });
    fireEvent.change(screen.getByLabelText(/Name/), {
      target: { value: 'Dock Reader 1' },
    });
    fireEvent.change(screen.getByLabelText(/Type/), {
      target: { value: 'gl_s10' },
    });

    fireEvent.click(screen.getByRole('button', { name: /Create Scan Device/i }));

    await waitFor(() => {
      expect(scanDevicesApi.create).toHaveBeenCalledTimes(1);
    });

    expect(scanDevicesApi.create).toHaveBeenCalledWith(
      expect.objectContaining({
        external_key: 'dock_reader_1',
        name: 'Dock Reader 1',
        type: 'gl_s10',
      })
    );

    await waitFor(() => {
      expect(mockOnClose).toHaveBeenCalledTimes(1);
    });
  });

  it('does not call create API when required fields are missing', async () => {
    render(<ScanDeviceFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    fireEvent.click(screen.getByRole('button', { name: /Create Scan Device/i }));

    await waitFor(() => {
      expect(screen.getByText('External key is required')).toBeInTheDocument();
    });
    expect(scanDevicesApi.create).not.toHaveBeenCalled();
    expect(mockOnClose).not.toHaveBeenCalled();
  });
});
