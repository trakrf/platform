import { X } from 'lucide-react';
import type { TagIdentifier } from '@/types/shared';
import { TagIdentifierList } from './TagIdentifierList';

interface TagIdentifiersModalProps {
  identifiers: TagIdentifier[];
  assetName?: string;
  isOpen: boolean;
  onClose: () => void;
}

/**
 * Modal displaying tag identifiers for an asset.
 * Mobile-friendly with full-screen on small devices.
 */
export function TagIdentifiersModal({
  identifiers,
  assetName,
  isOpen,
  onClose,
}: TagIdentifiersModalProps) {
  if (!isOpen) return null;

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
            <TagIdentifierList
              identifiers={identifiers}
              size="md"
              showHeader
            />
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
