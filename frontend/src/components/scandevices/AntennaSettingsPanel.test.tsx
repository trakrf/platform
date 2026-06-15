import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, act } from '@testing-library/react';
import { AntennaSettingsPanel } from './AntennaSettingsPanel';
import {
  useReaderConfig,
  useSetReaderConfig,
  useScanPoints,
  useScanPointMutations,
} from '@/hooks/scandevices';
import { useLocations } from '@/hooks/locations/useLocations';
import type { ReaderCapabilities, ReaderConfig, ScanPoint } from '@/types/scandevices';
import type { Location } from '@/types/locations';

vi.mock('@/hooks/scandevices');
vi.mock('@/hooks/locations/useLocations');
vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

const setConfig = vi.fn(() => Promise.resolve({ applied: 'pending_reload' }));
const create = vi.fn(() => Promise.resolve({} as ScanPoint));
const update = vi.fn(() => Promise.resolve({} as ScanPoint));

const caps = (over: Partial<ReaderCapabilities> = {}): ReaderCapabilities => ({
  contract_version: '1.0',
  reader_model: 'CSL CS463',
  antennas: 4,
  tx_power: { min_dbm: 10, max_dbm: 31.5, per_antenna: true },
  supports: ['tx_power_dbm'],
  unsupported: [],
  ...over,
});

