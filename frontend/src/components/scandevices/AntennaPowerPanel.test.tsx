import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, act } from '@testing-library/react';
import { AntennaPowerPanel } from './AntennaPowerPanel';
import type { ReaderCapabilities, ReaderConfig } from '@/types/scandevices';

// --- Hook mocks -------------------------------------------------------------
const setConfig = vi.fn(() => Promise.resolve({ applied: 'pending_reload' }));

const readerConfigState: {
  capabilities: ReaderCapabilities | undefined;
  config: ReaderConfig | undefined;
  isLoading: boolean;
  error: unknown;
} = {
  capabilities: undefined,
  config: undefined,
  isLoading: false,
  error: null,
};

vi.mock('@/hooks/scandevices/useReaderConfig', () => ({
  useReaderConfig: () => readerConfigState,
  useSetReaderConfig: () => ({ setConfig, isSetting: false, error: null }),
}));

// --- toast mock -------------------------------------------------------------
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock('react-hot-toast', () => ({
  default: { success: (m: string) => toastSuccess(m), error: (m: string) => toastError(m) },
}));

const caps = (over: Partial<ReaderCapabilities> = {}): ReaderCapabilities => ({
  contract_version: '1.0',
  reader_model: 'CSL CS463',
  antennas: 4,
  tx_power: { min_dbm: 10, max_dbm: 31.5, per_antenna: true },
  supports: ['tx_power_dbm'],
  unsupported: [],
  ...over,
});

describe('AntennaPowerPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    readerConfigState.capabilities = undefined;
    readerConfigState.config = undefined;
    readerConfigState.isLoading = false;
    readerConfigState.error = null;
  });
  afterEach(() => cleanup());

  it('shows a loading state while config is loading', () => {
    readerConfigState.isLoading = true;
    render(<AntennaPowerPanel deviceId={10} />);
    expect(screen.getByText(/loading reader config/i)).toBeInTheDocument();
  });

  it('shows an offline notice on error', () => {
    readerConfigState.error = new Error('502');
    render(<AntennaPowerPanel deviceId={10} />);
    expect(screen.getByText(/reader did not respond/i)).toBeInTheDocument();
  });

  it('renders exactly capabilities.antennas sliders with capability bounds', () => {
    readerConfigState.capabilities = caps({ antennas: 4 });
    readerConfigState.config = {};
    render(<AntennaPowerPanel deviceId={10} />);

    const sliders = screen.getAllByRole('slider');
    expect(sliders).toHaveLength(4);
    for (const s of sliders) {
      expect(s).toHaveAttribute('min', '10');
      expect(s).toHaveAttribute('max', '31.5');
      expect(s).toHaveAttribute('step', '0.5');
    }
    expect(screen.getByText('CSL CS463')).toBeInTheDocument();
  });

  it('seeds slider values from config.tx_power_dbm, defaulting absent antennas to max', () => {
    readerConfigState.capabilities = caps({ antennas: 2 });
    readerConfigState.config = { tx_power_dbm: [{ antenna: 1, power: 20 }] };
    render(<AntennaPowerPanel deviceId={10} />);

    // Antenna 1 seeded to 20, antenna 2 defaults to max (31.5).
    expect(screen.getByText('20.0 dBm')).toBeInTheDocument();
    expect(screen.getByText('31.5 dBm')).toBeInTheDocument();
  });

  it('debounces a slider change ~2s then calls setConfig with tx_power_dbm', async () => {
    vi.useFakeTimers();
    readerConfigState.capabilities = caps({ antennas: 2 });
    readerConfigState.config = { tx_power_dbm: [{ antenna: 1, power: 20 }, { antenna: 2, power: 25 }] };
    render(<AntennaPowerPanel deviceId={10} />);

    const sliders = screen.getAllByRole('slider');
    fireEvent.change(sliders[0], { target: { value: '15' } });

    // Not called immediately.
    expect(setConfig).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    expect(setConfig).toHaveBeenCalledTimes(1);
    const arg = setConfig.mock.calls[0][0];
    expect(arg).toEqual({
      tx_power_dbm: [
        { antenna: 1, power: 15 },
        { antenna: 2, power: 25 },
      ],
    });
    vi.useRealTimers();
  });

  it('surfaces the pending_reload note after a successful push', async () => {
    readerConfigState.capabilities = caps({ antennas: 1 });
    readerConfigState.config = { tx_power_dbm: [{ antenna: 1, power: 20 }] };
    render(<AntennaPowerPanel deviceId={10} />);

    const slider = screen.getByRole('slider');
    await act(async () => {
      fireEvent.change(slider, { target: { value: '18' } });
    });

    // Real timers here; wait for debounce + the resolved promise.
    await act(async () => {
      await new Promise((r) => setTimeout(r, 2100));
    });

    expect(setConfig).toHaveBeenCalled();
    expect(toastSuccess).toHaveBeenCalledWith('Power update sent');
    expect(screen.getByText(/next inventory cycle/i)).toBeInTheDocument();
  });
});
