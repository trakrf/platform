import { Radio } from 'lucide-react';
import type { TagIdentifier } from '@/types/shared';

interface TagIdentifierListProps {
  identifiers: TagIdentifier[];
  expanded: boolean;
  className?: string;
}

/**
 * Displays an expandable list of tag identifiers with icon, value, and status.
 * Used in AssetCard card variant to show full tag details when expanded.
 */
export function TagIdentifierList({ identifiers, expanded, className = '' }: TagIdentifierListProps) {
  if (!expanded || identifiers.length === 0) {
    return null;
  }

  return (
    <div className={`space-y-1.5 ${className}`}>
      {identifiers.map((identifier) => (
        <TagIdentifierRow key={identifier.id} identifier={identifier} />
      ))}
    </div>
  );
}

interface TagIdentifierRowProps {
  identifier: TagIdentifier;
}

/**
 * Single row displaying a tag identifier with icon, value, and status badge.
 */
export function TagIdentifierRow({ identifier }: TagIdentifierRowProps) {
  return (
    <div className="flex items-center justify-between bg-gray-50 dark:bg-gray-900 rounded px-2 py-1">
      <div className="flex items-center gap-1.5 min-w-0">
        <Radio className="w-3 h-3 text-blue-500 flex-shrink-0" />
        <span
          className="text-xs font-mono text-gray-700 dark:text-gray-300 truncate"
          title={identifier.value}
        >
          {identifier.value}
        </span>
      </div>
      <span
        className={`ml-2 text-xs px-1.5 py-0.5 rounded flex-shrink-0 ${
          identifier.is_active
            ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
            : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400'
        }`}
      >
        {identifier.is_active ? 'Active' : 'Inactive'}
      </span>
    </div>
  );
}
