/**
 * UserMenu - Dropdown menu with user info and logout
 */

import { Menu } from '@headlessui/react';
import { ChevronDown } from 'lucide-react';
import { Avatar } from './Avatar';
import type { User } from '@/lib/api/auth';

interface UserMenuProps {
  user: User;
  onLogout: () => void;
}

export function UserMenu({ user, onLogout }: UserMenuProps) {
  return (
    <Menu as="div" className="relative inline-block text-left">
      <Menu.Button className="flex items-center gap-2 px-3 py-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
        <Avatar email={user.email} />
        <span className="hidden sm:inline-block text-sm font-medium text-gray-900 dark:text-gray-100">
          {user.email}
        </span>
        <ChevronDown className="w-4 h-4 text-gray-500 dark:text-gray-400" />
      </Menu.Button>

      <Menu.Items className="absolute right-0 mt-2 w-48 origin-top-right divide-y divide-gray-100 dark:divide-gray-700 rounded-lg bg-white dark:bg-gray-800 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none z-50">
        <div className="p-1">
          <Menu.Item>
            {({ active }) => (
              <button
                onClick={onLogout}
                className={`${
                  active ? 'bg-gray-100 dark:bg-gray-700' : ''
                } group flex w-full items-center rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
              >
                Logout
              </button>
            )}
          </Menu.Item>
        </div>
      </Menu.Items>
    </Menu>
  );
}
