import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { LocationForm } from './LocationForm';
import { useDeviceStore } from '@/stores';
import { useLocationStore } from '@/stores/locations/locationStore';
import * as useScanToInputModule from '@/hooks/useScanToInput';
import type { Location } from '@/types/locations';
import { checkTagConflict } from '@/lib/tags/conflictCheck';

vi.mock('@/lib/tags/conflictCheck');

describe('LocationForm - Scanner Integration', () => {
  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();
  const mockStartBarcodeScan = vi.fn();
  const mockStopScan = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();

    // Mock useScanToInput - only barcode scanning for tags
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

  it('should show scanner button in Tags section when device connected', () => {
    useDeviceStore.setState({ isConnected: true });

    render(
      <LocationForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
      />
    );

    // Find the Scan button in Tags section
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
      external_key: 'loc-1',
      name: 'Test Location',
      description: '',
      parent_id: null,
      valid_from: '2025-01-01T00:00:00Z',
      valid_to: null,
      is_active: true,
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-01T00:00:00Z',
      tags: [],
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
    external_key: `loc_${id}`,
    name: `Location ${id}`,
    description: '',
    parent_id: null,
    tree_path: `loc_${id}`,
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
    const parentLocation = createMockLocation(1, { external_key: 'warehouse-a' });
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
    const location = createMockLocation(1, { external_key: 'test-loc', name: 'Test Location' });
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

describe('LocationForm - Tag conflict', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('renders an inline conflict error and disables Save', async () => {
    vi.mocked(checkTagConflict).mockResolvedValue(
      'Tag already attached to asset "Forklift 7" — remove it there before attaching here.',
    );

    render(<LocationForm mode="create" onSubmit={vi.fn()} onCancel={vi.fn()} />);

    // Click "Add Tag" to add a tag row (create mode already has one blank row)
    const addTagButton = screen.getByRole('button', { name: /Add Tag/i });
    fireEvent.click(addTagButton);

    // Get the first tag input (index 0, already present in create mode)
    const tagInputs = screen.getAllByPlaceholderText('Enter tag number...');
    const tagInput = tagInputs[0];

    // Type a value and blur to trigger the conflict check
    fireEvent.change(tagInput, { target: { value: 'AABBCCDD' } });
    fireEvent.blur(tagInput);

    // The conflict message should appear
    await screen.findByText(/already attached to asset "Forklift 7"/);

    // Save button should be disabled
    expect(screen.getByRole('button', { name: /create/i })).toBeDisabled();
  });

  it('clears the conflict when the tag is free', async () => {
    vi.mocked(checkTagConflict).mockResolvedValue(null);

    render(<LocationForm mode="create" onSubmit={vi.fn()} onCancel={vi.fn()} />);

    const tagInput = screen.getByPlaceholderText('Enter tag number...');
    fireEvent.change(tagInput, { target: { value: 'AABBCCDD' } });
    fireEvent.blur(tagInput);

    await waitFor(() => {
      expect(
        screen.queryByText(/already attached/),
      ).not.toBeInTheDocument();
    });

    // Save button should NOT be disabled (only by conflict — loading is false)
    expect(screen.getByRole('button', { name: /create location/i })).not.toBeDisabled();
  });

  it('the "Reassign" modal is gone', async () => {
    vi.mocked(checkTagConflict).mockResolvedValue(null);

    render(<LocationForm mode="create" onSubmit={vi.fn()} onCancel={vi.fn()} />);

    const tagInput = screen.getByPlaceholderText('Enter tag number...');
    fireEvent.change(tagInput, { target: { value: 'AABBCCDD' } });
    fireEvent.blur(tagInput);

    await waitFor(() => {
      expect(screen.queryByText('Tag Already Assigned')).toBeNull();
    });
  });

  describe('tag identity is immutable in edit mode', () => {
    it('renders existing server-sourced tag values as read-only text', () => {
      const locationWithTag: Location = {
        id: 1,
        external_key: 'loc-1',
        name: 'Test Location',
        description: '',
        parent_id: null,
        valid_from: '2025-01-01T00:00:00Z',
        valid_to: null,
        is_active: true,
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
        tags: [{ id: 99, tag_type: 'rfid', value: 'AABBCCDD11223344AABBCCDD' }],
      } as Location;

      const { container } = render(
        <LocationForm
          mode="edit"
          location={locationWithTag}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />,
      );

      expect(screen.getByText('AABBCCDD11223344AABBCCDD')).toHaveAttribute(
        'aria-readonly',
        'true',
      );
      expect(screen.getByPlaceholderText('Enter tag number...')).toBeInTheDocument();
      expect(container.querySelectorAll('[aria-label="Remove tag"]').length).toBeGreaterThan(0);
    });

    it('renders newly-added tag rows as editable, not read-only', () => {
      render(<LocationForm mode="create" onSubmit={vi.fn()} onCancel={vi.fn()} />);

      const inputs = screen.getAllByPlaceholderText('Enter tag number...');
      expect(inputs).toHaveLength(1);

      fireEvent.click(screen.getByRole('button', { name: /Add Tag/i }));
      expect(screen.getAllByPlaceholderText('Enter tag number...')).toHaveLength(2);
    });
  });

  it('warns inline and disables Save when two rows have the same typed value', async () => {
    vi.mocked(checkTagConflict).mockResolvedValue(null);

    render(<LocationForm mode="create" onSubmit={vi.fn()} onCancel={vi.fn()} />);

    // Add a second tag row.
    fireEvent.click(screen.getByRole('button', { name: /Add Tag/i }));

    // Type the same value into both rows, blurring each to trigger the check.
    const tagInputs = screen.getAllByPlaceholderText('Enter tag number...');
    fireEvent.change(tagInputs[0], { target: { value: 'AABBCCDD' } });
    fireEvent.blur(tagInputs[0]);
    fireEvent.change(tagInputs[1], { target: { value: 'AABBCCDD' } });
    fireEvent.blur(tagInputs[1]);

    // The same-form duplicate warning should appear.
    await screen.findByText(/already in this form's tag list/i);

    // Save button should be disabled.
    expect(screen.getByRole('button', { name: /create/i })).toBeDisabled();
  });
});
