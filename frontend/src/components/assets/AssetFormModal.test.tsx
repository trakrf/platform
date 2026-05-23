import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { AssetFormModal } from './AssetFormModal';
import { useAssetStore } from '@/stores';
import * as useScanToInputModule from '@/hooks/useScanToInput';
import { assetsApi } from '@/lib/api/assets';
import type { Asset } from '@/types/assets';

vi.mock('@/lib/api/assets');
vi.mock('@/lib/tags/conflictCheck', () => ({
  checkTagConflict: vi.fn().mockResolvedValue(null),
}));

describe('AssetFormModal', () => {
  afterEach(() => {
    cleanup();
  });

  const mockOnClose = vi.fn();

  const mockAsset: Asset = {
    id: 1,
    org_id: 1,
    external_key: 'LAP-001',
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
    tags: [],
  } as Asset;

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

    // Seed real asset store with spy methods so the modal's selectors resolve
    useAssetStore.setState({
      addAsset: vi.fn(),
      updateCachedAsset: vi.fn(),
    } as any);

    // Mock the assetsApi.get for edit-mode prefetch
    (assetsApi.get as any).mockResolvedValue({ data: { data: mockAsset } });
  });

  it('does not render when isOpen is false', () => {
    const { container } = render(
      <AssetFormModal isOpen={false} mode="create" onClose={mockOnClose} />
    );

    expect(container.firstChild).toBeNull();
  });

  it('renders create modal when isOpen is true', () => {
    render(<AssetFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    expect(screen.getByText('Create New Asset')).toBeInTheDocument();
  });

  it('renders edit modal with asset identifier', () => {
    render(
      <AssetFormModal isOpen={true} mode="edit" asset={mockAsset} onClose={mockOnClose} />
    );

    expect(screen.getByText(`Edit Asset: ${mockAsset.external_key}`)).toBeInTheDocument();
  });

  it('closes modal when close button is clicked', () => {
    render(<AssetFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    const closeButton = screen.getByLabelText('Close modal');
    fireEvent.click(closeButton);

    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it('closes modal when backdrop is clicked', () => {
    const { container } = render(
      <AssetFormModal isOpen={true} mode="create" onClose={mockOnClose} />
    );

    const backdrop = container.firstChild as HTMLElement;
    fireEvent.click(backdrop);

    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it('does not close when clicking inside modal', () => {
    render(<AssetFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    const modalContent = screen.getByText('Create New Asset').closest('div');
    fireEvent.click(modalContent!);

    expect(mockOnClose).not.toHaveBeenCalled();
  });

  it('renders AssetForm inside modal', () => {
    render(<AssetFormModal isOpen={true} mode="create" onClose={mockOnClose} />);

    expect(screen.getByLabelText(/Asset ID/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Name/)).toBeInTheDocument();
  });

  it('fires DELETE for tags removed during edit (TRA-813)', async () => {
    const assetWithTags: Asset = {
      ...mockAsset,
      tags: [
        { id: 42, tag_type: 'rfid', value: 'TAG-A' },
        { id: 43, tag_type: 'rfid', value: 'TAG-B' },
      ],
    } as Asset;

    (assetsApi.get as any).mockResolvedValue({ data: { data: assetWithTags } });
    (assetsApi.update as any).mockResolvedValue({ data: { data: assetWithTags } });
    (assetsApi.removeTag as any).mockResolvedValue({ data: { data: { deleted: true } } });
    (assetsApi.addTag as any).mockResolvedValue({ data: { data: {} } });

    render(
      <AssetFormModal isOpen={true} mode="edit" asset={assetWithTags} onClose={mockOnClose} />
    );

    // Wait for the prefetch + form render
    const removeButtons = await screen.findAllByLabelText('Remove tag');
    // First two rows are existing read-only tags (with Remove buttons);
    // a third row is the blank new-tag input row.
    expect(removeButtons.length).toBeGreaterThanOrEqual(2);

    // Remove TAG-A (the first existing tag, id=42)
    fireEvent.click(removeButtons[0]);

    // Submit
    fireEvent.click(screen.getByText('Update Asset'));

    await waitFor(() => {
      expect(assetsApi.removeTag).toHaveBeenCalledWith(assetWithTags.id, 42);
    });
    expect(assetsApi.removeTag).not.toHaveBeenCalledWith(assetWithTags.id, 43);
  });

  it('has proper modal styling (shadow-xl)', () => {
    const { container } = render(
      <AssetFormModal isOpen={true} mode="create" onClose={mockOnClose} />
    );

    const modal = container.querySelector('.shadow-xl');
    expect(modal).toBeInTheDocument();
  });

  // TRA-817: when the modal is kept mounted by its parent (e.g. InventoryTableRow
  // passes isOpen as a prop rather than conditionally rendering), state was
  // retained across open/close cycles. A second Edit on the same row could
  // initialize the form from the previous edit's freshAsset snapshot and drive
  // a phantom DELETE via the TRA-813 diff. The fix unmounts internal state
  // each time the modal closes.
  describe('TRA-817 rapid re-edit on same asset (persistent-modal lifecycle)', () => {
    const asset3Tags: Asset = {
      ...mockAsset,
      tags: [
        { id: 11, tag_type: 'rfid', value: 'EPC-A' },
        { id: 12, tag_type: 'rfid', value: 'EPC-B' },
        { id: 13, tag_type: 'rfid', value: 'EPC-C' },
      ],
    };

    const asset2Tags: Asset = {
      ...mockAsset,
      tags: [
        { id: 11, tag_type: 'rfid', value: 'EPC-A' },
        { id: 12, tag_type: 'rfid', value: 'EPC-B' },
      ],
    };

    it('reflects current server tag list on re-open after a prior update', async () => {
      // First open returns 3 tags. Second open returns 2 (the prior update
      // removed one). The modal must show 2 tags, not 3 or 1.
      (assetsApi.get as any)
        .mockResolvedValueOnce({ data: { data: asset3Tags } })
        .mockResolvedValueOnce({ data: { data: asset2Tags } });

      const { rerender } = render(
        <AssetFormModal isOpen={true} mode="edit" asset={asset3Tags} onClose={mockOnClose} />,
      );

      await waitFor(() => {
        expect(screen.getByText('EPC-C')).toBeInTheDocument();
      });

      // Parent toggles modal closed while keeping it mounted.
      rerender(
        <AssetFormModal isOpen={false} mode="edit" asset={asset2Tags} onClose={mockOnClose} />,
      );

      // Parent re-opens with the freshly-updated asset prop.
      rerender(
        <AssetFormModal isOpen={true} mode="edit" asset={asset2Tags} onClose={mockOnClose} />,
      );

      await waitFor(() => {
        expect(screen.getByText('EPC-A')).toBeInTheDocument();
        expect(screen.getByText('EPC-B')).toBeInTheDocument();
      });

      // EPC-C is no longer attached and must not appear (would mean stale state).
      expect(screen.queryByText('EPC-C')).not.toBeInTheDocument();
    });

    it('does not render the prior-open tag list while the re-open GET is in flight', async () => {
      // Hold the second GET so we can observe the render between reopen and
      // its resolution. Without the fix, the first paint of the re-opened
      // modal still uses the prior-edit freshAsset (3 tags, including EPC-C)
      // before the in-flight fetch resolves.
      let resolveGet2: ((value: any) => void) | undefined;
      (assetsApi.get as any)
        .mockResolvedValueOnce({ data: { data: asset3Tags } })
        .mockImplementationOnce(
          () => new Promise((r) => {
            resolveGet2 = r;
          }),
        );

      const { rerender } = render(
        <AssetFormModal isOpen={true} mode="edit" asset={asset3Tags} onClose={mockOnClose} />,
      );

      await waitFor(() => {
        expect(screen.getByText('EPC-C')).toBeInTheDocument();
      });

      // Close (parent keeps modal mounted).
      rerender(
        <AssetFormModal isOpen={false} mode="edit" asset={asset2Tags} onClose={mockOnClose} />,
      );

      // Re-open. The second GET is still pending — nothing in the form should
      // reflect the prior edit's tag list.
      rerender(
        <AssetFormModal isOpen={true} mode="edit" asset={asset2Tags} onClose={mockOnClose} />,
      );

      expect(screen.queryByText('EPC-C')).not.toBeInTheDocument();

      // Drain the pending GET so React doesn't complain about leaked promises.
      resolveGet2?.({ data: { data: asset2Tags } });
      await waitFor(() => {
        expect(screen.getByText('EPC-B')).toBeInTheDocument();
      });
    });

    it('does not fire phantom DELETEs on re-open Update with no user changes', async () => {
      (assetsApi.get as any)
        .mockResolvedValueOnce({ data: { data: asset3Tags } })
        // Second open's prefetch returns 2 tags (post-prior-update server state).
        .mockResolvedValueOnce({ data: { data: asset2Tags } })
        // Post-submit re-fetch after the no-op Update.
        .mockResolvedValueOnce({ data: { data: asset2Tags } });
      (assetsApi.update as any).mockResolvedValue({ data: { data: asset2Tags } });
      (assetsApi.removeTag as any).mockResolvedValue({ data: {} });

      const { rerender } = render(
        <AssetFormModal isOpen={true} mode="edit" asset={asset3Tags} onClose={mockOnClose} />,
      );

      await waitFor(() => {
        expect(screen.getByText('EPC-C')).toBeInTheDocument();
      });

      // Close + re-open without modifying anything in between.
      rerender(
        <AssetFormModal isOpen={false} mode="edit" asset={asset2Tags} onClose={mockOnClose} />,
      );
      rerender(
        <AssetFormModal isOpen={true} mode="edit" asset={asset2Tags} onClose={mockOnClose} />,
      );

      await waitFor(() => {
        expect(screen.getByText('EPC-A')).toBeInTheDocument();
        expect(screen.getByText('EPC-B')).toBeInTheDocument();
      });

      const submitBtn = screen.getByRole('button', { name: /Update Asset/i });
      fireEvent.click(submitBtn);

      await waitFor(() => {
        expect((assetsApi.update as any)).toHaveBeenCalled();
      });

      // With a stale freshAsset baseline (3 tags) and a fresh tagInputs (2 tags),
      // the TRA-813 diff would mistakenly compute one removed id and fire a
      // DELETE. After the fix, the second open's baseline is the current server
      // response, so the diff is empty and no DELETE should be issued.
      expect((assetsApi.removeTag as any)).not.toHaveBeenCalled();
    });
  });
});
