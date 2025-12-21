import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import '@testing-library/jest-dom';
import { TagIdentifiersModal } from './TagIdentifiersModal';
import { assetsApi } from '@/lib/api/assets';
import type { TagIdentifier } from '@/types/shared';

// Mock the assets API
vi.mock('@/lib/api/assets', () => ({
  assetsApi: {
    removeIdentifier: vi.fn(),
  },
}));

// Mock react-hot-toast
vi.mock('react-hot-toast', () => ({
  default: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe('TagIdentifiersModal', () => {
  const mockIdentifiers: TagIdentifier[] = [
    { id: 1, type: 'rfid', value: 'TAG-001', is_active: true },
    { id: 2, type: 'rfid', value: 'TAG-002', is_active: true },
    { id: 3, type: 'rfid', value: 'TAG-003', is_active: false },
  ];

  const defaultProps = {
    identifiers: mockIdentifiers,
    isOpen: true,
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
    // Clean up any portals left in the document body
    document.body.innerHTML = '';
  });

  describe('rendering', () => {
    it('renders nothing when isOpen is false', () => {
      render(<TagIdentifiersModal {...defaultProps} isOpen={false} />);

      expect(screen.queryByText('Tag Identifiers')).not.toBeInTheDocument();
    });

    it('renders modal when isOpen is true', () => {
      render(<TagIdentifiersModal {...defaultProps} />);

      expect(screen.getByText('Tag Identifiers')).toBeInTheDocument();
    });

    it('renders in a portal (attached to document.body)', () => {
      render(<TagIdentifiersModal {...defaultProps} />);

      // The modal should be a direct child of body due to portal
      const backdrop = document.querySelector('[aria-hidden="true"]');
      expect(backdrop?.parentElement).toBe(document.body);
    });

    it('displays entity name when provided', () => {
      render(<TagIdentifiersModal {...defaultProps} entityName="LAPTOP-001" />);

      expect(screen.getByText('LAPTOP-001')).toBeInTheDocument();
    });

    it('displays all identifiers', () => {
      render(<TagIdentifiersModal {...defaultProps} />);

      expect(screen.getByText('TAG-001')).toBeInTheDocument();
      expect(screen.getByText('TAG-002')).toBeInTheDocument();
      expect(screen.getByText('TAG-003')).toBeInTheDocument();
    });

    it('displays RFID badge for each identifier', () => {
      render(<TagIdentifiersModal {...defaultProps} />);

      const rfidBadges = screen.getAllByText('RFID');
      expect(rfidBadges).toHaveLength(3);
    });

    it('displays Active/Inactive status badges', () => {
      render(<TagIdentifiersModal {...defaultProps} />);

      const activeBadges = screen.getAllByText('Active');
      const inactiveBadges = screen.getAllByText('Inactive');

      expect(activeBadges).toHaveLength(2);
      expect(inactiveBadges).toHaveLength(1);
    });

    it('displays empty state when no identifiers', () => {
      render(<TagIdentifiersModal {...defaultProps} identifiers={[]} />);

      expect(
        screen.getByText('No tag identifiers linked to this asset.')
      ).toBeInTheDocument();
    });
  });

  describe('close functionality', () => {
    it('calls onClose when X close button is clicked', () => {
      const onClose = vi.fn();
      render(<TagIdentifiersModal {...defaultProps} onClose={onClose} />);

      fireEvent.click(screen.getByLabelText('Close modal'));

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('calls onClose when footer Close button is clicked', () => {
      const onClose = vi.fn();
      render(<TagIdentifiersModal {...defaultProps} onClose={onClose} />);

      // Get the Close button in the footer (not the X button)
      const closeButtons = screen.getAllByRole('button');
      const footerCloseButton = closeButtons.find(
        (btn) => btn.textContent === 'Close'
      );
      if (footerCloseButton) {
        fireEvent.click(footerCloseButton);
      }

      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('calls onClose when backdrop is clicked', () => {
      const onClose = vi.fn();
      render(<TagIdentifiersModal {...defaultProps} onClose={onClose} />);

      // Click the backdrop
      const backdrop = document.querySelector('[aria-hidden="true"]');
      if (backdrop) {
        fireEvent.click(backdrop);
      }

      expect(onClose).toHaveBeenCalledTimes(1);
    });
  });

  describe('remove functionality', () => {
    it('does not show remove buttons when entityId is not provided', () => {
      render(<TagIdentifiersModal {...defaultProps} />);

      expect(
        screen.queryByLabelText('Remove tag identifier')
      ).not.toBeInTheDocument();
    });

    it('does not show remove buttons when onIdentifierRemoved is not provided', () => {
      render(<TagIdentifiersModal {...defaultProps} entityId={1} />);

      expect(
        screen.queryByLabelText('Remove tag identifier')
      ).not.toBeInTheDocument();
    });

    it('shows remove buttons when both entityId and onIdentifierRemoved are provided', () => {
      render(
        <TagIdentifiersModal
          {...defaultProps}
          entityId={1}
          onIdentifierRemoved={vi.fn()}
        />
      );

      const removeButtons = screen.getAllByLabelText('Remove tag identifier');
      expect(removeButtons).toHaveLength(3);
    });

    it('shows confirmation buttons when remove is clicked', () => {
      render(
        <TagIdentifiersModal
          {...defaultProps}
          entityId={1}
          onIdentifierRemoved={vi.fn()}
        />
      );

      // Click the first remove button
      const removeButtons = screen.getAllByLabelText('Remove tag identifier');
      fireEvent.click(removeButtons[0]);

      // Should show Cancel and Remove confirmation buttons
      expect(screen.getByText('Cancel')).toBeInTheDocument();
      // Find the confirmation Remove button (not the one in the trash buttons)
      const allButtons = screen.getAllByRole('button');
      const confirmRemoveButton = allButtons.find(
        (btn) => btn.textContent?.trim() === 'Remove'
      );
      expect(confirmRemoveButton).toBeDefined();
    });

    it('hides confirmation when Cancel is clicked', () => {
      render(
        <TagIdentifiersModal
          {...defaultProps}
          entityId={1}
          onIdentifierRemoved={vi.fn()}
        />
      );

      // Click remove, then cancel
      const removeButtons = screen.getAllByLabelText('Remove tag identifier');
      fireEvent.click(removeButtons[0]);
      fireEvent.click(screen.getByText('Cancel'));

      // Confirmation should be hidden
      expect(screen.queryByText('Cancel')).not.toBeInTheDocument();
    });

    it('calls API and callback when Remove is confirmed', async () => {
      const onIdentifierRemoved = vi.fn();
      vi.mocked(assetsApi.removeIdentifier).mockResolvedValue({
        data: { deleted: true },
      } as any);

      render(
        <TagIdentifiersModal
          {...defaultProps}
          entityId={5}
          entityType="asset"
          onIdentifierRemoved={onIdentifierRemoved}
        />
      );

      // Click remove button for first identifier (id: 1)
      const removeButtons = screen.getAllByLabelText('Remove tag identifier');
      fireEvent.click(removeButtons[0]);

      // Find and click the confirmation Remove button
      const allButtons = screen.getAllByRole('button');
      const confirmRemoveButton = allButtons.find(
        (btn) => btn.textContent?.trim() === 'Remove'
      );
      if (confirmRemoveButton) {
        fireEvent.click(confirmRemoveButton);
      }

      await waitFor(() => {
        expect(assetsApi.removeIdentifier).toHaveBeenCalledWith(5, 1);
        expect(onIdentifierRemoved).toHaveBeenCalledWith(1);
      });
    });

    it('shows error toast when removal fails', async () => {
      const toast = await import('react-hot-toast');
      vi.mocked(assetsApi.removeIdentifier).mockRejectedValue(
        new Error('Network error')
      );

      render(
        <TagIdentifiersModal
          {...defaultProps}
          entityId={1}
          entityType="asset"
          onIdentifierRemoved={vi.fn()}
        />
      );

      // Click remove and confirm
      const removeButtons = screen.getAllByLabelText('Remove tag identifier');
      fireEvent.click(removeButtons[0]);

      const allButtons = screen.getAllByRole('button');
      const confirmRemoveButton = allButtons.find(
        (btn) => btn.textContent?.trim() === 'Remove'
      );
      if (confirmRemoveButton) {
        fireEvent.click(confirmRemoveButton);
      }

      await waitFor(() => {
        expect(toast.default.error).toHaveBeenCalledWith(
          'Failed to remove tag identifier'
        );
      });
    });
  });
});
