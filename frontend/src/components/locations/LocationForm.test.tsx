import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { LocationForm } from './LocationForm';
import { useDeviceStore } from '@/stores';
import * as useScanToInputModule from '@/hooks/useScanToInput';

describe('LocationForm - Scanner Integration', () => {
  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();
  const mockStartRfidScan = vi.fn();
  const mockStartBarcodeScan = vi.fn();
  const mockStopScan = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();

    // Mock useScanToInput
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: false,
      scanType: null,
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('should show scanner buttons when device connected in create mode', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Scan RFID')).toBeInTheDocument();
    expect(screen.getByText('Scan Barcode')).toBeInTheDocument();
  });

  it('should use consistent styling with AssetForm', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    const rfidButton = screen.getByText('Scan RFID').closest('button');
    expect(rfidButton).toHaveClass('bg-blue-600');

    const barcodeButton = screen.getByText('Scan Barcode').closest('button');
    expect(barcodeButton).toHaveClass('bg-green-600');
  });

  it('should show scanning state feedback', () => {
    useDeviceStore.setState({ isConnected: true });
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: true,
      scanType: 'rfid',
    });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Scanning for RFID tag...')).toBeInTheDocument();
    // Look for the cancel button with red background (scanning cancel button)
    const cancelButtons = screen.getAllByText('Cancel');
    const scanningCancelButton = cancelButtons.find(
      btn => btn.closest('button')?.className.includes('bg-red-600')
    );
    expect(scanningCancelButton).toBeInTheDocument();
  });

  it('should call scanner functions correctly', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    fireEvent.click(screen.getByText('Scan RFID'));
    expect(mockStartRfidScan).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByText('Scan Barcode'));
    expect(mockStartBarcodeScan).toHaveBeenCalledTimes(1);
  });
});
