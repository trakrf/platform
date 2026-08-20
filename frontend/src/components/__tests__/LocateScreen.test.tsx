import '@testing-library/jest-dom';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import LocateScreen from '../LocateScreen';
import { LOCATE_TEST_TAG, EPC_FORMATS } from '@test-utils/constants';
import { ReaderState } from '@/worker/types/reader';

// Mock the stores
let mockStatusMessage = 'Connected';
const mockSetStatusMessage = vi.fn((msg: string) => {
  mockStatusMessage = msg;
});

// Mutable locate-store readings so tests can simulate reads arriving (or not)
// independently of the reader's state machine — that split is the whole point
// of TRA-1080.
let mockFilteredRSSI = -120;
// When true, getStatistics() reports the signal as gone while the raw store
// fields keep the frozen values a finished search left behind — the TRA-1123
// state the screen used to render straight to the operator.
let mockStatsStale = false;
const mockSetTarget = vi.fn();
const mockLocateStats = {
  currentRSSI: -120,
  averageRSSI: -120,
  peakRSSI: -120,
  updateRate: 0,
  rssiBuffer: [] as unknown[]
};

vi.mock('@/stores/locateStore', () => ({
  useLocateStore: () => ({
    get currentRSSI() { return mockLocateStats.currentRSSI; },
    get averageRSSI() { return mockLocateStats.averageRSSI; },
    get peakRSSI() { return mockLocateStats.peakRSSI; },
    get updateRate() { return mockLocateStats.updateRate; },
    get rssiBuffer() { return mockLocateStats.rssiBuffer; },
    get statusMessage() { return mockStatusMessage; },
    setStatusMessage: mockSetStatusMessage,
    setTarget: mockSetTarget,
    getFilteredRSSI: () => mockFilteredRSSI,
    getStatistics: () => mockStatsStale
      ? { currentRSSI: -120, averageRSSI: -120, peakRSSI: -120, updateRate: 0 }
      : {
          currentRSSI: mockLocateStats.currentRSSI,
          averageRSSI: mockLocateStats.averageRSSI,
          peakRSSI: mockLocateStats.peakRSSI,
          updateRate: mockLocateStats.updateRate
        }
  })
}));

// Selector-aware device store. The previous mock ignored the selector and
// returned the whole state object, so `readerState` was never a real value and
// no test could exercise state-dependent rendering.
const mockToggleScanButton = vi.fn();
let mockDeviceState: Record<string, unknown> = {};
const resetDeviceState = () => {
  mockDeviceState = {
    triggerState: false,
    isConnected: true,
    readerMode: 'Locate',
    readerState: ReaderState.CONNECTED,
    scanButtonActive: false,
    toggleScanButton: mockToggleScanButton
  };
};
resetDeviceState();

vi.mock('@/stores/deviceStore', () => ({
  useDeviceStore: Object.assign(
    (selector?: (s: Record<string, unknown>) => unknown) =>
      selector ? selector(mockDeviceState) : mockDeviceState,
    {
      getState: () => mockDeviceState,
      setState: (patch: Record<string, unknown>) => {
        mockDeviceState = { ...mockDeviceState, ...patch };
      }
    }
  )
}));

// Web Audio has no jsdom implementation; the tone hook is not under test here.
vi.mock('@/hooks/useWebAudioTone', () => ({
  useWebAudioTone: () => ({
    updateProximity: vi.fn(),
    startSearching: vi.fn(),
    stopBeeping: vi.fn(),
    toggleSound: vi.fn(),
    isEnabled: false,
    isPlaying: false
  })
}));

const mockSetTargetEPC = vi.fn();
let mockStoredEPC = '';
vi.mock('@/stores/settingsStore', () => {
  const readState = () => ({
    rfid: {
      targetEPC: mockStoredEPC
    },
    setTargetEPC: mockSetTargetEPC
  });
  return {
    useSettingsStore: Object.assign(
      (selector?: any) => (selector ? selector(readState()) : readState()),
      { getState: readState }
    )
  };
});

// Mock the gauge component. It renders the same formatted text the real gauge
// shows in its value label, so tests can assert what the user actually reads.
vi.mock('react-gauge-component', () => ({
  default: ({ value, labels }: {
    value: number;
    labels?: { valueLabel?: { formatTextValue?: (v: number) => string } };
  }) => (
    <div data-testid="gauge-value">
      {labels?.valueLabel?.formatTextValue ? labels.valueLabel.formatTextValue(value) : String(value)}
    </div>
  )
}));

