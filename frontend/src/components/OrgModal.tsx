/**
 * OrgModal - Unified modal for organization management
 * Modes: 'create' (new org) | 'manage' (members/settings tabs)
 */

import { X, Users, Settings, Trash2 } from 'lucide-react';
import { DeleteOrgModal } from './DeleteOrgModal';
import { RoleBadge } from './RoleBadge';
import InvitationsSection from './InvitationsSection';
import { useOrgModal, ROLES, type ModalMode, type TabType } from './useOrgModal';
import type { OrgRole } from '@/types/org';

interface OrgModalProps {
  isOpen: boolean;
  onClose: () => void;
  mode?: ModalMode;
  defaultTab?: TabType;
}

const formatDate = (dateString: string) =>
  new Date(dateString).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });

export function OrgModal({ isOpen, onClose, mode = 'manage', defaultTab = 'members' }: OrgModalProps) {
  const {
    currentOrg,
    currentRole,
    isAdmin,
    handleBackdropClick,
    // Manage mode
    activeTab,
    members,
    isLoadingMembers,
    membersError,
    updatingUserId,
    removingUserId,
    orgName,
    setOrgName,
    isSaving,
    isDeleting,
    showDeleteModal,
    settingsError,
    currentUserId,
    hasNameChanges,
    isNameValid,
    handleTabChange,
    handleRoleChange,
    handleRemoveMember,
    handleSaveSettings,
    handleDeleteOrg,
    openDeleteModal,
    closeDeleteModal,
    // Create mode
    newOrgName,
    setNewOrgName,
    createError,
    createNameError,
    isCreating,
    nameInputRef,
    handleCreateOrg,
    handleCreateNameBlur,
  } = useOrgModal({ isOpen, onClose, mode, defaultTab });

  if (!isOpen) return null;

  // Create mode - no currentOrg required
  if (mode === 'create') {
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

  // Manage mode - requires currentOrg
  if (!currentOrg) return null;

  return (
    <>
      <div
        className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50 p-4"
        onClick={handleBackdropClick}
      >
        <div className="relative w-full max-w-3xl bg-white dark:bg-gray-900 rounded-lg shadow-xl max-h-[85vh] overflow-hidden flex flex-col">
          {/* Header */}
          <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex items-center justify-between z-10">
            <div className="flex items-center gap-3">
              <h2 className="text-xl font-semibold text-gray-900 dark:text-white">{currentOrg.name}</h2>
              {currentRole && <RoleBadge role={currentRole} />}
            </div>
            <button
              onClick={onClose}
              className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg transition-colors"
              aria-label="Close modal"
            >
              <X className="h-5 w-5" />
            </button>
          </div>

          {/* Tabs */}
          <div className="border-b border-gray-200 dark:border-gray-700 px-6">
            <nav className="flex gap-4" aria-label="Tabs">
              <button
                onClick={() => handleTabChange('members')}
                className={`py-3 px-1 border-b-2 font-medium text-sm transition-colors ${
                  activeTab === 'members'
                    ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                    : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
                }`}
              >
                <span className="flex items-center gap-2">
                  <Users className="w-4 h-4" />
                  Members
                </span>
              </button>
              {isAdmin && (
                <button
                  onClick={() => handleTabChange('settings')}
                  className={`py-3 px-1 border-b-2 font-medium text-sm transition-colors ${
                    activeTab === 'settings'
                      ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                      : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
                  }`}
                >
                  <span className="flex items-center gap-2">
                    <Settings className="w-4 h-4" />
                    Settings
                  </span>
                </button>
              )}
            </nav>
          </div>

          {/* Content */}
          <div className="flex-1 overflow-y-auto p-6">
            {activeTab === 'members' && (
              <div>
                {membersError && (
                  <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-4">
                    <p className="text-red-400 text-sm">{membersError}</p>
                  </div>
                )}

                {isLoadingMembers && (
                  <div className="text-center py-8">
                    <p className="text-gray-400">Loading members...</p>
                  </div>
                )}

                {!isLoadingMembers && members.length > 0 && (
                  <div className="overflow-x-auto">
                    <table className="w-full">
                      <thead>
                        <tr className="text-left text-gray-500 dark:text-gray-400 text-sm border-b border-gray-200 dark:border-gray-700">
                          <th className="pb-3 font-medium">Name</th>
                          <th className="pb-3 font-medium">Email</th>
                          <th className="pb-3 font-medium">Role</th>
                          <th className="pb-3 font-medium">Joined</th>
                          {isAdmin && <th className="pb-3 font-medium">Actions</th>}
                        </tr>
                      </thead>
                      <tbody>
                        {members.map(member => (
                          <MemberRow
                            key={member.user_id}
                            member={member}
                            isCurrentUser={member.user_id === currentUserId}
                            isUpdating={updatingUserId === member.user_id}
                            isRemoving={removingUserId === member.user_id}
                            isAdmin={isAdmin}
                            onRoleChange={handleRoleChange}
                            onRemove={handleRemoveMember}
                          />
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}

                {!isLoadingMembers && members.length === 0 && !membersError && (
                  <div className="text-center py-8">
                    <p className="text-gray-400">No members found.</p>
                  </div>
                )}

                <InvitationsSection orgId={currentOrg.id} isAdmin={isAdmin} />
              </div>
            )}

            {activeTab === 'settings' && isAdmin && (
              <div>
                {settingsError && (
                  <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-6">
                    <p className="text-red-400 text-sm">{settingsError}</p>
                  </div>
                )}

                <form onSubmit={handleSaveSettings} className="space-y-6">
                  <div>
                    <label htmlFor="org-name" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                      Organization Name
                    </label>
                    <input
                      id="org-name"
                      type="text"
                      value={orgName}
                      onChange={e => setOrgName(e.target.value)}
                      placeholder="Organization name"
                      className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500 disabled:opacity-50"
                      disabled={isSaving}
                    />
                  </div>
                  <button
                    type="submit"
                    disabled={!hasNameChanges || !isNameValid || isSaving}
                    className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {isSaving ? 'Saving...' : 'Save Changes'}
                  </button>
                </form>

                <div className="mt-8 pt-6 border-t border-gray-200 dark:border-gray-700">
                  <h3 className="text-lg font-semibold text-red-500 mb-2">Danger Zone</h3>
                  <p className="text-gray-500 dark:text-gray-400 text-sm mb-4">
                    Once you delete an organization, there is no going back. Please be certain.
                  </p>
                  <button
                    type="button"
                    onClick={openDeleteModal}
                    className="px-4 py-2 bg-red-600/20 hover:bg-red-600/30 text-red-500 border border-red-600/50 rounded-lg font-medium transition-colors"
                  >
                    Delete Organization
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      {showDeleteModal && (
        <DeleteOrgModal
          orgName={currentOrg.name}
          onConfirm={handleDeleteOrg}
          onCancel={closeDeleteModal}
          isLoading={isDeleting}
        />
      )}
    </>
  );
}

// Sub-component for member table rows
interface MemberRowProps {
  member: { user_id: number; name: string; email: string; role: string; joined_at: string };
  isCurrentUser: boolean;
  isUpdating: boolean;
  isRemoving: boolean;
  isAdmin: boolean;
  onRoleChange: (userId: number, role: OrgRole) => void;
  onRemove: (userId: number) => void;
}

function MemberRow({ member, isCurrentUser, isUpdating, isRemoving, isAdmin, onRoleChange, onRemove }: MemberRowProps) {
  return (
    <tr className="border-b border-gray-100 dark:border-gray-700/50 text-gray-900 dark:text-gray-200">
      <td className="py-3">
        <span className="flex items-center gap-2">
          {member.name}
          {isCurrentUser && (
            <span className="px-2 py-0.5 text-xs bg-blue-600/20 text-blue-600 dark:text-blue-400 rounded-full">You</span>
          )}
        </span>
      </td>
      <td className="py-3 text-gray-500 dark:text-gray-400">{member.email}</td>
      <td className="py-3">
        {isAdmin ? (
          <select
            value={member.role}
            onChange={e => onRoleChange(member.user_id, e.target.value as OrgRole)}
            disabled={isUpdating || isRemoving}
            className="bg-gray-100 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 text-gray-900 dark:text-gray-200 text-sm rounded px-2 py-1 focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
          >
            {ROLES.map(role => (
              <option key={role} value={role}>
                {role.charAt(0).toUpperCase() + role.slice(1)}
              </option>
            ))}
          </select>
        ) : (
          <span className="capitalize">{member.role}</span>
        )}
      </td>
      <td className="py-3 text-gray-500 dark:text-gray-400">{formatDate(member.joined_at)}</td>
      {isAdmin && (
        <td className="py-3">
          {!isCurrentUser && (
            <button
              onClick={() => onRemove(member.user_id)}
              disabled={isRemoving || isUpdating}
              className="text-red-500 hover:text-red-400 disabled:opacity-50 disabled:cursor-not-allowed p-1"
              title="Remove member"
            >
              {isRemoving ? <span className="text-xs">...</span> : <Trash2 className="w-4 h-4" />}
            </button>
          )}
        </td>
      )}
    </tr>
  );
}
