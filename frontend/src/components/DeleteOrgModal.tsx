/**
 * DeleteOrgModal - Confirmation modal for deleting an organization
 * Requires typing the org name exactly to confirm deletion
 */

import { useState } from 'react';

interface DeleteOrgModalProps {
  orgName: string;
  onConfirm: (confirmName: string) => void;
  onCancel: () => void;
  isLoading?: boolean;
}

export function DeleteOrgModal({
  orgName,
  onConfirm,
  onCancel,
  isLoading = false,
}: DeleteOrgModalProps) {
  const [confirmName, setConfirmName] = useState('');
  const canDelete = confirmName === orgName;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (canDelete && !isLoading) {
      onConfirm(confirmName);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black bg-opacity-50"
        onClick={onCancel}
        data-testid="delete-org-modal-backdrop"
      />

      {/* Modal */}
      <div className="relative bg-gray-800 rounded-lg shadow-xl p-6 max-w-md w-full mx-4">
        <h3 className="text-lg font-semibold text-white mb-2">
          Delete Organization
        </h3>

        <p className="text-gray-400 mb-4">
          This action cannot be undone. All members will be removed and data
          will be permanently deleted.
        </p>

        <form onSubmit={handleSubmit}>
          <label className="block text-sm text-gray-300 mb-2">
            Type{' '}
            <span className="font-mono font-bold text-red-400">{orgName}</span>{' '}
            to confirm
          </label>

          <input
            type="text"
            value={confirmName}
            onChange={(e) => setConfirmName(e.target.value)}
            placeholder="Organization name"
            className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg mb-4 focus:ring-2 focus:ring-red-500 focus:border-red-500"
            disabled={isLoading}
            autoFocus
            data-testid="delete-org-confirm-input"
          />

          <div className="flex gap-3 justify-end">
            <button
              type="button"
              onClick={onCancel}
              disabled={isLoading}
              className="px-4 py-2 text-gray-300 hover:text-white transition-colors disabled:opacity-50"
              data-testid="delete-org-cancel-button"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!canDelete || isLoading}
              className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              data-testid="delete-org-confirm-button"
            >
              {isLoading ? 'Deleting...' : 'Delete Organization'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
