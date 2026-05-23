import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { LocationFormModal } from './LocationFormModal';
import { useLocationStore } from '@/stores/locations/locationStore';
import * as useScanToInputModule from '@/hooks/useScanToInput';
import { locationsApi } from '@/lib/api/locations';
import type { Location } from '@/types/locations';

vi.mock('@/lib/api/locations');
vi.mock('@/lib/tags/conflictCheck', () => ({
  checkTagConflict: vi.fn().mockResolvedValue(null),
}));

describe('LocationFormModal', () => {
  afterEach(() => {
    cleanup();
  });

  const mockOnClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();

    // Spy on useScanToInput so the real useBarcodeStore path is bypassed
    vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
      startRfidScan: vi.fn(),
      startBarcodeScan: vi.fn(),
      stopScan: vi.fn(),
      isScanning: false,
      scanType: null,
      setFocused: vi.fn(),
    });

    // Seed real location store with spy methods so the modal's selectors resolve
    useLocationStore.setState({
      addLocation: vi.fn(),
      updateLocation: vi.fn(),
      getLocationById: vi.fn().mockReturnValue(null),
    } as any);
  });

  it('surfaces a save-time 409 detail', async () => {
    const conflictDetail =
      'tag rfid:E2-X already exists — it is attached to asset "Forklift 7" (AST-7); remove it there before attaching here';

    (locationsApi.create as any).mockRejectedValue({
      response: {
        status: 409,
        data: {
          error: {
            detail: conflictDetail,
          },
        },
      },
    });

    render(<LocationFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    // Fill in required fields
    fireEvent.change(screen.getByPlaceholderText('e.g., warehouse_a'), {
      target: { value: 'loc-test' },
    });
    fireEvent.change(screen.getByPlaceholderText('e.g., Main Warehouse'), {
      target: { value: 'Test Location' },
    });

    // Submit the form
    fireEvent.click(screen.getByText('Create Location'));

    // Expect the real detail message to appear, not the generic axios string
    await screen.findByText(/attached to asset "Forklift 7"/);
  });

  // TRA-817: regression — re-edit on the same location after a tag removal
  // must not carry the prior open's tag-diff baseline into the next Update.
  describe('TRA-817 rapid re-edit on same location (persistent-modal lifecycle)', () => {
    const baseLocation: Location = {
      id: 7,
      external_key: 'WAREHOUSE-A',
      name: 'Warehouse A',
      description: '',
      parent_id: null,
      parent_external_key: null,
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      is_active: true,
      metadata: {},
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      tags: [],
    };

    const location3Tags: Location = {
      ...baseLocation,
      tags: [
        { id: 21, tag_type: 'rfid', value: 'LOC-A' },
        { id: 22, tag_type: 'rfid', value: 'LOC-B' },
        { id: 23, tag_type: 'rfid', value: 'LOC-C' },
      ],
    };

    const location2Tags: Location = {
      ...baseLocation,
      tags: [
        { id: 21, tag_type: 'rfid', value: 'LOC-A' },
        { id: 22, tag_type: 'rfid', value: 'LOC-B' },
      ],
    };

    it('reflects the latest location prop on re-open after a prior update', async () => {
      const { rerender } = render(
        <LocationFormModal isOpen={true} mode="edit" location={location3Tags} onClose={mockOnClose} />,
      );

      await waitFor(() => {
        expect(screen.getByText('LOC-C')).toBeInTheDocument();
      });

      rerender(
        <LocationFormModal isOpen={false} mode="edit" location={location2Tags} onClose={mockOnClose} />,
      );
      rerender(
        <LocationFormModal isOpen={true} mode="edit" location={location2Tags} onClose={mockOnClose} />,
      );

      await waitFor(() => {
        expect(screen.getByText('LOC-A')).toBeInTheDocument();
        expect(screen.getByText('LOC-B')).toBeInTheDocument();
      });
      expect(screen.queryByText('LOC-C')).not.toBeInTheDocument();
    });

    it('does not fire phantom DELETEs on re-open Update with no user changes', async () => {
      (locationsApi.update as any).mockResolvedValue({ data: { data: location2Tags } });
      (locationsApi.get as any).mockResolvedValue({ data: { data: location2Tags } });
      (locationsApi.removeTag as any).mockResolvedValue({ data: {} });

      const { rerender } = render(
        <LocationFormModal isOpen={true} mode="edit" location={location3Tags} onClose={mockOnClose} />,
      );

      await waitFor(() => {
        expect(screen.getByText('LOC-C')).toBeInTheDocument();
      });

      rerender(
        <LocationFormModal isOpen={false} mode="edit" location={location2Tags} onClose={mockOnClose} />,
      );
      rerender(
        <LocationFormModal isOpen={true} mode="edit" location={location2Tags} onClose={mockOnClose} />,
      );

      await waitFor(() => {
        expect(screen.getByText('LOC-A')).toBeInTheDocument();
      });

      const submitBtn = screen.getByRole('button', { name: /Update Location/i });
      fireEvent.click(submitBtn);

      await waitFor(() => {
        expect((locationsApi.update as any)).toHaveBeenCalled();
      });

      expect((locationsApi.removeTag as any)).not.toHaveBeenCalled();
    });
  });
});
