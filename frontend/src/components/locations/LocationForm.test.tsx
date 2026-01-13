import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { LocationForm } from './LocationForm';
import { useDeviceStore } from '@/stores';
import * as useScanToInputModule from '@/hooks/useScanToInput';

describe('LocationForm - Scanner Integration', () => {
  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();
  const mockStartBarcodeScan = vi.fn();
  const mockStopScan = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();

    // Mock useScanToInput - only barcode scanning for tag identifiers
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: vi.fn(),
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: false,
      scanType: null,
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('should show scanner button in Tag Identifiers section when device connected', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    // Find the Scan button in Tag Identifiers section
    expect(screen.getByText('Scan')).toBeInTheDocument();
    expect(screen.getByText('Add Tag')).toBeInTheDocument();
  });

  it('should hide scanner button when device not connected', () => {
    useDeviceStore.setState({ isConnected: false });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    // Scan button should not be present, but Add Tag should still be there
    expect(screen.queryByText('Scan')).not.toBeInTheDocument();
    expect(screen.getByText('Add Tag')).toBeInTheDocument();
  });

  it('should use green styling for scan button', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    const scanButton = screen.getByText('Scan').closest('button');
    expect(scanButton?.className).toContain('text-green-600');
  });

  it('should show scanner button in edit mode as well', () => {
    useDeviceStore.setState({ isConnected: true });

    const mockLocation = {
      id: 1,
      org_id: 1,
      identifier: 'loc-1',
      name: 'Test Location',
      description: '',
      parent_location_id: null,
      valid_from: '2025-01-01T00:00:00Z',
      valid_to: '2099-12-31T00:00:00Z',
      is_active: true,
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-01T00:00:00Z',
      identifiers: [],
    };

    render(
      <LocationForm
        mode="edit"
        location={mockLocation}
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    // Scanner should be available in edit mode
    expect(screen.getByText('Scan')).toBeInTheDocument();
  });

  it('should start barcode scan when scan button is clicked', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    fireEvent.click(screen.getByText('Scan'));
    expect(mockStartBarcodeScan).toHaveBeenCalledTimes(1);
  });
});
