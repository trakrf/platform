/**
 * MembersScreen - Organization member management
 * Features:
 * - View all members in a table
 * - "You" badge on current user's row
 * - Role dropdown to change roles (admin only)
 * - Remove member button (admin only)
 * - Backend enforces last-admin protection
 */

import { useState, useEffect, useCallback } from 'react';
import { ArrowLeft, Trash2 } from 'lucide-react';
import { useOrgStore, useAuthStore } from '@/stores';
import { orgsApi } from '@/lib/api/orgs';
import type { OrgMember, OrgRole } from '@/types/org';
import InvitationsSection from './InvitationsSection';
import toast from 'react-hot-toast';

const ROLES: OrgRole[] = ['owner', 'admin', 'manager', 'operator', 'viewer'];

export default function MembersScreen() {
  const { currentOrg, currentRole } = useOrgStore();
  const { profile, fetchProfile } = useAuthStore();
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [updatingUserId, setUpdatingUserId] = useState<number | null>(null);
  const [removingUserId, setRemovingUserId] = useState<number | null>(null);

  const isAdmin = currentRole === 'owner' || currentRole === 'admin';
  const currentUserId = profile?.id;

  const fetchMembers = useCallback(async () => {
    if (!currentOrg) return;

    setIsLoading(true);
    setError(null);

    try {
      const response = await orgsApi.listMembers(currentOrg.id);
      setMembers(response.data.data);
    } catch (err: unknown) {
      const errorMessage = extractErrorMessage(err, 'Failed to load members');
      setError(errorMessage);
    } finally {
      setIsLoading(false);
    }
  }, [currentOrg]);

  // Fetch members on mount / org change
  useEffect(() => {
    if (currentOrg) {
      fetchMembers();
    }
  }, [currentOrg, fetchMembers]);

  const handleRoleChange = async (userId: number, newRole: OrgRole) => {
    if (!currentOrg || updatingUserId !== null) return;

    setUpdatingUserId(userId);
    setError(null);

    try {
      await orgsApi.updateMemberRole(currentOrg.id, userId, newRole);
      // Update local state
      setMembers((prev) =>
        prev.map((m) => (m.user_id === userId ? { ...m, role: newRole } : m))
      );
      // Refresh profile in case it was the current user
      if (userId === currentUserId) {
        await fetchProfile();
      }
      toast.success('Member role updated');
    } catch (err: unknown) {
      const errorMessage = extractErrorMessage(err, 'Failed to update role');
      setError(errorMessage);
      // Refresh to get correct state
      await fetchMembers();
    } finally {
      setUpdatingUserId(null);
    }
  };

  const handleRemoveMember = async (userId: number) => {
    if (!currentOrg || removingUserId !== null) return;

    // Don't allow removing yourself
    if (userId === currentUserId) {
      setError('You cannot remove yourself from the organization');
      return;
    }

    setRemovingUserId(userId);
    setError(null);

    try {
      await orgsApi.removeMember(currentOrg.id, userId);
      // Update local state
      setMembers((prev) => prev.filter((m) => m.user_id !== userId));
      toast.success('Member removed');
    } catch (err: unknown) {
      const errorMessage = extractErrorMessage(err, 'Failed to remove member');
      setError(errorMessage);
    } finally {
      setRemovingUserId(null);
    }
  };

  // Format date for display
  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
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
            Please select an organization to view members.
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
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-4xl">
        {/* Header */}
        <div className="flex items-center gap-4 mb-6">
          <a
            href="#home"
            className="text-gray-400 hover:text-gray-300 transition-colors"
          >
            <ArrowLeft className="w-5 h-5" />
          </a>
          <h1 className="text-2xl font-semibold text-white">Members</h1>
          <span className="text-gray-500">
            {currentOrg.name}
          </span>
        </div>

        {/* Error */}
        {error && (
          <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-6">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        {/* Loading */}
        {isLoading && (
          <div className="text-center py-8">
            <p className="text-gray-400">Loading members...</p>
          </div>
        )}

        {/* Members Table */}
        {!isLoading && members.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="text-left text-gray-400 text-sm border-b border-gray-700">
                  <th className="pb-3 font-medium">Name</th>
                  <th className="pb-3 font-medium">Email</th>
                  <th className="pb-3 font-medium">Role</th>
                  <th className="pb-3 font-medium">Joined</th>
                  {isAdmin && <th className="pb-3 font-medium">Actions</th>}
                </tr>
              </thead>
              <tbody>
                {members.map((member) => {
                  const isCurrentUser = member.user_id === currentUserId;
                  const isUpdating = updatingUserId === member.user_id;
                  const isRemoving = removingUserId === member.user_id;

                  return (
                    <tr
                      key={member.user_id}
                      className="border-b border-gray-700/50 text-gray-200"
                    >
                      {/* Name */}
                      <td className="py-4">
                        <span className="flex items-center gap-2">
                          {member.name}
                          {isCurrentUser && (
                            <span className="px-2 py-0.5 text-xs bg-blue-600/20 text-blue-400 rounded-full">
                              You
                            </span>
                          )}
                        </span>
                      </td>

                      {/* Email */}
                      <td className="py-4 text-gray-400">{member.email}</td>

                      {/* Role */}
                      <td className="py-4">
                        {isAdmin ? (
                          <select
                            value={member.role}
                            onChange={(e) =>
                              handleRoleChange(
                                member.user_id,
                                e.target.value as OrgRole
                              )
                            }
                            disabled={isUpdating || isRemoving}
                            className="bg-gray-700 border border-gray-600 text-gray-200 text-sm rounded px-2 py-1 focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
                          >
                            {ROLES.map((role) => (
                              <option key={role} value={role}>
                                {role.charAt(0).toUpperCase() + role.slice(1)}
                              </option>
                            ))}
                          </select>
                        ) : (
                          <span className="capitalize">{member.role}</span>
                        )}
                      </td>

                      {/* Joined */}
                      <td className="py-4 text-gray-400">
                        {formatDate(member.joined_at)}
                      </td>

                      {/* Actions */}
                      {isAdmin && (
                        <td className="py-4">
                          {!isCurrentUser && (
                            <button
                              onClick={() => handleRemoveMember(member.user_id)}
                              disabled={isRemoving || isUpdating}
                              className="text-red-400 hover:text-red-300 disabled:opacity-50 disabled:cursor-not-allowed p-1"
                              title="Remove member"
                            >
                              {isRemoving ? (
                                <span className="text-xs">...</span>
                              ) : (
                                <Trash2 className="w-4 h-4" />
                              )}
                            </button>
                          )}
                        </td>
                      )}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}

        {/* Empty State */}
        {!isLoading && members.length === 0 && !error && (
          <div className="text-center py-8">
            <p className="text-gray-400">No members found.</p>
          </div>
        )}

        {/* Invitations Section */}
        {isAdmin && currentOrg && (
          <InvitationsSection orgId={currentOrg.id} isAdmin={isAdmin} />
        )}
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
