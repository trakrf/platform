import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { AssetFormModal } from './AssetFormModal';
import { useAssetStore } from '@/stores';
import * as useScanToInputModule from '@/hooks/useScanToInput';
import { assetsApi } from '@/lib/api/assets';
import type { Asset } from '@/types/assets';

vi.mock('@/lib/api/assets');

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
});
