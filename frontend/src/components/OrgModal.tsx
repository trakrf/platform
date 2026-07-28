/**
 * OrgModal - Create Organization dialog.
 *
 * TRA-1058 removed this modal's 'manage' mode. Members, org name, entitlement,
 * capabilities and the danger zone live on #org-members / #org-settings — the
 * one org-admin surface — and OrgSwitcher links there directly.
 *
 * Create stays a modal rather than folding into the #create-org screen: this
 * one goes through useOrgSwitch().createOrg, which mints a token for the new
 * org and clears org-scoped caches. CreateOrgScreen calls the bare store
 * action and does neither, so the two are not at parity (see TRA-1058 scope
 * note); collapsing them is its own change.
 */

import { X } from 'lucide-react';
import { useOrgModal } from './useOrgModal';

interface OrgModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function OrgModal({ isOpen, onClose }: OrgModalProps) {
  const {
    handleBackdropClick,
    newOrgName,
    setNewOrgName,
    createError,
    createNameError,
    isCreating,
    nameInputRef,
    handleCreateOrg,
    handleCreateNameBlur,
  } = useOrgModal({ isOpen, onClose });

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50 p-4"
      onClick={handleBackdropClick}
    >
      <div className="relative w-full max-w-md bg-white dark:bg-gray-900 rounded-lg shadow-xl">
        <div className="border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex items-center justify-between">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">Create Organization</h2>
          <button
            onClick={onClose}
            disabled={isCreating}
            className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg transition-colors disabled:opacity-50"
            aria-label="Close modal"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="px-6 py-6">
          <form onSubmit={handleCreateOrg} className="space-y-4">
            <div>
              <label htmlFor="new-org-name" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                Organization Name
              </label>
              <input
                ref={nameInputRef}
                id="new-org-name"
                type="text"
                value={newOrgName}
                onChange={e => setNewOrgName(e.target.value)}
                onBlur={handleCreateNameBlur}
                placeholder="My Organization"
                className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                disabled={isCreating}
              />
              {createNameError && <p className="text-red-500 text-sm mt-1">{createNameError}</p>}
            </div>

            {createError && (
              <div className="bg-red-900/20 border border-red-800 rounded-lg p-3">
                <p className="text-red-400 text-sm">{createError}</p>
              </div>
            )}

            <div className="flex gap-3 justify-end pt-2">
              <button
                type="button"
                onClick={onClose}
                disabled={isCreating}
                className="px-4 py-2 text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={isCreating}
                className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isCreating ? 'Creating...' : 'Create'}
              </button>
            </div>
          </form>

          <p className="text-gray-500 dark:text-gray-400 text-sm mt-4 text-center">
            You will be the owner of this organization and can invite others to join.
          </p>
        </div>
      </div>
    </div>
  );
}
