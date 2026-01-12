import { Trash2 } from 'lucide-react';

type TagType = 'rfid';

interface TagIdentifierInputRowProps {
  type: TagType;
  value: string;
  onTypeChange: (type: TagType) => void;
  onValueChange: (value: string) => void;
  onRemove?: () => void;
  disabled?: boolean;
  error?: string;
}

/**
 * Input row for a single RFID tag number.
 */
export function TagIdentifierInputRow({
  value,
  onValueChange,
  onRemove,
  disabled = false,
  error,
}: TagIdentifierInputRowProps) {
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        {/* Static RFID label - only tag type supported */}
        <span className="w-16 px-2 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 bg-gray-100 dark:bg-gray-700 rounded-lg text-center">
          RFID
        </span>

        {/* Value input */}
        <input
          type="text"
          value={value}
          onChange={(e) => onValueChange(e.target.value)}
          disabled={disabled}
          placeholder="Enter tag number..."
          className={`flex-1 px-3 py-2 text-sm border rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50 ${
            error
              ? 'border-red-500 focus:ring-red-500'
              : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
          }`}
        />

        {/* Remove button */}
        {onRemove && (
          <button
            type="button"
            onClick={onRemove}
            disabled={disabled}
            className="p-2 text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20 rounded-lg transition-colors disabled:opacity-50"
            aria-label="Remove tag"
          >
            <Trash2 className="w-4 h-4" />
          </button>
        )}
      </div>

      {/* Error message */}
      {error && (
        <p className="text-sm text-red-600 dark:text-red-400 pl-1">{error}</p>
      )}
    </div>
  );
}
