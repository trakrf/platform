/**
 * OrgSwitcher - Dropdown menu to switch between organizations and user actions
 */

import { useState } from 'react';
import { Menu } from '@headlessui/react';
import { ChevronDown, Plus, Check, Settings, Users, LogOut } from 'lucide-react';
import { useOrgStore } from '@/stores';
import { useOrgSwitch } from '@/hooks/orgs/useOrgSwitch';
import { RoleBadge } from './RoleBadge';
import { OrgModal } from './OrgModal';
import type { ModalMode, TabType } from './useOrgModal';
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
  const { switchOrg } = useOrgSwitch();
  const [showModal, setShowModal] = useState(false);
  const [modalMode, setModalMode] = useState<ModalMode>('manage');
  const [modalTab, setModalTab] = useState<TabType>('members');

  const handleSwitchOrg = async (orgId: number) => {
    if (orgId === currentOrg?.id) return;
    try {
      await switchOrg(orgId);
    } catch (error) {
      console.error('Failed to switch org:', error);
    }
  };

  const openModal = (mode: ModalMode, tab: TabType = 'members') => {
    setModalMode(mode);
    setModalTab(tab);
    setShowModal(true);
  };

  const avatarLetter = user ? getFirstLetter(user.email) : null;

  if (!currentOrg && !user) {
    return (
      <>
        <button
          onClick={() => openModal('create')}
          className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-md text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors text-sm"
        >
          <span>No organization</span>
        </button>
        <OrgModal isOpen={showModal} onClose={() => setShowModal(false)} mode={modalMode} defaultTab={modalTab} />
      </>
    );
  }

  return (
    <Menu as="div" className="relative inline-block text-left">
      <Menu.Button
        disabled={isLoading}
        data-testid="org-switcher"
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
                onClick={() => openModal('create')}
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
        {currentRole && ['owner', 'admin'].includes(currentRole) && (
          <div className="p-1">
            <Menu.Item>
              {({ active }) => (
                <button
                  onClick={() => openModal('manage', 'settings')}
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
                  onClick={() => openModal('manage', 'members')}
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
                >
                  <Users className="w-4 h-4" />
                  Members
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

      <OrgModal isOpen={showModal} onClose={() => setShowModal(false)} mode={modalMode} defaultTab={modalTab} />
    </Menu>
  );
}
