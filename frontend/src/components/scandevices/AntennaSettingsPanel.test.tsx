import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
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
});