describe('LocateScreen EPC Input', () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    mockSetTargetEPC.mockClear();
    mockSetStatusMessage.mockClear();
    mockStatusMessage = 'Connected';
  });

  it('should allow typing partial EPC values', async () => {
    render(<LocateScreen />);

    const input = screen.getByTestId('target-epc-display') as HTMLInputElement;

    // Type single character - React will uppercase it
    fireEvent.change(input, { target: { value: '1' } });
    // Wait for React to process the change
    await waitFor(() => {
      expect(input.value).toBe('1');
    });

    // Type multiple characters
    fireEvent.change(input, { target: { value: LOCATE_TEST_TAG } });
    await waitFor(() => {
      expect(input.value).toBe(LOCATE_TEST_TAG);
    });

    // Should convert to uppercase
    fireEvent.change(input, { target: { value: 'abc' } });
    await waitFor(() => {
      expect(input.value).toBe('ABC');
    });
  });

  it('should validate on blur and call setTargetEPC', async () => {
    render(<LocateScreen />);

    const input = screen.getByTestId('target-epc-display') as HTMLInputElement;

    // Type an odd number of characters
    fireEvent.change(input, { target: { value: LOCATE_TEST_TAG } });

    // Mock validation failure
    mockSetTargetEPC.mockReturnValue(false);

    // Blur the input
    fireEvent.blur(input);

    // Check that setTargetEPC was called
    expect(mockSetTargetEPC).toHaveBeenCalledWith(LOCATE_TEST_TAG);

    // Check that setStatusMessage was called with error
    expect(mockSetStatusMessage).toHaveBeenCalledWith('Invalid EPC format. Must contain only hexadecimal characters (0-9, A-F).');
  });

  it('should validate on Enter key and accept even number of hex characters', async () => {
    render(<LocateScreen />);

    const input = screen.getByTestId('target-epc-display') as HTMLInputElement;

    // Type an even number of hex characters
    const paddedTag = EPC_FORMATS.toCustomerInput(LOCATE_TEST_TAG).slice(-6); // Get last 6 chars for '010020' format
    fireEvent.change(input, { target: { value: paddedTag } });

    // Mock successful validation
    mockSetTargetEPC.mockReturnValue(true);

    // Press Enter
    fireEvent.keyDown(input, { key: 'Enter' });

    // Check that setTargetEPC was called
    expect(mockSetTargetEPC).toHaveBeenCalledWith(paddedTag);

    // Check that setStatusMessage was called with success
    expect(mockSetStatusMessage).toHaveBeenCalledWith('EPC updated. Press trigger to start searching.');
  });

  it('should only accept hex characters', async () => {
    render(<LocateScreen />);

    const input = screen.getByTestId('target-epc-display') as HTMLInputElement;

    // Type valid hex
    fireEvent.change(input, { target: { value: 'ABCDEF0123456789' } });
    expect(input.value).toBe('ABCDEF0123456789');

    // Mock successful validation for valid hex
    mockSetTargetEPC.mockReturnValue(true);
    fireEvent.blur(input);
    expect(mockSetTargetEPC).toHaveBeenCalledWith('ABCDEF0123456789');
  });
});

/**
 * TRA-1080: the Signal Strength gauge and the Status row were driven by
 * `readerState === SCANNING`, while the Statistics panel was driven by the
 * locate ring buffer. Whenever the reader sat in any non-SCANNING state with
 * reads still streaming in — observed live with the reader in ERROR at 14 Hz —
 * the screen contradicted itself: "No signal" and "Idle" next to a live dBm
 * reading and a non-zero update rate.
 *
 * "No signal" on a tag finder means "the item is not here", so this is a false
 * negative on the primary function of the screen. Both indicators must follow
 * the same signal that feeds Statistics.
 */
describe('LocateScreen signal display (TRA-1080)', () => {
  const statusRowValue = () =>
    screen.getByText('Status:').parentElement?.lastElementChild?.textContent?.trim();

  afterEach(() => {
    cleanup();
    resetDeviceState();
    mockFilteredRSSI = -120;
    mockLocateStats.updateRate = 0;
    mockLocateStats.currentRSSI = -120;
  });

  it('shows the live RSSI on the gauge when reads arrive but the reader is not Scanning', async () => {
    mockDeviceState.readerState = ReaderState.CONNECTED;
    mockFilteredRSSI = -35;
    mockLocateStats.currentRSSI = -35;
    mockLocateStats.updateRate = 13.5;

    render(<LocateScreen />);

    expect(await screen.findByTestId('gauge-value')).toHaveTextContent('-35 dBm');
  });

  it('shows Status "Searching" when reads arrive but the reader is not Scanning', async () => {
    mockDeviceState.readerState = ReaderState.CONNECTED;
    mockFilteredRSSI = -35;
    mockLocateStats.updateRate = 13.5;

    render(<LocateScreen />);
    await screen.findByTestId('gauge-value');

    expect(statusRowValue()).toBe('Searching');
  });

  it('never renders "No signal" while the Statistics panel reports a live reading', async () => {
    // The exact contradiction from the ticket: reader in ERROR, reads flowing.
    mockDeviceState.readerState = ReaderState.ERROR;
    mockFilteredRSSI = -35;
    mockLocateStats.currentRSSI = -35;
    mockLocateStats.updateRate = 13.5;

    render(<LocateScreen />);
    const gauge = await screen.findByTestId('gauge-value');

    expect(gauge).not.toHaveTextContent('No signal');
    expect(statusRowValue()).not.toBe('Idle');
  });

  it('still reports "No signal" and "Idle" when no reads are arriving', async () => {
    mockDeviceState.readerState = ReaderState.CONNECTED;
    mockFilteredRSSI = -120; // stale/absent — getFilteredRSSI floors at DEFAULT_RSSI

    render(<LocateScreen />);

    expect(await screen.findByTestId('gauge-value')).toHaveTextContent('No signal');
    expect(statusRowValue()).toBe('Idle');
  });

  it('reports "Searching" while Scanning even before the first read lands', async () => {
    mockDeviceState.readerState = ReaderState.SCANNING;
    mockFilteredRSSI = -120;

    render(<LocateScreen />);
    await screen.findByTestId('gauge-value');

    expect(statusRowValue()).toBe('Searching');
  });
});

