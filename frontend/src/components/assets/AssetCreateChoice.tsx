import React from 'react';
import { X, FileText, Upload } from 'lucide-react';

interface AssetCreateChoiceProps {
  isOpen: boolean;
  onClose: () => void;
  onSingleCreate: () => void;
  onBulkUpload: () => void;
}

export function AssetCreateChoice({
  isOpen,
  onClose,
  onSingleCreate,
  onBulkUpload,
}: AssetCreateChoiceProps) {
  if (!isOpen) {
    return null;
  }

  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50 p-4"
      onClick={handleBackdropClick}
    >
      <div className="relative w-full max-w-md bg-white dark:bg-gray-900 rounded-lg shadow-xl">
        <div className="border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex items-center justify-between">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
            Create Assets
          </h2>
          <button
            onClick={onClose}
            className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg transition-colors"
            aria-label="Close modal"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="p-6 space-y-4">
          <button
            onClick={onSingleCreate}
            className="w-full p-6 border-2 border-gray-300 dark:border-gray-600 rounded-lg hover:border-blue-500 dark:hover:border-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-all group"
          >
            <div className="flex items-start gap-4">
              <div className="p-3 bg-blue-100 dark:bg-blue-900/30 rounded-lg group-hover:bg-blue-200 dark:group-hover:bg-blue-900/50 transition-colors">
                <FileText className="h-6 w-6 text-blue-600 dark:text-blue-400" />
              </div>
              <div className="flex-1 text-left">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-1">
                  Create Single Asset
                </h3>
                <p className="text-sm text-gray-600 dark:text-gray-400">
                  Create one asset at a time
                </p>
              </div>
            </div>
          </button>

          <button
            onClick={onBulkUpload}
            className="w-full p-6 border-2 border-gray-300 dark:border-gray-600 rounded-lg hover:border-blue-500 dark:hover:border-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-all group"
          >
            <div className="flex items-start gap-4">
              <div className="p-3 bg-blue-100 dark:bg-blue-900/30 rounded-lg group-hover:bg-blue-200 dark:group-hover:bg-blue-900/50 transition-colors">
                <Upload className="h-6 w-6 text-blue-600 dark:text-blue-400" />
              </div>
              <div className="flex-1 text-left">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-1">
                  Bulk Upload CSV
                </h3>
                <p className="text-sm text-gray-600 dark:text-gray-400">
                  Upload multiple assets from a CSV file
                </p>
              </div>
            </div>
          </button>
        </div>
      </div>
    </div>
  );
}
