import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { ConfirmModal } from './ConfirmModal';

const base = {
  isOpen: true,
  title: 'Delete Thing',
  message: 'Sure?',
  confirmText: 'Delete',
};

describe('ConfirmModal', () => {
  afterEach(() => cleanup());

  it('confirms and cancels when idle', () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    render(<ConfirmModal {...base} onConfirm={onConfirm} onCancel={onCancel} />);
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    fireEvent.click(screen.getByTestId('modal-backdrop'));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  // A slow confirm (a delete that first drives an output off) must show it is
  // working and must not take a second click while it does.
  describe('while pending', () => {
    it('shows the pending text with a spinner on the confirm button', () => {
      render(
        <ConfirmModal {...base} pending pendingText="Deleting…" onConfirm={vi.fn()} onCancel={vi.fn()} />
      );
      const confirm = screen.getByTestId('modal-confirm-button');
      expect(confirm).toHaveTextContent('Deleting…');
      expect(confirm).toBeDisabled();
      expect(screen.getByTestId('modal-confirm-spinner')).toBeInTheDocument();
      expect(screen.getByRole('dialog')).toHaveAttribute('aria-busy', 'true');
    });

    it('ignores confirm, cancel and backdrop clicks', () => {
      const onConfirm = vi.fn();
      const onCancel = vi.fn();
      render(<ConfirmModal {...base} pending onConfirm={onConfirm} onCancel={onCancel} />);
      fireEvent.click(screen.getByTestId('modal-confirm-button'));
      fireEvent.click(screen.getByTestId('modal-cancel-button'));
      fireEvent.click(screen.getByTestId('modal-backdrop'));
      expect(onConfirm).not.toHaveBeenCalled();
      expect(onCancel).not.toHaveBeenCalled();
    });

    it('falls back to a generic pending label', () => {
      render(<ConfirmModal {...base} pending onConfirm={vi.fn()} onCancel={vi.fn()} />);
      expect(screen.getByTestId('modal-confirm-button')).toHaveTextContent('Working…');
    });
  });
});
