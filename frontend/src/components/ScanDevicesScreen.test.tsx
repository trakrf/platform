import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  render as rtlRender,
  screen,
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
  external_key: 'dock_reader_1',
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
    expect(screen.getByText('dock_reader_1')).toBeInTheDocument();
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
