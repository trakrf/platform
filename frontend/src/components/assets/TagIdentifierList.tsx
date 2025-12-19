import { Radio, HelpCircle } from 'lucide-react';
import type { TagIdentifier } from '@/types/shared';

interface TagIdentifierListProps {
  identifiers: TagIdentifier[];
  expanded?: boolean;
  size?: 'sm' | 'md';
  showHeader?: boolean;
  className?: string;
}

/**
 * Displays a list of tag identifiers with optional header and help text.
 * Supports two sizes: 'sm' for compact card view, 'md' for modal/detail view.
 */
export function TagIdentifierList({
  identifiers,
  expanded = true,
  size = 'sm',
  showHeader = false,
  className = '',
}: TagIdentifierListProps) {
  if (!expanded || identifiers.length === 0) {
    if (showHeader) {
      return (
        <div className={className}>
          <TagIdentifierHeader />
          <p className="text-sm text-gray-500 dark:text-gray-400 italic">
            No tag identifiers linked
          </p>
        </div>
      );
    }
    return null;
  }

  const spacing = size === 'md' ? 'space-y-2' : 'space-y-1.5';

  return (
    <div className={className}>
      {showHeader && <TagIdentifierHeader />}
      <div className={spacing}>
        {identifiers.map((identifier) => (
          <TagIdentifierRow key={identifier.id} identifier={identifier} size={size} />
        ))}
      </div>
    </div>
  );
}

/**
 * Header with title and help tooltip explaining tag identifiers.
 */
function TagIdentifierHeader() {
  return (
    <div className="flex items-center gap-2 mb-3">
      <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
        Tag Identifiers
      </h3>
      <div className="group relative">
        <HelpCircle className="w-4 h-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 cursor-help" />
        <div className="absolute left-0 bottom-full mb-2 hidden group-hover:block w-64 p-2 bg-gray-900 dark:bg-gray-700 text-white text-xs rounded-lg shadow-lg z-10">
          <p className="font-medium mb-1">What are Tag Identifiers?</p>
          <p>RFID tags, barcodes, or BLE beacons physically attached to this asset for scanning and tracking.</p>
        </div>
      </div>
    </div>
  );
}

interface TagIdentifierRowProps {
  identifier: TagIdentifier;
  size?: 'sm' | 'md';
}

/**
 * Single row displaying a tag identifier with icon, value, and status badge.
 * - sm: Compact for card views (smaller text, tighter padding)
 * - md: Comfortable for modals (larger text, more padding)
 */
export function TagIdentifierRow({ identifier, size = 'sm' }: TagIdentifierRowProps) {
  const isSmall = size === 'sm';

  const containerClasses = isSmall
    ? 'flex items-center justify-between bg-gray-50 dark:bg-gray-900 rounded px-2 py-1'
    : 'flex items-center justify-between bg-gray-50 dark:bg-gray-900 rounded-lg px-3 py-2';

  const iconClasses = isSmall
    ? 'w-3 h-3 text-blue-500 flex-shrink-0'
    : 'w-4 h-4 text-blue-500 flex-shrink-0';

  const textClasses = isSmall
    ? 'text-xs font-mono text-gray-700 dark:text-gray-300 truncate'
    : 'text-sm font-mono text-gray-900 dark:text-gray-100 truncate';

  const badgeClasses = isSmall
    ? `ml-2 text-xs px-1.5 py-0.5 rounded flex-shrink-0 ${
        identifier.is_active
          ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
          : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400'
      }`
    : `ml-2 inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium flex-shrink-0 ${
        identifier.is_active
          ? 'bg-green-100 dark:bg-green-900/30 text-green-800 dark:text-green-300'
          : 'bg-gray-100 dark:bg-gray-700 text-gray-800 dark:text-gray-300'
      }`;

  return (
    <div className={containerClasses}>
      <div className="flex items-center gap-1.5 min-w-0">
        <Radio className={iconClasses} />
        <span className={textClasses} title={identifier.value}>
          {identifier.value}
        </span>
      </div>
      <span className={badgeClasses}>
        {identifier.is_active ? 'Active' : 'Inactive'}
      </span>
    </div>
  );
}
