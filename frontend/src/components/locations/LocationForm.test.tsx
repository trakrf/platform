import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { LocationForm } from './LocationForm';
import { useDeviceStore } from '@/stores';
import { useLocationStore } from '@/stores/locations/locationStore';
import * as useScanToInputModule from '@/hooks/useScanToInput';
import type { Location } from '@/types/locations';

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
      setFocused: vi.fn(),
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

  it('should auto-add tag row and enable scan button in create mode', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    // Form auto-adds a blank tag row in create mode
    expect(screen.getByPlaceholderText('Enter tag number...')).toBeInTheDocument();

    // Button starts enabled with green styling due to auto-focus
    const scanButton = screen.getByText('Scan').closest('button');
    expect(scanButton?.className).toContain('text-green-600');
    expect(scanButton).not.toBeDisabled();
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

  it('should disable scan button when tag field loses focus', async () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    // Button starts enabled (auto-focus on blank row)
    const scanButton = screen.getByText('Scan').closest('button');
    expect(scanButton).not.toBeDisabled();

    // Blur the tag input
    const tagInput = screen.getByPlaceholderText('Enter tag number...');
    fireEvent.blur(tagInput);

    // Button should now be disabled with gray styling
    expect(scanButton).toBeDisabled();
    expect(scanButton?.className).toContain('text-gray-400');
  });
});

describe('LocationForm - Context-Aware Parent', () => {
  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();

  const createMockLocation = (id: number, overrides = {}): Location => ({
    id,
    org_id: 1,
    identifier: `loc_${id}`,
    name: `Location ${id}`,
    description: '',
    parent_location_id: null,
    path: `loc_${id}`,
    depth: 1,
    valid_from: '2024-01-01',
    valid_to: null,
    is_active: true,
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  });

  beforeEach(() => {
    vi.clearAllMocks();
    useLocationStore.getState().invalidateCache();
    useDeviceStore.setState({ isConnected: false });

    // Mock useScanToInput
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: vi.fn(),
      startBarcodeScan: vi.fn(),
      stopScan: vi.fn(),
      isScanning: false,
      scanType: null,
      setFocused: vi.fn(),
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('should show "Creating a top-level location" when no parentLocationId', () => {
    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText('Creating a top-level location')).toBeInTheDocument();
  });

  it('should show "Creating inside: {identifier}" when parentLocationId provided', () => {
    const parentLocation = createMockLocation(1, { identifier: 'warehouse-a' });
    useLocationStore.getState().setLocations([parentLocation]);

    render(
      <LocationForm
        mode="create"
        parentLocationId={1}
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    expect(screen.getByText(/Creating inside:/)).toBeInTheDocument();
    expect(screen.getByText('warehouse-a')).toBeInTheDocument();
  });

  it('should NOT show LocationParentSelector dropdown in create mode', () => {
    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    // The dropdown would have a "Select a parent" option or similar
    expect(screen.queryByText('Select a parent location or leave as root')).not.toBeInTheDocument();
  });

  it('should show LocationParentSelector in edit mode', () => {
    const location = createMockLocation(1, { identifier: 'test-loc', name: 'Test Location' });
    useLocationStore.getState().setLocations([location]);

    render(
      <LocationForm
        mode="edit"
        location={location}
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    // The helper text is shown only when the selector is visible
    expect(screen.getByText('Select a parent location or leave as root')).toBeInTheDocument();
  });
});