describe('LocateScreen heading', () => {
  afterEach(() => {
    cleanup();
  });

  /**
   * Header already titles this tab "Locate" from PAGE_TITLES, so the screen must
   * not print a second page heading of its own — it used to say "Find Item",
   * disagreeing with both the header and the nav item (TRA-1071). Assets,
   * Locations, Reports and Scan all rely on the header for this.
   */
  it('does not print its own page heading', () => {
    render(<LocateScreen />);

    expect(screen.queryByText('Find Item')).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Locate' })).not.toBeInTheDocument();
  });
});
/**
 * TRA-1123: the Statistics rows read four store fields that are only ever
 * recalculated when a reading arrives, so a search returning nothing shows the
 * previous search's signal — "Current: -36 dBm / Peak: -35 dBm / 14.5 Hz" for a
 * tag that is not in the building. Observed on hardware with a decoy EPC
 * matching no tag on the bench, where it briefly read as a tag-mask defect.
 *
 * The four rows and the Signal History panel must follow the same staleness
 * signal the gauge follows.
 */
describe('LocateScreen stale statistics (TRA-1123)', () => {
  const rowValue = (label: string) =>
    screen.getByText(label).parentElement?.lastElementChild?.textContent?.trim();

  afterEach(() => {
    cleanup();
    resetDeviceState();
    mockStatsStale = false;
    mockFilteredRSSI = -120;
    mockLocateStats.currentRSSI = -120;
    mockLocateStats.averageRSSI = -120;
    mockLocateStats.peakRSSI = -120;
    mockLocateStats.updateRate = 0;
    mockLocateStats.rssiBuffer = [];
  });

  it('shows No signal on every statistic when the last read has gone stale', () => {
    // The frozen fields a finished search leaves behind in the store.
    mockLocateStats.currentRSSI = -36;
    mockLocateStats.averageRSSI = -36;
    mockLocateStats.peakRSSI = -35;
    mockLocateStats.updateRate = 14.5;
    mockStatsStale = true;

    render(<LocateScreen />);

    expect(rowValue('Current:')).toBe('No signal');
    expect(rowValue('Average (1s):')).toBe('No signal');
    expect(rowValue('Peak:')).toBe('No signal');
    expect(rowValue('Update Rate:')).toBe('0 Hz');
  });

  it('does not print a Signal History range for a search that is hearing nothing', () => {
    mockLocateStats.rssiBuffer = [{ timestamp: Date.now(), nb_rssi: -36 }];
    mockStatsStale = true;
    mockFilteredRSSI = -120;

    render(<LocateScreen />);

    expect(screen.queryByText('Signal History (10s)')).not.toBeInTheDocument();
  });

  it('still shows live statistics while reads are arriving', () => {
    mockLocateStats.currentRSSI = -35;
    mockLocateStats.averageRSSI = -36;
    mockLocateStats.updateRate = 13.5;
    mockFilteredRSSI = -35;

    render(<LocateScreen />);

    expect(rowValue('Current:')).toBe('-35 dBm');
    expect(rowValue('Average (1s):')).toBe('-36 dBm');
    expect(rowValue('Update Rate:')).toBe('13.5 Hz');
  });
});

/**
 * TRA-1123: the ring buffer is module-level state that outlives both the
 * screen and the target. Retarget — by typing, by the Locate deep link, or by
 * coming back to the tab — and the previous tag's readings are still what the
 * screen renders. Point the store at the current target whenever the screen
 * knows what it is, so the readings that no longer describe it are dropped.
 */
describe('LocateScreen target handoff (TRA-1123)', () => {
  afterEach(() => {
    cleanup();
    resetDeviceState();
    mockSetTarget.mockClear();
    mockStoredEPC = '';
  });

  it('points the locate buffer at the stored target on mount', () => {
    // The deep-link path: App.tsx stores the EPC, then the tab mounts.
    mockStoredEPC = 'E280689400000000001018DD';

    render(<LocateScreen />);

    expect(mockSetTarget).toHaveBeenCalledWith('E280689400000000001018DD');
  });

  it('follows the target when it changes under the screen', () => {
    mockStoredEPC = 'E280689400000000001018DD';
    const { rerender } = render(<LocateScreen />);
    mockSetTarget.mockClear();

    mockStoredEPC = 'E280689400000000001018EE';
    rerender(<LocateScreen />);

    expect(mockSetTarget).toHaveBeenCalledWith('E280689400000000001018EE');
  });
});
