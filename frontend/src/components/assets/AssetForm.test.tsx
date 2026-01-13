import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { AssetForm } from './AssetForm';
import type { Asset } from '@/types/assets';
import { useDeviceStore } from '@/stores';
import * as useScanToInputModule from '@/hooks/useScanToInput';

describe('AssetForm', () => {
  afterEach(() => {
    cleanup();
  });

  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();

  const mockAsset: Asset = {
    id: 1,
    org_id: 1,
    identifier: 'LAP-001',
    name: 'Test Laptop',
    type: 'device',
    description: 'Test description',
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    metadata: {},
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    deleted_at: null,
  };

  it('renders create mode form', () => {
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    expect(screen.getByLabelText(/Identifier/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Name/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Type/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Create Asset/ })).toBeInTheDocument();
  });

  it('renders edit mode form with asset data', () => {
    render(
      <AssetForm mode="edit" asset={mockAsset} onSubmit={mockOnSubmit} onCancel={mockOnCancel} />
    );

    expect(screen.getByDisplayValue('LAP-001')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Test Laptop')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Update Asset/ })).toBeInTheDocument();
  });

  it('disables identifier field in edit mode', () => {
    render(
      <AssetForm mode="edit" asset={mockAsset} onSubmit={mockOnSubmit} onCancel={mockOnCancel} />
    );

    const identifierInput = screen.getByDisplayValue('LAP-001');
    expect(identifierInput).toBeDisabled();
  });

  it('validates required fields', async () => {
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    // Clear name field and submit
    const nameInput = screen.getByLabelText(/Name/);
    fireEvent.change(nameInput, { target: { value: '' } });

    const submitButton = screen.getByRole('button', { name: /Create Asset/ });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText('Name is required')).toBeInTheDocument();
    });

    expect(mockOnSubmit).not.toHaveBeenCalled();
  });

  it('validates identifier format', async () => {
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    const identifierInput = screen.getByLabelText(/Identifier/);
    fireEvent.change(identifierInput, { target: { value: 'invalid id!' } });

    const submitButton = screen.getByRole('button', { name: /Create Asset/ });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(
        screen.getByText(/must contain only letters, numbers, hyphens, and underscores/)
      ).toBeInTheDocument();
    });
  });

  it('calls onSubmit with form data when valid', async () => {
    mockOnSubmit.mockResolvedValue(undefined);
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    fireEvent.change(screen.getByLabelText(/Identifier/), { target: { value: 'TEST-001' } });
    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Test Asset' } });

    const submitButton = screen.getByRole('button', { name: /Create Asset/ });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalled();
    });
  });

  it('calls onCancel when cancel button is clicked', () => {
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    const cancelButton = screen.getByRole('button', { name: /Cancel/ });
    fireEvent.click(cancelButton);

    expect(mockOnCancel).toHaveBeenCalledTimes(1);
  });

  it('shows loading state during submission', () => {
    render(
      <AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} loading={true} />
    );

    expect(screen.getByText('Saving...')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Saving/ })).toBeDisabled();
  });

  it('displays error message when provided', () => {
    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
        error="Test error message"
      />
    );

    expect(screen.getByText('Test error message')).toBeInTheDocument();
  });

  it('clears field error when user starts typing', async () => {
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    // Trigger validation error
    const nameInput = screen.getByLabelText(/Name/);
    fireEvent.change(nameInput, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: /Create Asset/ }));

    await waitFor(() => {
      expect(screen.getByText('Name is required')).toBeInTheDocument();
    });

    // Type in field - error should clear
    fireEvent.change(nameInput, { target: { value: 'Test' } });

    await waitFor(() => {
      expect(screen.queryByText('Name is required')).not.toBeInTheDocument();
    });
  });
});

describe('AssetForm - Scanner Integration', () => {
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
      setFocused: vi.fn(),
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('should show scanner buttons when device connected in create mode', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Scan RFID')).toBeInTheDocument();
    expect(screen.getByText('Scan Barcode')).toBeInTheDocument();
  });

  it('should hide scanner buttons when device disconnected', () => {
    useDeviceStore.setState({ isConnected: false });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.queryByText('Scan RFID')).not.toBeInTheDocument();
    expect(screen.queryByText('Scan Barcode')).not.toBeInTheDocument();
  });

  it('should hide scanner buttons in edit mode', () => {
    useDeviceStore.setState({ isConnected: true });

    const mockAsset: Asset = {
      id: 1,
      org_id: 1,
      identifier: 'TEST-001',
      name: 'Test Asset',
      type: 'device',
      description: '',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: '2099-12-31T00:00:00Z',
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
      metadata: {},
    };

    render(
      <AssetForm
        mode="edit"
        asset={mockAsset}
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.queryByText('Scan RFID')).not.toBeInTheDocument();
  });

  it('should call startRfidScan when RFID button clicked', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    fireEvent.click(screen.getByText('Scan RFID'));
    expect(mockStartRfidScan).toHaveBeenCalledTimes(1);
  });

  it('should call startBarcodeScan when Barcode button clicked', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    fireEvent.click(screen.getByText('Scan Barcode'));
    expect(mockStartBarcodeScan).toHaveBeenCalledTimes(1);
  });

  it('should show scanning state feedback for RFID', () => {
    useDeviceStore.setState({ isConnected: true });
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: true,
      scanType: 'rfid',
      setFocused: vi.fn(),
    });

    render(
      <AssetForm
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

  it('should show scanning state feedback for barcode', () => {
    useDeviceStore.setState({ isConnected: true });
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: true,
      scanType: 'barcode',
      setFocused: vi.fn(),
    });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Scanning for barcode...')).toBeInTheDocument();
  });

  it('should disable input while scanning', () => {
    useDeviceStore.setState({ isConnected: true });
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: mockStartRfidScan,
      startBarcodeScan: mockStartBarcodeScan,
      stopScan: mockStopScan,
      isScanning: true,
      scanType: 'rfid',
      setFocused: vi.fn(),
    });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    const input = screen.getByPlaceholderText(/Scanning RFID/i);
    expect(input).toBeDisabled();
  });

  it('should populate identifier when onScan callback triggers', async () => {
    useDeviceStore.setState({ isConnected: true });
    let capturedOnScan: ((value: string) => void) | null = null;

    vi.spyOn(useScanToInputModule, 'useScanToInput').mockImplementation(({ onScan }) => {
      capturedOnScan = onScan;
      return {
        startRfidScan: mockStartRfidScan,
        startBarcodeScan: mockStartBarcodeScan,
        stopScan: mockStopScan,
        isScanning: false,
        scanType: null,
      };
    });

    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    // Simulate scan callback
    capturedOnScan?.('E280116060000020957C5876');

    await waitFor(() => {
      const input = screen.getByPlaceholderText(/e.g., LAP-001/i) as HTMLInputElement;
      expect(input.value).toBe('E280116060000020957C5876');
    });
  });
});
