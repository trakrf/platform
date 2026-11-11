import React from 'react';
import { Loader2 } from 'lucide-react';

interface ConfirmModalProps {
  isOpen: boolean;
  onConfirm: () => void;
  onCancel: () => void;
  title: string;
  message: string;
  confirmText?: string;
  /** The confirmed action is running: show progress and take no further clicks. */
  pending?: boolean;
  /** Confirm-button label while pending. */
  pendingText?: string;
}

export const ConfirmModal: React.FC<ConfirmModalProps> = ({
  isOpen,
  onConfirm,
  onCancel,
  title,
  message,
  confirmText = 'Confirm',
  pending = false,
  pendingText = 'Working…',
}) => {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black bg-opacity-50"
        onClick={pending ? undefined : onCancel}
        data-testid="modal-backdrop"
      />

      {/* Modal */}
      <div
        role="dialog"
        aria-busy={pending}
        className="relative bg-white rounded-lg shadow-xl p-6 max-w-sm w-full mx-4"
      >
        <h3 className="text-lg font-semibold mb-2">{title}</h3>
        <p className="text-gray-600 mb-6">{message}</p>

        <div className="flex gap-3 justify-end">
          <button
            onClick={onCancel}
            disabled={pending}
            className="px-4 py-2 text-gray-600 hover:text-gray-800 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            data-testid="modal-cancel-button"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={pending}
            className="inline-flex items-center gap-2 px-4 py-2 bg-red-500 text-white rounded hover:bg-red-600 transition-colors disabled:opacity-75 disabled:cursor-wait"
            data-testid="modal-confirm-button"
          >
            {pending && (
              <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" data-testid="modal-confirm-spinner" />
            )}
            {pending ? pendingText : confirmText}
          </button>
        </div>
      </div>
    </div>
  );
};
