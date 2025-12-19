import { Radio } from 'lucide-react';
import type { TagIdentifier } from '@/types/shared';

interface TagCountBadgeProps {
  identifiers: TagIdentifier[] | undefined;
  onClick?: (e: React.MouseEvent) => void;
  expanded?: boolean;
}

/**
 * Displays a badge showing the count of RFID tags linked to an asset.
 * Can be static (row variant) or clickable (card variant for expand/collapse).
 */
export function TagCountBadge({ identifiers, onClick, expanded }: TagCountBadgeProps) {
  if (!identifiers || identifiers.length === 0) {
    return <span className="text-sm text-gray-400 dark:text-gray-500">-</span>;
  }

  const count = identifiers.length;
  const label = `${count} tag${count !== 1 ? 's' : ''}`;
  const baseTitle = `${count} RFID tag${count !== 1 ? 's' : ''} linked`;
  const title = onClick
    ? `${baseTitle} - click to ${expanded ? 'collapse' : 'expand'}`
    : baseTitle;

  const baseClasses = "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-400";
  const interactiveClasses = onClick
    ? "hover:bg-blue-100 dark:hover:bg-blue-900/40 transition-colors flex-shrink-0"
    : "";

  if (onClick) {
    return (
      <button
        onClick={onClick}
        className={`${baseClasses} ${interactiveClasses}`}
        title={title}
      >
        <Radio className="w-3 h-3" />
        {label}
      </button>
    );
  }

  return (
    <span className={baseClasses} title={title}>
      <Radio className="w-3 h-3" />
      {label}
    </span>
  );
}
