import { Radio } from 'lucide-react';
import type { TagIdentifier } from '@/types/shared';

interface TagCountBadgeProps {
  identifiers: TagIdentifier[] | undefined;
  onClick?: (e: React.MouseEvent) => void;
}

/**
 * Displays a clickable badge showing the count of RFID tags linked to an asset.
 * Shows just the count with an icon - clicking opens the tag identifiers modal.
 */
export function TagCountBadge({ identifiers, onClick }: TagCountBadgeProps) {
  if (!identifiers || identifiers.length === 0) {
    return <span className="text-sm text-gray-400 dark:text-gray-500">-</span>;
  }

  const count = identifiers.length;
  const title = `${count} RFID tag${count !== 1 ? 's' : ''} linked - click to view`;

  const baseClasses =
    'inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-400';
  const interactiveClasses = onClick
    ? 'hover:bg-blue-100 dark:hover:bg-blue-900/40 transition-colors cursor-pointer'
    : '';

  if (onClick) {
    return (
      <button
        onClick={onClick}
        className={`${baseClasses} ${interactiveClasses}`}
        title={title}
      >
        <Radio className="w-3 h-3" />
        <span className="font-semibold">{count}</span>
      </button>
    );
  }

  return (
    <span className={`${baseClasses} ${interactiveClasses}`} title={title}>
      <Radio className="w-3 h-3" />
      <span className="font-semibold">{count}</span>
    </span>
  );
}
