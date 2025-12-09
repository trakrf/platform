/**
 * OrgSwitcher - Dropdown menu to switch between organizations
 */

import { Menu } from '@headlessui/react';
import { ChevronDown, Building2, Plus, Check, Settings, Users } from 'lucide-react';
import { useOrgStore } from '@/stores';
import { RoleBadge } from './RoleBadge';

interface OrgSwitcherProps {
  onCreateOrg: () => void;
}

export function OrgSwitcher({ onCreateOrg }: OrgSwitcherProps) {
  const { currentOrg, currentRole, orgs, isLoading, switchOrg } = useOrgStore();

  const handleSwitchOrg = async (orgId: number) => {
    if (orgId === currentOrg?.id) return;
    try {
      await switchOrg(orgId);
    } catch (error) {
      console.error('Failed to switch org:', error);
    }
  };

  if (!currentOrg) {
    return (
      <button
        onClick={onCreateOrg}
        className="flex items-center gap-2 px-3 py-2 rounded-lg text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
      >
        <Building2 className="w-5 h-5" />
        <span className="text-sm">No organization</span>
      </button>
    );
  }

  return (
    <Menu as="div" className="relative inline-block text-left">
      <Menu.Button
        disabled={isLoading}
        className="flex items-center gap-2 px-3 py-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors disabled:opacity-50"
      >
        <Building2 className="w-5 h-5 text-gray-600 dark:text-gray-400" />
        <span className="text-sm font-medium text-gray-900 dark:text-gray-100 max-w-[150px] truncate">
          {currentOrg.name}
        </span>
        {currentRole && <RoleBadge role={currentRole} />}
        <ChevronDown className="w-4 h-4 text-gray-500 dark:text-gray-400" />
      </Menu.Button>

      <Menu.Items className="absolute left-0 mt-2 w-64 origin-top-left divide-y divide-gray-100 dark:divide-gray-700 rounded-lg bg-white dark:bg-gray-800 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none z-50">
        <div className="p-1">
          <div className="px-3 py-2 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">
            Organizations
          </div>
          {orgs.map((org) => (
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
                  {org.id === currentOrg.id && (
                    <Check className="w-4 h-4 text-blue-600" />
                  )}
                </button>
              )}
            </Menu.Item>
          ))}
        </div>
        <div className="p-1">
          <Menu.Item>
            {({ active }) => (
              <button
                onClick={onCreateOrg}
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
                <a
                  href="#org-settings"
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
                >
                  <Settings className="w-4 h-4" />
                  Organization Settings
                </a>
              )}
            </Menu.Item>
            <Menu.Item>
              {({ active }) => (
                <a
                  href="#org-members"
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
                >
                  <Users className="w-4 h-4" />
                  Members
                </a>
              )}
            </Menu.Item>
          </div>
        )}
      </Menu.Items>
    </Menu>
  );
}
