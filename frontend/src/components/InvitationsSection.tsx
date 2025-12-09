/**
 * InvitationsSection - Pending invitations table with actions
 * Features:
 * - Fetch invitations on mount
 * - Table: Email, Role, Invited By, Expires, Actions
 * - Cancel button per row
 * - Resend button per row
 * - "Invite Member" button opens InviteModal
 * - Empty state when no pending invitations
 */

import { useState, useEffect, useCallback } from 'react';
import { UserPlus, X, RefreshCw } from 'lucide-react';
import { orgsApi } from '@/lib/api/orgs';
import type { Invitation } from '@/types/org';
import { InviteModal } from './InviteModal';
import toast from 'react-hot-toast';

interface InvitationsSectionProps {
  orgId: number;
  isAdmin: boolean;
}

// Format expiry date with "Expires in X days" or "Expired"
const formatExpiry = (expiresAt: string) => {
  const now = new Date();
  const expiry = new Date(expiresAt);
  const diffDays = Math.ceil((expiry.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));
  if (diffDays < 0) return 'Expired';
  if (diffDays === 0) return 'Expires today';
  if (diffDays === 1) return 'Expires tomorrow';
  return `Expires in ${diffDays} days`;
};

export default function InvitationsSection({ orgId, isAdmin }: InvitationsSectionProps) {
  const [invitations, setInvitations] = useState<Invitation[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showInviteModal, setShowInviteModal] = useState(false);
  const [cancellingId, setCancellingId] = useState<number | null>(null);
  const [resendingId, setResendingId] = useState<number | null>(null);

  const fetchInvitations = useCallback(async () => {
    setIsLoading(true);
    setError(null);

    try {
      const response = await orgsApi.listInvitations(orgId);
      setInvitations(response.data.data);
    } catch (err: unknown) {
      const errorMessage = extractErrorMessage(err, 'Failed to load invitations');
      setError(errorMessage);
    } finally {
      setIsLoading(false);
    }
  }, [orgId]);

  useEffect(() => {
    fetchInvitations();
  }, [fetchInvitations]);

  const handleCancel = async (inviteId: number) => {
    if (cancellingId !== null) return;

    setCancellingId(inviteId);
    setError(null);

    try {
      await orgsApi.cancelInvitation(orgId, inviteId);
      setInvitations((prev) => prev.filter((inv) => inv.id !== inviteId));
      toast.success('Invitation cancelled');
    } catch (err: unknown) {
      const errorMessage = extractErrorMessage(err, 'Failed to cancel invitation');
      setError(errorMessage);
    } finally {
      setCancellingId(null);
    }
  };

  const handleResend = async (inviteId: number) => {
    if (resendingId !== null) return;

    setResendingId(inviteId);
    setError(null);

    try {
      await orgsApi.resendInvitation(orgId, inviteId);
      toast.success('Invitation resent');
      // Refresh to get updated expiry
      await fetchInvitations();
    } catch (err: unknown) {
      const errorMessage = extractErrorMessage(err, 'Failed to resend invitation');
      setError(errorMessage);
    } finally {
      setResendingId(null);
    }
  };

  const handleInviteSuccess = () => {
    toast.success('Invitation sent');
    fetchInvitations();
  };

  // Only show to admins
  if (!isAdmin) return null;

  return (
    <div className="mt-6 pt-6 border-t border-gray-700">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-medium text-white">Pending Invitations</h2>
        <button
          onClick={() => setShowInviteModal(true)}
          className="flex items-center gap-2 px-3 py-1.5 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors text-sm"
          data-testid="invite-member-button"
        >
          <UserPlus className="w-4 h-4" />
          Invite Member
        </button>
      </div>

      {/* Error */}
      {error && (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-4">
          <p className="text-red-400 text-sm">{error}</p>
        </div>
      )}

      {/* Loading */}
      {isLoading && (
        <div className="text-center py-4">
          <p className="text-gray-400">Loading invitations...</p>
        </div>
      )}

      {/* Invitations Table */}
      {!isLoading && invitations.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="text-left text-gray-400 text-sm border-b border-gray-700">
                <th className="pb-3 font-medium">Email</th>
                <th className="pb-3 font-medium">Role</th>
                <th className="pb-3 font-medium">Invited By</th>
                <th className="pb-3 font-medium">Expires</th>
                <th className="pb-3 font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {invitations.map((invitation) => {
                const isCancelling = cancellingId === invitation.id;
                const isResending = resendingId === invitation.id;
                const isExpired = new Date(invitation.expires_at) < new Date();

                return (
                  <tr
                    key={invitation.id}
                    className="border-b border-gray-700/50 text-gray-200"
                  >
                    {/* Email */}
                    <td className="py-3">{invitation.email}</td>

                    {/* Role */}
                    <td className="py-3 capitalize">{invitation.role}</td>

                    {/* Invited By */}
                    <td className="py-3 text-gray-400">
                      {invitation.invited_by?.name || 'Unknown'}
                    </td>

                    {/* Expires */}
                    <td className={`py-3 ${isExpired ? 'text-red-400' : 'text-gray-400'}`}>
                      {formatExpiry(invitation.expires_at)}
                    </td>

                    {/* Actions */}
                    <td className="py-3">
                      <div className="flex items-center gap-2">
                        {/* Resend */}
                        <button
                          onClick={() => handleResend(invitation.id)}
                          disabled={isResending || isCancelling}
                          className="text-blue-400 hover:text-blue-300 disabled:opacity-50 disabled:cursor-not-allowed p-1"
                          title="Resend invitation"
                          data-testid={`resend-invite-${invitation.id}`}
                        >
                          {isResending ? (
                            <span className="text-xs">...</span>
                          ) : (
                            <RefreshCw className="w-4 h-4" />
                          )}
                        </button>

                        {/* Cancel */}
                        <button
                          onClick={() => handleCancel(invitation.id)}
                          disabled={isCancelling || isResending}
                          className="text-red-400 hover:text-red-300 disabled:opacity-50 disabled:cursor-not-allowed p-1"
                          title="Cancel invitation"
                          data-testid={`cancel-invite-${invitation.id}`}
                        >
                          {isCancelling ? (
                            <span className="text-xs">...</span>
                          ) : (
                            <X className="w-4 h-4" />
                          )}
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Empty State */}
      {!isLoading && invitations.length === 0 && !error && (
        <div className="text-center py-4">
          <p className="text-gray-500 text-sm">No pending invitations</p>
        </div>
      )}

      {/* Invite Modal */}
      {showInviteModal && (
        <InviteModal
          orgId={orgId}
          onClose={() => setShowInviteModal(false)}
          onSuccess={handleInviteSuccess}
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