const point = (over: Partial<ScanPoint>): ScanPoint => ({
  id: 1,
  org_id: 1,
  scan_device_id: 10,
  location_id: null,
  name: 'Antenna 1',
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
  ({ id: 100, external_key: 'receiving', name: 'Receiving', ...over }) as Location;

interface ReaderState {
  capabilities: ReaderCapabilities | undefined;
  config: ReaderConfig | undefined;
  isLoading: boolean;
  error: unknown;
}
const readerState: ReaderState = {
  capabilities: undefined,
  config: undefined,
  isLoading: false,
  error: null,
};

function setup(opts: {
  scanPoints?: ScanPoint[];
  locations?: Location[];
}) {
  vi.mocked(useReaderConfig).mockReturnValue(readerState as ReturnType<typeof useReaderConfig>);
  vi.mocked(useSetReaderConfig).mockReturnValue({
    setConfig,
    isSetting: false,
    error: null,
  } as unknown as ReturnType<typeof useSetReaderConfig>);
  vi.mocked(useScanPoints).mockReturnValue({
    scanPoints: opts.scanPoints ?? [],
    isLoading: false,
  } as unknown as ReturnType<typeof useScanPoints>);
  vi.mocked(useScanPointMutations).mockReturnValue({
    create,
    update,
    delete: vi.fn(),
  } as unknown as ReturnType<typeof useScanPointMutations>);
  vi.mocked(useLocations).mockReturnValue({
    locations: opts.locations ?? [],
    isLoading: false,
  } as unknown as ReturnType<typeof useLocations>);
}

describe('AntennaSettingsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    readerState.capabilities = undefined;
    readerState.config = undefined;
    readerState.isLoading = false;
    readerState.error = null;
  });
  afterEach(() => cleanup());

  it('shows a loading state while config loads', () => {
    readerState.isLoading = true;
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getByText(/loading reader config/i)).toBeInTheDocument();
  });

  it('shows an offline notice on error', () => {
    readerState.error = new Error('502');
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getByText(/reader did not respond/i)).toBeInTheDocument();
  });

  it('renders exactly capabilities.antennas rows', () => {
    readerState.capabilities = caps({ antennas: 4 });
    readerState.config = {};
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getAllByRole('slider')).toHaveLength(4);
    expect(screen.getByText('CSL CS463')).toBeInTheDocument();
  });

  it('seeds power from config.tx_power_dbm, defaulting absent antennas to max', () => {
    readerState.capabilities = caps({ antennas: 2 });
    readerState.config = { tx_power_dbm: [{ antenna: 1, power: 20 }] };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getByText('20.0 dBm')).toBeInTheDocument();
    expect(screen.getByText('31.5 dBm')).toBeInTheDocument();
  });

  it('reflects a scan point as enabled with its location selected', () => {
    readerState.capabilities = caps({ antennas: 2 });
    readerState.config = {};
    setup({
      scanPoints: [point({ id: 7, antenna_port: 1, location_id: 100, is_active: true })],
      locations: [location({ id: 100, name: 'Receiving' })],
    });
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getByLabelText(/enable antenna 1/i)).toBeChecked();
    expect(screen.getByLabelText(/enable antenna 2/i)).not.toBeChecked();
    // locationLabel() renders "Receiving (receiving)" when name !== external_key.
    expect(screen.getByText(/Receiving/)).toBeInTheDocument();
  });

  it('debounces a slider change ~2s then pushes the full tx_power_dbm map', async () => {
    vi.useFakeTimers();
    readerState.capabilities = caps({ antennas: 2 });
    readerState.config = {
      tx_power_dbm: [{ antenna: 1, power: 20 }, { antenna: 2, power: 25 }],
    };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.change(screen.getAllByRole('slider')[0], { target: { value: '15' } });
    expect(setConfig).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    expect(setConfig).toHaveBeenCalledTimes(1);
    expect(setConfig.mock.calls[0][0]).toEqual({
      tx_power_dbm: [
        { antenna: 1, power: 15 },
        { antenna: 2, power: 25 },
      ],
    });
    vi.useRealTimers();
  });

  it('shows the pending_reload note after a successful push', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = { tx_power_dbm: [{ antenna: 1, power: 20 }] };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);

    await act(async () => {
      fireEvent.change(screen.getByRole('slider'), { target: { value: '18' } });
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 2100));
    });

    expect(setConfig).toHaveBeenCalled();
    expect(screen.getByText(/next inventory cycle/i)).toBeInTheDocument();
  });

  it('shows a saving indicator while a power change is pending (before the debounce flushes)', () => {
    vi.useFakeTimers();
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = { tx_power_dbm: [{ antenna: 1, power: 20 }] };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.change(screen.getByRole('slider'), { target: { value: '15' } });

    expect(screen.getByText(/saving/i)).toBeInTheDocument();
    expect(setConfig).not.toHaveBeenCalled();
    vi.useRealTimers();
  });

  it('updates an existing scan point when its location changes', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = {};
    setup({
      scanPoints: [point({ id: 7, antenna_port: 1, location_id: null, is_active: true })],
      locations: [location({ id: 100, name: 'Receiving' })],
    });
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.click(screen.getByLabelText(/antenna 1 location/i)); // enter edit mode
    fireEvent.change(screen.getByLabelText(/antenna 1 location/i), {
      target: { value: '100' },
    });

    await act(async () => {});
    expect(update).toHaveBeenCalledWith({ id: 7, updates: { location_id: 100 } });
    expect(create).not.toHaveBeenCalled();
  });

  it('lazily creates an enabled scan point when location set on an antenna with none', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = {};
    setup({ scanPoints: [], locations: [location({ id: 100, name: 'Receiving' })] });
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.click(screen.getByLabelText(/antenna 1 location/i));
    fireEvent.change(screen.getByLabelText(/antenna 1 location/i), {
      target: { value: '100' },
    });

    await act(async () => {});
    expect(create).toHaveBeenCalledWith({
      antenna_port: 1,
      name: 'Antenna 1',
      location_id: 100,
      is_active: true,
    });
  });

  it('disables via update on an existing scan point', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = {};
    setup({ scanPoints: [point({ id: 7, antenna_port: 1, is_active: true })] });
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.click(screen.getByLabelText(/enable antenna 1/i)); // checked -> unchecked
    await act(async () => {});
    expect(update).toHaveBeenCalledWith({ id: 7, updates: { is_active: false } });
  });

  it('lazily creates an enabled scan point when enabling an antenna with none', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = {};
    setup({ scanPoints: [] });
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.click(screen.getByLabelText(/enable antenna 1/i)); // unchecked -> checked
    await act(async () => {});
    expect(create).toHaveBeenCalledWith({
      antenna_port: 1,
      name: 'Antenna 1',
      location_id: null,
      is_active: true,
    });
  });
});
