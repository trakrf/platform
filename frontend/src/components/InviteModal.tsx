/**
 * InviteModal - Modal for sending organization invitations
 * Features:
 * - Email input with basic format validation
 * - Role dropdown (admin, manager, operator, viewer - NO owner)
 * - Send button with loading state
 * - Error display for API errors
 */

import { useState } from 'react';
import { orgsApi } from '@/lib/api/orgs';
import type { OrgRole } from '@/types/org';

interface InviteModalProps {
  orgId: number;
  onClose: () => void;
  onSuccess: () => void;
}

// Exclude 'owner' - must be promoted after joining
const INVITE_ROLES: OrgRole[] = ['admin', 'manager', 'operator', 'viewer'];

// Basic email validation
const isValidEmail = (email: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);

export function InviteModal({ orgId, onClose, onSuccess }: InviteModalProps) {
  const [email, setEmail] = useState('');
  const [role, setRole] = useState<OrgRole>('viewer');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canSend = email.trim() && isValidEmail(email) && !isLoading;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSend) return;

    setIsLoading(true);
    setError(null);

    try {
      await orgsApi.createInvitation(orgId, email.trim(), role);
      onSuccess();
      onClose();
    } catch (err: unknown) {
      const errorMessage = extractErrorMessage(err, 'Failed to send invitation');
      setError(errorMessage);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black bg-opacity-50"
        onClick={onClose}
        data-testid="invite-modal-backdrop"
      />

      {/* Modal */}
      <div className="relative bg-gray-800 rounded-lg shadow-xl p-6 max-w-md w-full mx-4">
        <h3 className="text-lg font-semibold text-white mb-4">
          Invite Member
        </h3>

        <form onSubmit={handleSubmit}>
          {/* Email Input */}
          <label className="block text-sm text-gray-300 mb-2">
            Email address
          </label>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="colleague@example.com"
            className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg mb-4 focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
            disabled={isLoading}
            autoFocus
            data-testid="invite-email-input"
          />

          {/* Role Dropdown */}
          <label className="block text-sm text-gray-300 mb-2">
            Role
          </label>
          <select
            value={role}
            onChange={(e) => setRole(e.target.value as OrgRole)}
            disabled={isLoading}
            className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg mb-4 focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
            data-testid="invite-role-select"
          >
            {INVITE_ROLES.map((r) => (
              <option key={r} value={r}>
                {r.charAt(0).toUpperCase() + r.slice(1)}
              </option>
            ))}
          </select>

          {/* Error Display */}
          {error && (
            <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-4">
              <p className="text-red-400 text-sm">{error}</p>
            </div>
          )}

          {/* Actions */}
          <div className="flex gap-3 justify-end">
            <button
              type="button"
              onClick={onClose}
              disabled={isLoading}
              className="px-4 py-2 text-gray-300 hover:text-white transition-colors disabled:opacity-50"
              data-testid="invite-cancel-button"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!canSend}
              className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              data-testid="invite-send-button"
            >
              {isLoading ? 'Sending...' : 'Send Invitation'}
            </button>
          </div>
        </form>
      </div>
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
