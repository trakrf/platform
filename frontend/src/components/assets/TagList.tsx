import { useState } from 'react';
import { Radio, HelpCircle, Target, Trash2 } from 'lucide-react';
import type { Tag } from '@/types/shared';
import { assetsApi } from '@/lib/api/assets';
import { locationsApi } from '@/lib/api/locations';
import toast from 'react-hot-toast';

interface TagListProps {
  tags: Tag[];
  expanded?: boolean;
  size?: 'sm' | 'md';
  showHeader?: boolean;
  className?: string;
  entityId?: number;
  entityType?: 'asset' | 'location';
  onTagRemoved?: (tagId: number) => void;
}

export function TagList({
  tags,
  expanded = true,
  size = 'sm',
  showHeader = false,
  className = '',
  entityId,
  entityType,
  onTagRemoved,
}: TagListProps) {
  if (!expanded || tags.length === 0) {
    if (showHeader) {
      return (
        <div className={className}>
          <TagHeader />
          <p className="text-sm text-gray-500 dark:text-gray-400 italic">
            No RFID tags linked
          </p>
        </div>
      );
    }
    return null;
  }

  const spacing = size === 'md' ? 'space-y-2' : 'space-y-1.5';

  const canDelete = entityId !== undefined && entityType !== undefined && onTagRemoved !== undefined;

  return (
    <div className={className}>
      {showHeader && <TagHeader />}
      <div className={spacing}>
        {tags.map((tag) => (
          <TagRow
            key={tag.surrogate_id}
            tag={tag}
            size={size}
            entityId={entityId}
            entityType={entityType}
            onDelete={canDelete ? onTagRemoved : undefined}
          />
        ))}
      </div>
    </div>
  );
}

function TagHeader() {
  return (
    <div className="flex items-center gap-2 mb-3">
      <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
        RFID Tags
      </h3>
      <div className="group relative">
        <HelpCircle className="w-4 h-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 cursor-help" />
        <div className="absolute left-0 bottom-full mb-2 hidden group-hover:block w-64 p-2 bg-gray-900 dark:bg-gray-700 text-white text-xs rounded-lg shadow-lg z-10">
          <p className="font-medium mb-1">What are RFID Tags?</p>
          <p>Physical RFID tags attached to this asset for scanning and tracking. Each tag has a unique number.</p>
        </div>
      </div>
    </div>
  );
}

interface TagRowProps {
  tag: Tag;
  size?: 'sm' | 'md';
  entityId?: number;
  entityType?: 'asset' | 'location';
  onDelete?: (tagId: number) => void;
}

function getTypeLabel(type: string): string {
  switch (type.toLowerCase()) {
    case 'rfid':
      return 'RFID';
    case 'ble':
      return 'BLE';
    case 'barcode':
      return 'Barcode';
    default:
      return type.toUpperCase();
  }
}

export function TagRow({ tag, size = 'sm', entityId, entityType, onDelete }: TagRowProps) {
  const [isDeleting, setIsDeleting] = useState(false);
  const isSmall = size === 'sm';

  const handleLocate = () => {
    window.location.hash = `#locate?epc=${encodeURIComponent(tag.value)}`;
  };

  const handleDelete = async () => {
    if (!entityId || !entityType || !onDelete) return;

    setIsDeleting(true);
    try {
      if (entityType === 'asset') {
        await assetsApi.removeTag(entityId, tag.surrogate_id);
      } else {
        await locationsApi.removeTag(entityId, tag.surrogate_id);
      }
      toast.success('Tag removed');
      onDelete(tag.surrogate_id);
    } catch (err) {
      console.error('Failed to remove tag:', err);
      toast.error('Failed to remove tag');
    } finally {
      setIsDeleting(false);
    }
  };

  const containerClasses = isSmall
    ? 'flex items-center justify-between bg-gray-50 dark:bg-gray-900 rounded px-2 py-1.5 gap-2'
    : 'flex items-center justify-between bg-gray-50 dark:bg-gray-900 rounded-lg px-3 py-2 gap-3';

  const iconClasses = isSmall
    ? 'w-3 h-3 text-blue-500 flex-shrink-0'
    : 'w-4 h-4 text-blue-500 flex-shrink-0';

  const textClasses = isSmall
    ? 'text-xs font-mono text-gray-700 dark:text-gray-300 truncate'
    : 'text-sm font-mono text-gray-900 dark:text-gray-100 truncate';

  const typeBadgeClasses = isSmall
    ? 'text-xs px-1.5 py-0.5 rounded bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 font-medium flex-shrink-0'
    : 'text-xs px-2 py-0.5 rounded bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 font-medium flex-shrink-0';

  const statusBadgeClasses = isSmall
    ? `text-xs px-1.5 py-0.5 rounded flex-shrink-0 ${
        tag.is_active
          ? 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
          : 'bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-400'
      }`
    : `inline-flex items-center px-2 py-0.5 rounded text-xs font-medium flex-shrink-0 ${
        tag.is_active
          ? 'bg-green-100 dark:bg-green-900/30 text-green-800 dark:text-green-300'
          : 'bg-gray-100 dark:bg-gray-700 text-gray-800 dark:text-gray-300'
      }`;

  const locateButtonClasses = isSmall
    ? `p-1 rounded transition-colors flex-shrink-0 ${
        tag.is_active
          ? 'text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20'
          : 'text-gray-300 dark:text-gray-600 cursor-not-allowed'
      }`
    : `p-1.5 rounded transition-colors flex-shrink-0 ${
        tag.is_active
          ? 'text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20'
          : 'text-gray-300 dark:text-gray-600 cursor-not-allowed'
      }`;

  return (
    <div className={containerClasses}>
      <div className="flex items-center gap-1.5 min-w-0 flex-1">
        <Radio className={iconClasses} />
        <span className={typeBadgeClasses}>{getTypeLabel(tag.tag_type)}</span>
        <span className={textClasses} title={tag.value}>
          {tag.value}
        </span>
      </div>
      <div className="flex items-center gap-1.5">
        <span className={statusBadgeClasses}>
          {tag.is_active ? 'Active' : 'Inactive'}
        </span>
        <button
          onClick={handleLocate}
          disabled={!tag.is_active}
          className={locateButtonClasses}
          aria-label={`Locate tag ${tag.value}`}
          title={tag.is_active ? 'Locate this tag' : 'Tag is inactive'}
        >
          <Target className={isSmall ? 'w-3.5 h-3.5' : 'w-4 h-4'} />
        </button>
        {onDelete && (
          <button
            onClick={handleDelete}
            disabled={isDeleting}
            className={`${isSmall ? 'p-1' : 'p-1.5'} rounded transition-colors flex-shrink-0 text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20 disabled:opacity-50`}
            aria-label={`Remove tag ${tag.value}`}
            title="Remove this tag"
          >
            <Trash2 className={isSmall ? 'w-3.5 h-3.5' : 'w-4 h-4'} />
          </button>
        )}
      </div>
    </div>
  );
}
