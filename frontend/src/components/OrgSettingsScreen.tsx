/**
 * OrgSettingsScreen - Organization settings management
 * Features:
 * - Edit org name (admin only)
 * - Delete organization with confirmation (admin only)
 */

import { useState, useEffect, useRef } from 'react';
import { ArrowLeft } from 'lucide-react';
import { useOrgStore, useAuthStore } from '@/stores';
import { orgsApi } from '@/lib/api/orgs';
import { DeleteOrgModal } from './DeleteOrgModal';
import toast from 'react-hot-toast';

export default function OrgSettingsScreen() {
  const { currentOrg, currentRole } = useOrgStore();
  const { fetchProfile } = useAuthStore();
  const [name, setName] = useState('');
  const [originalName, setOriginalName] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const nameInputRef = useRef<HTMLInputElement>(null);

  const isAdmin = currentRole === 'owner' || currentRole === 'admin';
  const hasChanges = name !== originalName;

  // Initialize name from current org
  useEffect(() => {
    if (currentOrg) {
      setName(currentOrg.name);
      setOriginalName(currentOrg.name);
    }
  }, [currentOrg]);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!currentOrg || !hasChanges || isSaving) return;

    setError(null);
    setIsSaving(true);

    try {
      await orgsApi.update(currentOrg.id, { name });
      // Refresh profile to get updated org data
      await fetchProfile();
      setOriginalName(name);
      toast.success('Organization name updated');
    } catch (err: unknown) {
      const errorMessage = extractErrorMessage(err, 'Failed to update organization');
      setError(errorMessage);
    } finally {
      setIsSaving(false);
    }
  };

  const handleDelete = async (confirmName: string) => {
    if (!currentOrg || isDeleting) return;

    setIsDeleting(true);

    try {
      await orgsApi.delete(currentOrg.id, confirmName);
      // Refresh profile to update org list
      await fetchProfile();
      toast.success('Organization deleted');
      // Redirect to home
      window.location.hash = '#home';
    } catch (err: unknown) {
      const errorMessage = extractErrorMessage(err, 'Failed to delete organization');
      setError(errorMessage);
      setShowDeleteModal(false);
    } finally {
      setIsDeleting(false);
    }
  };

  // No org selected
  if (!currentOrg) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <h1 className="text-2xl font-semibold text-white mb-4">
            No Organization Selected
          </h1>
          <p className="text-gray-400 mb-6">
            Please select an organization to view settings.
          </p>
          <a
            href="#home"
            className="inline-flex items-center gap-2 text-blue-400 hover:text-blue-300"
          >
            <ArrowLeft className="w-4 h-4" />
            Go Home
          </a>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md">
        {/* Header */}
        <div className="flex items-center gap-4 mb-6">
          <a
            href="#home"
            className="text-gray-400 hover:text-gray-300 transition-colors"
          >
            <ArrowLeft className="w-5 h-5" />
          </a>
          <h1 className="text-2xl font-semibold text-white">
            Organization Settings
          </h1>
        </div>

        {/* General Error */}
        {error && (
          <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-6">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        {/* Settings Form */}
        <form onSubmit={handleSave} className="space-y-6">
          {/* Org Name */}
          <div>
            <label
              htmlFor="org-name"
              className="block text-sm font-medium text-gray-300 mb-2"
            >
              Organization Name
            </label>
            <input
              ref={nameInputRef}
              id="org-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Organization name"
              className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500 disabled:opacity-50"
              disabled={!isAdmin || isSaving}
            />
            {!isAdmin && (
              <p className="text-gray-500 text-sm mt-1">
                Only admins can edit the organization name.
              </p>
            )}
          </div>

          {/* Save Button */}
          {isAdmin && (
            <button
              type="submit"
              disabled={!hasChanges || isSaving}
              className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSaving ? 'Saving...' : 'Save Changes'}
            </button>
          )}
        </form>

        {/* Danger Zone */}
        {isAdmin && (
          <div className="mt-8 pt-6 border-t border-gray-700">
            <h2 className="text-lg font-semibold text-red-400 mb-2">
              Danger Zone
            </h2>
            <p className="text-gray-400 text-sm mb-4">
              Once you delete an organization, there is no going back. Please be
              certain.
            </p>
            <button
              type="button"
              onClick={() => setShowDeleteModal(true)}
              className="w-full bg-red-600/20 hover:bg-red-600/30 text-red-400 border border-red-600/50 py-2 px-4 rounded-lg font-medium transition-colors"
            >
              Delete Organization
            </button>
          </div>
        )}
      </div>

      {/* Delete Modal */}
      {showDeleteModal && (
        <DeleteOrgModal
          orgName={currentOrg.name}
          onConfirm={handleDelete}
          onCancel={() => setShowDeleteModal(false)}
          isLoading={isDeleting}
        />
      )}
    </div>
  );
}

// Helper to extract error message from API responses
function extractErrorMessage(err: unknown, fallback: string): string {
  const data = (err as { response?: { data?: Record<string, unknown> } })
    .response?.data;
  const errorObj = (data?.error as Record<string, unknown>) || data;
  let message =
    (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) ||
    (typeof errorObj?.title === 'string' && errorObj.title.trim()) ||
    (typeof data?.error === 'string' && data.error.trim()) ||
    (typeof (err as Error).message === 'string' &&
      (err as Error).message.trim()) ||
    fallback;

  if (typeof message !== 'string') {
    message = JSON.stringify(message);
  }

  return message;
}
