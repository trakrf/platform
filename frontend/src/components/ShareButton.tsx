/**
 * ShareButton - Dropdown button for multi-format export options
 */

import { Menu } from '@headlessui/react';
import { Share2, ChevronDown } from 'lucide-react';
import type { ExportFormat } from '@/types/export';
import { getFormatOptions } from '@/utils/exportFormats';

interface ShareButtonProps {
  onFormatSelect: (format: ExportFormat) => void;
  disabled?: boolean;
  className?: string;
  iconOnly?: boolean;
}

export function ShareButton({ onFormatSelect, disabled = false, className = '', iconOnly = false }: ShareButtonProps) {
  const formatOptions = getFormatOptions();

  const baseButtonClass = iconOnly
    ? "h-[42px] w-[42px] bg-blue-600 hover:bg-blue-700 text-white rounded-lg disabled:opacity-50 disabled:cursor-not-allowed transition-colors inline-flex items-center justify-center"
    : "h-[42px] px-3 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-medium disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center text-sm";

  return (
    <Menu as="div" className="relative inline-block text-left">
      <Menu.Button 
        disabled={disabled}
        className={`${baseButtonClass} ${className}`}
        title={iconOnly ? "Share" : undefined}
      >
        <Share2 className={iconOnly ? "w-4 h-4" : "w-4 h-4 mr-1.5"} />
        {!iconOnly && (
          <>
            <span>Share</span>
            <ChevronDown className="w-3 h-3 ml-1" />
          </>
        )}
      </Menu.Button>

      <Menu.Items className="absolute right-0 mt-2 w-56 origin-top-right divide-y divide-gray-100 dark:divide-gray-700 rounded-lg bg-white dark:bg-gray-800 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none z-50">
        <div className="p-1">
          {formatOptions.map((option) => {
            const Icon = option.icon;
            return (
              <Menu.Item key={option.id}>
                {({ active }) => (
                  <button
                    onClick={() => onFormatSelect(option.id)}
                    className={`${
                      active ? 'bg-gray-100 dark:bg-gray-700' : ''
                    } group flex w-full items-center rounded-md px-3 py-2 text-sm transition-colors`}
                  >
                    <Icon className="mr-3 h-4 w-4 text-gray-500 dark:text-gray-400" />
                    <div className="flex-1 text-left">
                      <div className="font-medium text-gray-900 dark:text-gray-100">
                        {option.shortLabel}
                      </div>
                      <div className="text-xs text-gray-500 dark:text-gray-400">
                        {option.description}
                      </div>
                    </div>
                  </button>
                )}
              </Menu.Item>
            );
          })}
        </div>
      </Menu.Items>
    </Menu>
  );
}