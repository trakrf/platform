import { useState } from 'react';
import { X, Trash2, Loader2 } from 'lucide-react';
import toast from 'react-hot-toast';
import type { TagIdentifier } from '@/types/shared';
import { assetsApi } from '@/lib/api/assets';

interface TagIdentifiersModalProps {
  identifiers: TagIdentifier[];
  assetId?: number;
  assetName?: string;
  isOpen: boolean;
  onClose: () => void;
  onIdentifierRemoved?: (identifierId: number) => void;
}

const TAG_TYPE_LABELS: Record<string, string> = {
  rfid: 'RFID',
};

/**
 * Modal displaying tag identifiers for an asset.
 * Mobile-friendly with full-screen on small devices.
 * Supports removing identifiers when assetId is provided.
 */
export function TagIdentifiersModal({
  identifiers,
  assetId,
  assetName,
  isOpen,
  onClose,
  onIdentifierRemoved,
}: TagIdentifiersModalProps) {
  const [confirmingId, setConfirmingId] = useState<number | null>(null);
  const [removingId, setRemovingId] = useState<number | null>(null);

  if (!isOpen) return null;

  const handleRemoveClick = (identifierId: number) => {
    setConfirmingId(identifierId);
  };

  const handleConfirmRemove = async () => {
    if (!assetId || !confirmingId) return;

    setRemovingId(confirmingId);
    try {
      await assetsApi.removeIdentifier(assetId, confirmingId);
      toast.success('Tag identifier removed');
      onIdentifierRemoved?.(confirmingId);
    } catch (err) {
      toast.error('Failed to remove tag identifier');
    } finally {
      setRemovingId(null);
      setConfirmingId(null);
    }
  };

  const handleCancelRemove = () => {
    setConfirmingId(null);
  };

  const canRemove = assetId !== undefined && onIdentifierRemoved !== undefined;

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50 z-50 animate-fadeIn"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Modal */}
      <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-0 sm:p-4">
        <div
          className="bg-white dark:bg-gray-800 w-full sm:max-w-md sm:rounded-lg shadow-2xl max-h-[80vh] sm:max-h-[60vh] overflow-hidden animate-slideUp rounded-t-2xl sm:rounded-b-lg"
          onClick={(e) => e.stopPropagation()}
        >
          {/* Header */}
          <div className="flex items-center justify-between px-4 sm:px-6 py-4 border-b border-gray-200 dark:border-gray-700">
            <div>
              <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">
                Tag Identifiers
              </h2>
              {assetName && (
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
                  {assetName}
                </p>
              )}
            </div>
            <button
              onClick={onClose}
              className="p-2 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
              aria-label="Close modal"
            >
              <X className="w-5 h-5 text-gray-500 dark:text-gray-400" />
            </button>
          </div>

          {/* Content */}
          <div className="px-4 sm:px-6 py-4 overflow-y-auto max-h-[calc(80vh-8rem)] sm:max-h-[calc(60vh-8rem)]">
            {identifiers.length === 0 ? (
              <p className="text-sm text-gray-500 dark:text-gray-400 text-center py-4">
                No tag identifiers linked to this asset.
              </p>
            ) : (
              <div className="space-y-2">
                {identifiers.map((identifier) => (
                  <div
                    key={identifier.id}
                    className="flex items-center gap-3 p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg"
                  >
                    {/* Type badge */}
                    <span className="inline-flex items-center px-2 py-0.5 text-xs font-medium rounded bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300 flex-shrink-0">
                      {TAG_TYPE_LABELS[identifier.type] || identifier.type.toUpperCase()}
                    </span>

                    {/* Value */}
                    <span className="flex-1 text-sm font-mono text-gray-900 dark:text-gray-100 truncate min-w-0">
                      {identifier.value}
                    </span>

                    {/* Status badge */}
                    <span
                      className={`inline-flex items-center px-2 py-0.5 text-xs font-medium rounded flex-shrink-0 ${
                        identifier.is_active
                          ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300'
                          : 'bg-gray-100 text-gray-600 dark:bg-gray-600 dark:text-gray-300'
                      }`}
                    >
                      {identifier.is_active ? 'Active' : 'Inactive'}
                    </span>

                    {/* Delete button */}
                    {canRemove && (
                      <div className="flex-shrink-0">
                        {confirmingId === identifier.id ? (
                          <div className="flex items-center gap-2">
                            <button
                              onClick={handleCancelRemove}
                              disabled={removingId !== null}
                              className="px-2 py-1 text-xs font-medium text-gray-600 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200 disabled:opacity-50"
                            >
                              Cancel
                            </button>
                            <button
                              onClick={handleConfirmRemove}
                              disabled={removingId !== null}
                              className="flex items-center gap-1 px-2 py-1 text-xs font-medium text-white bg-red-600 hover:bg-red-700 rounded disabled:opacity-50"
                            >
                              {removingId === identifier.id ? (
                                <Loader2 className="w-3 h-3 animate-spin" />
                              ) : null}
                              Remove
                            </button>
                          </div>
                        ) : (
                          <button
                            onClick={() => handleRemoveClick(identifier.id)}
                            className="p-1.5 text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20 rounded transition-colors"
                            aria-label="Remove tag identifier"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        )}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Footer */}
          <div className="flex justify-end px-4 sm:px-6 py-4 border-t border-gray-200 dark:border-gray-700">
            <button
              onClick={onClose}
              className="w-full sm:w-auto px-4 py-2 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg hover:bg-gray-300 dark:hover:bg-gray-600 font-medium transition-colors"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </>
  );
}
