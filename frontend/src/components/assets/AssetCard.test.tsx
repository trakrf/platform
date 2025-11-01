import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { AssetCard } from './AssetCard';
import type { Asset } from '@/types/assets';

describe('AssetCard', () => {
  afterEach(() => {
    cleanup();
  });

  const mockAsset: Asset = {
    id: 1,
    org_id: 1,
    identifier: 'LAP-001',
    name: 'Engineering Laptop',
    type: 'device',
    description: 'Test laptop',
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    metadata: { location: 'Building A - Floor 2' },
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    deleted_at: null,
  };

  describe('Card variant', () => {
    it('renders asset information correctly', () => {
      render(<AssetCard asset={mockAsset} />);

      expect(screen.getByText('LAP-001')).toBeInTheDocument();
      expect(screen.getByText('Engineering Laptop')).toBeInTheDocument();
      expect(screen.getByText(/Building A - Floor 2/)).toBeInTheDocument();
      expect(screen.getByText('Active ✓')).toBeInTheDocument();
    });

    it('displays active status badge with correct styling', () => {
      const { container } = render(<AssetCard asset={mockAsset} />);
      const badge = screen.getByText('Active ✓');

      expect(badge).toBeInTheDocument();
      expect(badge.className).toContain('bg-green-50');
      expect(badge.className).toContain('text-green-700');
    });

    it('displays inactive status badge with correct styling', () => {
      const inactiveAsset = { ...mockAsset, is_active: false };
      render(<AssetCard asset={inactiveAsset} />);

      const badge = screen.getByText('Inactive');
      expect(badge).toBeInTheDocument();
      expect(badge.className).toContain('bg-gray-50');
      expect(badge.className).toContain('text-gray-700');
    });

    it('renders location when present in metadata', () => {
      render(<AssetCard asset={mockAsset} />);

      expect(screen.getByText(/Location:/)).toBeInTheDocument();
      expect(screen.getByText(/Building A - Floor 2/)).toBeInTheDocument();
    });

    it('does not render location when not present', () => {
      const assetWithoutLocation = { ...mockAsset, metadata: {} };
      render(<AssetCard asset={assetWithoutLocation} />);

      expect(screen.queryByText(/Location:/)).not.toBeInTheDocument();
    });

    it('shows action buttons when showActions is true', () => {
      render(<AssetCard asset={mockAsset} showActions={true} />);

      expect(screen.getByRole('button', { name: /Edit/ })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /Delete/ })).toBeInTheDocument();
    });

    it('hides action buttons when showActions is false', () => {
      render(<AssetCard asset={mockAsset} showActions={false} />);

      expect(screen.queryByRole('button', { name: /Edit/ })).not.toBeInTheDocument();
      expect(screen.queryByRole('button', { name: /Delete/ })).not.toBeInTheDocument();
    });

    it('calls onClick when card is clicked', () => {
      const handleClick = vi.fn();
      const { container } = render(<AssetCard asset={mockAsset} onClick={handleClick} />);

      const card = container.firstChild as HTMLElement;
      fireEvent.click(card);

      expect(handleClick).toHaveBeenCalledTimes(1);
    });

    it('calls onEdit when edit button is clicked', () => {
      const handleEdit = vi.fn();
      render(<AssetCard asset={mockAsset} onEdit={handleEdit} />);

      const editButton = screen.getByRole('button', { name: /Edit/ });
      fireEvent.click(editButton);

      expect(handleEdit).toHaveBeenCalledTimes(1);
      expect(handleEdit).toHaveBeenCalledWith(mockAsset);
    });

    it('calls onDelete when delete button is clicked', () => {
      const handleDelete = vi.fn();
      render(<AssetCard asset={mockAsset} onDelete={handleDelete} />);

      const deleteButton = screen.getByRole('button', { name: /Delete/ });
      fireEvent.click(deleteButton);

      expect(handleDelete).toHaveBeenCalledTimes(1);
      expect(handleDelete).toHaveBeenCalledWith(mockAsset);
    });

    it('stops event propagation when action buttons are clicked', () => {
      const handleClick = vi.fn();
      const handleEdit = vi.fn();
      render(<AssetCard asset={mockAsset} onClick={handleClick} onEdit={handleEdit} />);

      const editButton = screen.getByRole('button', { name: /Edit/ });
      fireEvent.click(editButton);

      expect(handleEdit).toHaveBeenCalledTimes(1);
      expect(handleClick).not.toHaveBeenCalled(); // Card click should not fire
    });

    it('renders correct icon for each asset type', () => {
      const types: Array<Asset['type']> = ['person', 'device', 'asset', 'inventory', 'other'];

      types.forEach((type) => {
        const { container } = render(<AssetCard asset={{ ...mockAsset, type }} />);
        const svg = container.querySelector('svg');
        expect(svg).toBeInTheDocument();
        cleanup();
      });
    });

    it('applies custom className', () => {
      const { container } = render(<AssetCard asset={mockAsset} className="custom-class" />);
      const card = container.firstChild as HTMLElement;

      expect(card.className).toContain('custom-class');
    });
  });

  describe('Row variant', () => {
    it('renders as table row with correct data', () => {
      render(
        <table>
          <tbody>
            <AssetCard asset={mockAsset} variant="row" />
          </tbody>
        </table>
      );

      expect(screen.getByText('LAP-001')).toBeInTheDocument();
      expect(screen.getByText('Engineering Laptop')).toBeInTheDocument();
      expect(screen.getByText('Active')).toBeInTheDocument();
      expect(screen.getByText(/device/i)).toBeInTheDocument();
    });

    it('renders location in table cell when present', () => {
      render(
        <table>
          <tbody>
            <AssetCard asset={mockAsset} variant="row" />
          </tbody>
        </table>
      );

      expect(screen.getByText('Building A - Floor 2')).toBeInTheDocument();
    });

    it('shows action buttons in row variant', () => {
      render(
        <table>
          <tbody>
            <AssetCard asset={mockAsset} variant="row" showActions={true} />
          </tbody>
        </table>
      );

      expect(screen.getByLabelText(`Edit ${mockAsset.identifier}`)).toBeInTheDocument();
      expect(screen.getByLabelText(`Delete ${mockAsset.identifier}`)).toBeInTheDocument();
    });

    it('calls onClick when row is clicked', () => {
      const handleClick = vi.fn();
      render(
        <table>
          <tbody>
            <AssetCard asset={mockAsset} variant="row" onClick={handleClick} />
          </tbody>
        </table>
      );

      const row = screen.getByText('LAP-001').closest('tr');
      fireEvent.click(row!);

      expect(handleClick).toHaveBeenCalledTimes(1);
    });

    it('applies custom className to row', () => {
      const { container } = render(
        <table>
          <tbody>
            <AssetCard asset={mockAsset} variant="row" className="custom-row-class" />
          </tbody>
        </table>
      );

      const row = container.querySelector('tr');
      expect(row?.className).toContain('custom-row-class');
    });
  });
});
