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
import OutputDevicesScreen from './OutputDevicesScreen';
import { useOutputDevices, useOutputDeviceMutations } from '@/hooks/outputdevices';
import { useAuthStore } from '@/stores/authStore';
import type { OutputDevice } from '@/types/outputdevices';

vi.mock('@/hooks/outputdevices');
vi.mock('@/hooks/locations', () => ({
  useLocations: () => ({ locations: [{ id: 5, name: 'Dock 1' }], isLoading: false }),
}));
// Stub the device-edit surface so the screen test isolates the row-expander
// navigation (which row is open), not the hook-backed edit form (TRA-938).
vi.mock('@/components/outputdevices', () => ({
  OutputDeviceFormModal: () => null,
  OutputDeviceEditPanel: ({ device }: { device: OutputDevice }) => (
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

const device = (over: Partial<OutputDevice>): OutputDevice => ({
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
  updated_at: null,
  deleted_at: null,
  ...over,
});

describe('OutputDevicesScreen', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({ isAuthenticated: true });
    (useOutputDevices as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      outputDevices: [device({})],
      isLoading: false,
    });
    (useOutputDeviceMutations as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      delete: vi.fn(),
      test: vi.fn(),
      reset: vi.fn(),
    });
  });
  afterEach(() => cleanup());

  it('lists the device', () => {
    render(<OutputDevicesScreen />);
    expect(screen.getByText('Dock Strobe')).toBeInTheDocument();
  });

  it('does not surface test-fire / reset on the collapsed row (they moved into the panel)', () => {
    render(<OutputDevicesScreen />);
    expect(
      screen.queryByLabelText('Test-fire output device Dock Strobe')
    ).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Reset output device Dock Strobe')).not.toBeInTheDocument();
  });
});

describe('OutputDevicesScreen row expander (TRA-938)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({ isAuthenticated: true });
    (useOutputDevices as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      outputDevices: [device({ id: 1, name: 'Dock Strobe' }), device({ id: 2, name: 'Gate Horn' })],
      isLoading: false,
    });
    (useOutputDeviceMutations as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      delete: vi.fn(),
      test: vi.fn(),
      reset: vi.fn(),
    });
  });
  afterEach(() => cleanup());

  it('reveals the inline edit panel when a row toggle is clicked', () => {
    render(<OutputDevicesScreen />);
    expect(screen.queryByTestId('edit-panel')).not.toBeInTheDocument();

    fireEvent.click(screen.getByLabelText('Edit output device Dock Strobe'));

    expect(screen.getByTestId('edit-panel')).toHaveTextContent('editing:Dock Strobe');
  });

  it('collapses the row when its toggle is clicked again', () => {
    render(<OutputDevicesScreen />);
    const toggle = screen.getByLabelText('Edit output device Dock Strobe');

    fireEvent.click(toggle);
    expect(screen.getByTestId('edit-panel')).toBeInTheDocument();

    fireEvent.click(toggle);
    expect(screen.queryByTestId('edit-panel')).not.toBeInTheDocument();
  });

  it('keeps only one row open at a time (single-open accordion)', () => {
    render(<OutputDevicesScreen />);

    fireEvent.click(screen.getByLabelText('Edit output device Dock Strobe'));
    expect(screen.getByTestId('edit-panel')).toHaveTextContent('editing:Dock Strobe');

    fireEvent.click(screen.getByLabelText('Edit output device Gate Horn'));
    const panels = screen.getAllByTestId('edit-panel');
    expect(panels).toHaveLength(1);
    expect(panels[0]).toHaveTextContent('editing:Gate Horn');
  });

  it('marks the toggle expanded state for assistive tech', () => {
    render(<OutputDevicesScreen />);
    const toggle = screen.getByLabelText('Edit output device Dock Strobe');
    expect(toggle).toHaveAttribute('aria-expanded', 'false');

    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
  });

  it('collapses the open row when that device is deleted', async () => {
    render(<OutputDevicesScreen />);
    fireEvent.click(screen.getByLabelText('Edit output device Dock Strobe'));
    expect(screen.getByTestId('edit-panel')).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText('Delete output device Dock Strobe'));
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(screen.queryByTestId('edit-panel')).not.toBeInTheDocument();
    });
  });
});
