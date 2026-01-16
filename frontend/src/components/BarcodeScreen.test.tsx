import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import '@testing-library/jest-dom';
import BarcodeScreen from './BarcodeScreen';
import { useBarcodeStore } from '@/stores/barcodeStore';

// Mock the device store
vi.mock('@/stores', () => ({
  useDeviceStore: Object.assign(
    (selector?: (state: any) => any) => {
      const state = {
        readerState: 'Ready',
        triggerState: false,
        scanButtonActive: false,
        toggleScanButton: vi.fn(),
      };
      return selector ? selector(state) : state;
    },
    {
      getState: () => ({
        scanButtonActive: false,
      }),
      setState: vi.fn(),
    }
  ),
  useUIStore: Object.assign(
    () => ({
      setActiveTab: vi.fn(),
    }),
    {
      getState: () => ({
        setActiveTab: vi.fn(),
      }),
    }
  ),
}));

// Mock the barcode audio hook
vi.mock('@/hooks/useBarcodeAudio', () => ({
  useBarcodeAudio: () => {},
}));

describe('BarcodeScreen EPC Validation', () => {
  beforeEach(() => {
    // Reset store state before each test
    useBarcodeStore.setState({ barcodes: [], scanning: false });
  });

  afterEach(() => {
    cleanup();
  });

  it('shows no warning for valid 24-char hex EPC', () => {
    useBarcodeStore.setState({
      barcodes: [
        { data: 'E20034120000000022440401', type: 'EPC', timestamp: Date.now() },
      ],
    });
    render(<BarcodeScreen />);
    expect(screen.queryByTestId('epc-warning')).not.toBeInTheDocument();
  });

  it('shows no warning for valid 32-char hex EPC (128-bit)', () => {
    useBarcodeStore.setState({
      barcodes: [
        {
          data: 'E2003412000000002244040112345678',
          type: 'EPC',
          timestamp: Date.now(),
        },
      ],
    });
    render(<BarcodeScreen />);
    expect(screen.queryByTestId('epc-warning')).not.toBeInTheDocument();
  });

  it('shows warning for truncated EPC (< 24 chars)', () => {
    useBarcodeStore.setState({
      barcodes: [
        { data: 'E200341200000000', type: 'EPC', timestamp: Date.now() },
      ],
    });
    render(<BarcodeScreen />);
    expect(screen.getByTestId('epc-warning')).toHaveTextContent(
      'Scan may be incomplete'
    );
  });

  it('shows warning for non-hex characters', () => {
    useBarcodeStore.setState({
      barcodes: [
        { data: 'E20034120000GHIJ22440401', type: 'EPC', timestamp: Date.now() },
      ],
    });
    render(<BarcodeScreen />);
    expect(screen.getByTestId('epc-warning')).toHaveTextContent(
      'Invalid characters detected'
    );
  });

  it('shows warning for non-aligned length (25 chars)', () => {
    useBarcodeStore.setState({
      barcodes: [
        { data: 'E200341200000000224404012', type: 'EPC', timestamp: Date.now() },
      ],
    });
    render(<BarcodeScreen />);
    expect(screen.getByTestId('epc-warning')).toHaveTextContent(
      'must be divisible by 8'
    );
  });

  it('does not block Locate button when warning is shown', () => {
    useBarcodeStore.setState({
      barcodes: [{ data: 'E200341200', type: 'EPC', timestamp: Date.now() }],
    });
    render(<BarcodeScreen />);
    expect(screen.getByTestId('epc-warning')).toBeInTheDocument();
    expect(screen.getByTestId('locate-button')).toBeEnabled();
  });
});
