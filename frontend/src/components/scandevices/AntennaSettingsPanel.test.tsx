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
vi.mock('react-hot-toast', () => ({ default: { success: vi.fn(), error: vi.fn() } }));

const setConfig = vi.fn(() => Promise.resolve({ applied: 'pending_reload' }));
const create = vi.fn(() => Promise.resolve({} as ScanPoint));
const update = vi.fn(() => Promise.resolve({} as ScanPoint));
const retryWithForce = vi.fn();

const caps = (over: Partial<ReaderCapabilities> = {}): ReaderCapabilities => ({
  contract_version: '1',
  reader_model: 'CSL CS463',
  antennas: 4,
  tx_power: { min_dbm: 10, max_dbm: 31.5, per_antenna: true },
  supports: ['Reader.GetOperProfile', 'Reader.SetOperProfile'],
  unsupported: [],
  ...over,
});

const point = (over: Partial<ScanPoint>): ScanPoint => ({
  id: 1, org_id: 1, scan_device_id: 10, location_id: null, name: 'Antenna 1',
  antenna_port: 1, description: '', metadata: {}, valid_from: '2024-01-01T00:00:00Z',
  valid_to: null, is_active: true, created_at: '2024-01-01T00:00:00Z', updated_at: null,
  deleted_at: null, ...over,
});

const location = (over: Partial<Location>): Location =>
  ({ id: 100, external_key: 'receiving', name: 'Receiving', ...over }) as Location;

interface ReaderState {
  capabilities: ReaderCapabilities | undefined;
  config: ReaderConfig | undefined;
  isLoading: boolean;
  error: unknown;
  busy: { held_by: string } | null;
  retryWithForce: () => void;
}
const readerState: ReaderState = {
  capabilities: undefined, config: undefined, isLoading: false, error: null,
  busy: null, retryWithForce,
};

function setup(opts: { scanPoints?: ScanPoint[]; locations?: Location[] }) {
  vi.mocked(useReaderConfig).mockReturnValue(readerState as ReturnType<typeof useReaderConfig>);
  vi.mocked(useSetReaderConfig).mockReturnValue({
    setConfig, isSetting: false, error: null,
  } as unknown as ReturnType<typeof useSetReaderConfig>);
  vi.mocked(useScanPoints).mockReturnValue({
    scanPoints: opts.scanPoints ?? [], isLoading: false,
  } as unknown as ReturnType<typeof useScanPoints>);
  vi.mocked(useScanPointMutations).mockReturnValue({
    create, update, delete: vi.fn(),
  } as unknown as ReturnType<typeof useScanPointMutations>);
  vi.mocked(useLocations).mockReturnValue({
    locations: opts.locations ?? [], isLoading: false,
  } as unknown as ReturnType<typeof useLocations>);
}

describe('AntennaSettingsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    readerState.capabilities = undefined;
    readerState.config = undefined;
    readerState.isLoading = false;
    readerState.error = null;
    readerState.busy = null;
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
    readerState.config = { antennas: [] };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getAllByRole('slider')).toHaveLength(4);
  });

  it('reflects enablement + power from config.antennas (NOT scan points)', () => {
    readerState.capabilities = caps({ antennas: 2 });
    readerState.config = {
      antennas: [
        { antenna: 1, enabled: true, power_dbm: 20 },
        { antenna: 2, enabled: false, power_dbm: 31.5 },
      ],
    };
    // scan point says antenna 2 is_active=true, but the reader is the source of truth.
    setup({ scanPoints: [point({ antenna_port: 2, is_active: true })] });
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getByLabelText(/enable antenna 1/i)).toBeChecked();
    expect(screen.getByLabelText(/enable antenna 2/i)).not.toBeChecked();
    expect(screen.getByText('20.0 dBm')).toBeInTheDocument();
  });

  it('toggling an antenna pushes the full antennas[] to reader-config (not scan points)', async () => {
    readerState.capabilities = caps({ antennas: 2 });
    readerState.config = {
      antennas: [
        { antenna: 1, enabled: true, power_dbm: 20 },
        { antenna: 2, enabled: false, power_dbm: 25 },
      ],
    };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.click(screen.getByLabelText(/enable antenna 2/i)); // off -> on
    await act(async () => {});

    expect(setConfig).toHaveBeenCalledWith({
      body: {
        antennas: [
          { antenna: 1, enabled: true, power_dbm: 20 },
          { antenna: 2, enabled: true, power_dbm: 25 },
        ],
      },
      force: false,
    });
    expect(create).not.toHaveBeenCalled();
    expect(update).not.toHaveBeenCalled();
  });

  it('debounces a slider change then pushes the full antennas[]', async () => {
    vi.useFakeTimers();
    readerState.capabilities = caps({ antennas: 2 });
    readerState.config = {
      antennas: [
        { antenna: 1, enabled: true, power_dbm: 20 },
        { antenna: 2, enabled: true, power_dbm: 25 },
      ],
    };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);

    fireEvent.change(screen.getAllByRole('slider')[0], { target: { value: '15' } });
    expect(setConfig).not.toHaveBeenCalled();
    await act(async () => { vi.advanceTimersByTime(2000); });

    expect(setConfig).toHaveBeenCalledWith({
      body: {
        antennas: [
          { antenna: 1, enabled: true, power_dbm: 15 },
          { antenna: 2, enabled: true, power_dbm: 25 },
        ],
      },
      force: false,
    });
    vi.useRealTimers();
  });

  it('renders the Read Timing section seeded from config and the read-only RSSI gate', () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = {
      antennas: [{ antenna: 1, enabled: true, power_dbm: 30 }],
      dwell_ms: 500, dedup_window_ms: 500, rssi_gate_dbm: -80, antenna_differentiation: true,
    };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    // Read Timing inputs are present (detailed editing behaviour is covered in
    // ReadTimingSection.test.tsx); the read-only RSSI gate shows its value.
    expect(screen.getByLabelText(/dwell/i)).toHaveValue(500);
    expect(screen.getByLabelText(/dedup/i)).toHaveValue(500);
    expect(screen.getByText(/-80/)).toBeInTheDocument();
  });

  it('shows a busy banner with the holder IP and a force-retry button', () => {
    readerState.capabilities = undefined;
    readerState.busy = { held_by: '192.168.50.203' };
    setup({});
    render(<AntennaSettingsPanel deviceId={10} />);
    expect(screen.getByText(/192\.168\.50\.203/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /force logout/i }));
    expect(retryWithForce).toHaveBeenCalled();
  });

  it('still uses scan points for location', async () => {
    readerState.capabilities = caps({ antennas: 1 });
    readerState.config = { antennas: [{ antenna: 1, enabled: true, power_dbm: 30 }] };
    setup({
      scanPoints: [point({ id: 7, antenna_port: 1, location_id: null })],
      locations: [location({ id: 100, name: 'Receiving' })],
    });
    render(<AntennaSettingsPanel deviceId={10} />);
    fireEvent.click(screen.getByLabelText(/antenna 1 location/i));
    fireEvent.change(screen.getByLabelText(/antenna 1 location/i), { target: { value: '100' } });
    await act(async () => {});
    expect(update).toHaveBeenCalledWith({ id: 7, updates: { location_id: 100 } });
  });
});
