import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import LocateScreen from '../LocateScreen';
import { LOCATE_TEST_TAG, EPC_FORMATS } from '@test-utils/constants';

// Mock the stores
let mockStatusMessage = 'Connected';
const mockSetStatusMessage = vi.fn((msg: string) => {
  mockStatusMessage = msg;
});

vi.mock('@/stores/locateStore', () => ({
  useLocateStore: () => ({
    currentRSSI: -120,
    averageRSSI: -120,
    peakRSSI: -120,
    updateRate: 0,
    rssiBuffer: [],
    get statusMessage() { return mockStatusMessage; },
    setStatusMessage: mockSetStatusMessage,
    getFilteredRSSI: () => -120
  })
}));

vi.mock('@/stores/deviceStore', () => ({
  useDeviceStore: Object.assign(() => ({
    triggerState: false,
    isConnected: true,
    readerMode: 'Locate'
  }), {
    getState: () => ({
      triggerState: false,
      isConnected: true,
      readerMode: 'Locate'
    })
  })
}));

const mockSetTargetEPC = vi.fn();
vi.mock('@/stores/settingsStore', () => ({
  useSettingsStore: Object.assign((selector?: any) => {
    const state = {
      rfid: {
        targetEPC: ''
      },
      setTargetEPC: mockSetTargetEPC
    };
    return selector ? selector(state) : state;
  }, {
    getState: () => ({
      rfid: {
        targetEPC: ''
      },
      setTargetEPC: mockSetTargetEPC
    })
  })
}));

// Mock the gauge component
vi.mock('react-gauge-component', () => ({
  default: () => <div>Gauge</div>
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