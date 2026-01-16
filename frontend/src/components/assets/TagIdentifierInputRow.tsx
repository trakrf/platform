import { Trash2, AlertTriangle } from 'lucide-react';

type TagType = 'rfid';

/**
 * Validates EPC data for common issues caused by BLE truncation or corruption.
 * Returns warning message if invalid, undefined if valid.
 */
function validateEPC(data: string): string | undefined {
  if (!data || data.trim() === '') return undefined; // Don't warn on empty
  // Check hex characters first (catches corruption)
  if (!/^[0-9A-Fa-f]+$/.test(data)) {
    return "Invalid characters - try again or enter manually";
  }
  // Check minimum length (96-bit standard)
  if (data.length < 24) {
    return "Scan may be incomplete - try again or enter manually";
  }
  // Check 32-bit word boundary alignment
  if (data.length % 8 !== 0) {
    return "Invalid EPC length - must be divisible by 8";
  }
  return undefined;
}

interface TagIdentifierInputRowProps {
  type: TagType;
  value: string;
  onTypeChange: (type: TagType) => void;
  onValueChange: (value: string) => void;
  onRemove?: () => void;
  disabled?: boolean;
  error?: string;
  /** Called when input gains focus */
  onFocus?: () => void;
  /** Called when input loses focus */
  onBlur?: () => void;
  /** True when this input is focused (for styling) */
  isFocused?: boolean;
  /** Auto-focus this input on mount */
  autoFocus?: boolean;
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
  onFocus,
  onBlur,
  isFocused = false,
  autoFocus = false,
}: TagIdentifierInputRowProps) {
  const warning = validateEPC(value);

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
          onFocus={onFocus}
          onBlur={onBlur}
          disabled={disabled}
          autoFocus={autoFocus}
          placeholder="Enter tag number..."
          className={`flex-1 px-3 py-2 text-sm border rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50 ${
            error
              ? 'border-red-500 focus:ring-red-500'
              : warning
                ? 'border-yellow-500 focus:ring-yellow-500'
                : isFocused
                  ? 'border-green-500 focus:ring-green-500'
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

      {/* EPC validation warning */}
      {!error && warning && (
        <div className="flex items-center gap-1.5 text-sm text-yellow-700 dark:text-yellow-400 pl-1">
          <AlertTriangle className="w-3.5 h-3.5 flex-shrink-0" />
          <span>{warning}</span>
        </div>
      )}
    </div>
  );
}
