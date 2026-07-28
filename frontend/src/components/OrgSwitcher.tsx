/**
 * OrgSwitcher - Dropdown menu to switch between organizations and user actions
 */

import { useState } from 'react';
import { Menu } from '@headlessui/react';
import { ChevronDown, Plus, Check, Settings, Users, Key, LogOut, Building2, Webhook } from 'lucide-react';
import { useOrgStore, useAuthStore } from '@/stores';
import { useOrgSwitch } from '@/hooks/orgs/useOrgSwitch';
import { RoleBadge } from './RoleBadge';
import { OrgModal } from './OrgModal';
import type { User } from '@/lib/api/auth';

interface OrgSwitcherProps {
  user?: User;
  onLogout?: () => void;
}

function getFirstLetter(email: string): string {
  return email.charAt(0).toUpperCase();
}

export function OrgSwitcher({ user, onLogout }: OrgSwitcherProps) {
  const { currentOrg, currentRole, orgs, isLoading } = useOrgStore();
  const isSuperadmin = useAuthStore((s) => s.profile?.is_superadmin ?? false);
  const { switchOrg } = useOrgSwitch();
  const [showCreateModal, setShowCreateModal] = useState(false);

  const handleSwitchOrg = async (orgId: number) => {
    if (orgId === currentOrg?.id) return;
    try {
      await switchOrg(orgId);
    } catch (error) {
      console.error('Failed to switch org:', error);
    }
  };

  const avatarLetter = user ? getFirstLetter(user.email) : null;

  if (!currentOrg && !user) {
    return (
      <>
        <button
          onClick={() => setShowCreateModal(true)}
          className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-md text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors text-sm"
        >
          <span>No organization</span>
        </button>
        <OrgModal isOpen={showCreateModal} onClose={() => setShowCreateModal(false)} />
      </>
    );
  }

  return (
    <Menu as="div" className="relative inline-block text-left">
      <Menu.Button
        disabled={isLoading}
        data-testid="org-switcher"
        data-current-org-id={currentOrg?.id ?? ''}
        data-current-org-name={currentOrg?.name ?? ''}
        aria-label="Account menu"
        className="flex items-center gap-1.5 px-2 py-1.5 rounded-md hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors disabled:opacity-50"
      >
        {avatarLetter ? (
          <div className="flex items-center justify-center w-8 h-8 rounded-full bg-blue-600 text-white font-semibold text-sm">
            {avatarLetter}
          </div>
        ) : null}
        <ChevronDown className="w-4 h-4 text-gray-400 dark:text-gray-500" />
      </Menu.Button>

      <Menu.Items className="absolute right-0 mt-2 w-64 origin-top-right divide-y divide-gray-100 dark:divide-gray-700 rounded-lg bg-white dark:bg-gray-800 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none z-50">
        {user && (
          <div className="px-3 py-2">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">{user.email}</span>
              {currentRole && <RoleBadge role={currentRole} />}
            </div>
          </div>
        )}
        <div className="p-1">
          <div className="px-3 py-2 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">
            Organizations
          </div>
          {(orgs ?? []).map(org => (
            <Menu.Item key={org.id}>
              {({ active }) => (
                <button
                  onClick={() => handleSwitchOrg(org.id)}
                  disabled={isLoading}
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } group flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors disabled:opacity-50`}
                >
                  <span className="truncate">{org.name}</span>
                  {currentOrg && org.id === currentOrg.id && <Check className="w-4 h-4 text-blue-600" />}
                </button>
              )}
            </Menu.Item>
          ))}
        </div>
        <div className="p-1">
          <Menu.Item>
            {({ active }) => (
              <button
                onClick={() => setShowCreateModal(true)}
                className={`${
                  active ? 'bg-gray-100 dark:bg-gray-700' : ''
                } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
              >
                <Plus className="w-4 h-4" />
                Create Organization
              </button>
            )}
          </Menu.Item>
        </div>
        {isSuperadmin && (
          <div className="p-1">
            <Menu.Item>
              {({ active }) => (
                <button
                  onClick={() => {
                    window.location.hash = '#admin-orgs';
                  }}
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
                >
                  <Building2 className="w-4 h-4" />
                  All Organizations
                </button>
              )}
            </Menu.Item>
          </div>
        )}
        {/* TRA-1058: Settings and Members go to the hash screens. They used to
            open OrgModal's manage mode, a parallel org-admin surface that never
            received the controls built for the first — three tickets shipped
            features nobody could click to.

            What a viewer finds there still depends on who they are: a
            superadmin gets the entitlement editor (TRA-949) and capability
            grants (TRA-1027); a regular org admin gets the read-only
            subscription status (TRA-975) and no capability surface at all —
            a read-only view of an org's own grants does not exist yet. */}
        {currentRole && ['owner', 'admin'].includes(currentRole) && (
          <div className="p-1">
            <Menu.Item>
              {({ active }) => (
                <button
                  onClick={() => {
                    window.location.hash = '#org-settings';
                  }}
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
                >
                  <Settings className="w-4 h-4" />
                  Organization Settings
                </button>
              )}
            </Menu.Item>
            <Menu.Item>
              {({ active }) => (
                <button
                  onClick={() => {
                    window.location.hash = '#org-members';
                  }}
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
                >
                  <Users className="w-4 h-4" />
                  Members
                </button>
              )}
            </Menu.Item>
            <Menu.Item>
              {({ active }) => (
                <button
                  onClick={() => {
                    window.location.hash = '#api-keys';
                  }}
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
                >
                  <Key className="w-4 h-4" />
                  API Keys
                </button>
              )}
            </Menu.Item>
            {/* TRA-1043: webhooks sits next to API Keys because it is the same
                kind of thing — an outbound integration surface, admin-scoped —
                and both are base platform surface, never capability-gated. */}
            <Menu.Item>
              {({ active }) => (
                <button
                  onClick={() => {
                    window.location.hash = '#webhooks';
                  }}
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
                >
                  <Webhook className="w-4 h-4" />
                  Webhooks
                </button>
              )}
            </Menu.Item>
          </div>
        )}
        {onLogout && (
          <div className="p-1">
            <Menu.Item>
              {({ active }) => (
                <button
                  onClick={onLogout}
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
                >
                  <LogOut className="w-4 h-4" />
                  Logout
                </button>
              )}
            </Menu.Item>
          </div>
        )}
      </Menu.Items>

      <OrgModal isOpen={showCreateModal} onClose={() => setShowCreateModal(false)} />
    </Menu>
  );
}
