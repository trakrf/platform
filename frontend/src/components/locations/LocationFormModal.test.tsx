import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { LocationFormModal } from './LocationFormModal';
import { useLocationStore } from '@/stores/locations/locationStore';
import * as useScanToInputModule from '@/hooks/useScanToInput';
import { locationsApi } from '@/lib/api/locations';

vi.mock('@/lib/api/locations');

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
});
