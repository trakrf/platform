import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { AssetFormModal } from './AssetFormModal';
import { useAssetStore } from '@/stores';
import { assetsApi } from '@/lib/api/assets';
import type { Asset } from '@/types/assets';

vi.mock('@/stores');
vi.mock('@/lib/api/assets');

describe('AssetFormModal', () => {
  afterEach(() => {
    cleanup();
  });

  const mockOnClose = vi.fn();
  const mockAddAsset = vi.fn();
  const mockUpdateCachedAsset = vi.fn();

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

  beforeEach(() => {
    vi.clearAllMocks();
    const mockStore = {
      addAsset: mockAddAsset,
      updateCachedAsset: mockUpdateCachedAsset,
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));
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

    expect(screen.getByText(`Edit Asset: ${mockAsset.identifier}`)).toBeInTheDocument();
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

    expect(screen.getByLabelText(/Identifier/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Name/)).toBeInTheDocument();
  });

  it('has proper modal styling (shadow-xl)', () => {
    const { container } = render(
      <AssetFormModal isOpen={true} mode="create" onClose={mockOnClose} />
    );

    const modal = container.querySelector('.shadow-xl');
    expect(modal).toBeInTheDocument();
  });
});
