import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  render as rtlRender,
  screen,
  fireEvent,
  waitFor,
  cleanup,
  type RenderOptions,
} from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactElement, ReactNode } from 'react';
import ScanDevicesScreen from './ScanDevicesScreen';
import { useScanDevices, useScanDeviceMutations } from '@/hooks/scandevices';
import { useAuthStore } from '@/stores/authStore';
import type { ScanDevice } from '@/types/scandevices';

vi.mock('@/hooks/scandevices');
// Stub the device-edit surfaces so the screen test isolates the row-expander
// navigation (which row is open) rather than the hook-backed edit form (TRA-938).
vi.mock('@/components/scandevices', () => ({
  ScanDeviceFormModal: () => null,
  ScanDeviceEditPanel: ({ device }: { device: ScanDevice }) => (
    <div data-testid="edit-panel">editing:{device.name}</div>
  ),
}));

const wrapper = ({ children }: { children: ReactNode }) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
};
const render = (ui: ReactElement, options?: Omit<RenderOptions, 'wrapper'>) =>
  rtlRender(ui, { wrapper, ...options });

const device = (over: Partial<ScanDevice>): ScanDevice => ({
  id: 1,
  org_id: 1,
  name: 'Dock Reader 1',
  type: 'csl_cs463',
  transport: 'mqtt',
  publish_topic: 'trakrf.id/dock_reader_1/reads',
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

describe('ScanDevicesScreen flat list', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({ isAuthenticated: true });
    (useScanDevices as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      scanDevices: [device({})],
      isLoading: false,
    });
    (useScanDeviceMutations as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      delete: vi.fn(),
    });
  });
  afterEach(() => cleanup());

  it('lists the device', () => {
    render(<ScanDevicesScreen />);
    expect(screen.getByText('Dock Reader 1')).toBeInTheDocument();
  });

  it('has no scan-point tree expansion control', () => {
    render(<ScanDevicesScreen />);
    expect(screen.queryByLabelText(/Expand scan points/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Collapse scan points/i)).not.toBeInTheDocument();
  });

  it('does not render the inline scan points panel', () => {
    render(<ScanDevicesScreen />);
    // ScanPointsPanel's "Add Scan Point" affordance must not appear in the list.
    expect(screen.queryByRole('button', { name: /Add Scan Point/i })).not.toBeInTheDocument();
  });
});

describe('ScanDevicesScreen row expander (TRA-938)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({ isAuthenticated: true });
    (useScanDevices as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      scanDevices: [
        device({ id: 1, name: 'Dock Reader 1' }),
        device({ id: 2, name: 'Dock Reader 2' }),
      ],
      isLoading: false,
    });
    (useScanDeviceMutations as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      delete: vi.fn(),
    });
  });
  afterEach(() => cleanup());

  it('reveals the inline edit panel when a row toggle is clicked', () => {
    render(<ScanDevicesScreen />);
    expect(screen.queryByTestId('edit-panel')).not.toBeInTheDocument();

    fireEvent.click(screen.getByLabelText('Edit scan device Dock Reader 1'));

    expect(screen.getByTestId('edit-panel')).toHaveTextContent('editing:Dock Reader 1');
  });

  it('collapses the row when its toggle is clicked again', () => {
    render(<ScanDevicesScreen />);
    const toggle = screen.getByLabelText('Edit scan device Dock Reader 1');

    fireEvent.click(toggle);
    expect(screen.getByTestId('edit-panel')).toBeInTheDocument();

    fireEvent.click(toggle);
    expect(screen.queryByTestId('edit-panel')).not.toBeInTheDocument();
  });

  it('keeps only one row open at a time (single-open accordion)', () => {
    render(<ScanDevicesScreen />);

    fireEvent.click(screen.getByLabelText('Edit scan device Dock Reader 1'));
    expect(screen.getByTestId('edit-panel')).toHaveTextContent('editing:Dock Reader 1');

    fireEvent.click(screen.getByLabelText('Edit scan device Dock Reader 2'));
    const panels = screen.getAllByTestId('edit-panel');
    expect(panels).toHaveLength(1);
    expect(panels[0]).toHaveTextContent('editing:Dock Reader 2');
  });

  it('marks the toggle expanded state for assistive tech', () => {
    render(<ScanDevicesScreen />);
    const toggle = screen.getByLabelText('Edit scan device Dock Reader 1');
    expect(toggle).toHaveAttribute('aria-expanded', 'false');

    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
  });

  it('collapses the open row when that device is deleted', async () => {
    render(<ScanDevicesScreen />);
    fireEvent.click(screen.getByLabelText('Edit scan device Dock Reader 1'));
    expect(screen.getByTestId('edit-panel')).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText('Delete scan device Dock Reader 1'));
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(screen.queryByTestId('edit-panel')).not.toBeInTheDocument();
    });
  });
});
