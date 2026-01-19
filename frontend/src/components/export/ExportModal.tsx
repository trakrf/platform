/**
 * ExportModal - Presentational component for export modal
 *
 * Pure UI component - all logic lives in useExportModal hook.
 */

import type { ReactNode } from 'react';
import { Download, Share2, X, Loader2 } from 'lucide-react';
import type { ExportFormat, ExportResult } from '@/types/export';
import { useExportModal } from './useExportModal';

export interface ExportModalProps {
  isOpen: boolean;
  onClose: () => void;
  selectedFormat: ExportFormat;
  itemCount: number;
  itemLabel?: string;
  generateExport: (format: ExportFormat) => ExportResult;
  shareTitle?: string;
  statsFooter?: ReactNode;
}

export function ExportModal({
  isOpen,
  onClose,
  selectedFormat,
  itemCount,
  itemLabel = 'items',
  generateExport,
  shareTitle = 'Export',
  statsFooter,
}: ExportModalProps) {
  const {
    loading,
    shareAPIStatus,
    canShareThisFormat,
    hasShareAPI,
    formatConfig,
    handleShare,
    handleDownload,
  } = useExportModal({
    selectedFormat,
    itemCount,
    itemLabel,
    shareTitle,
    generateExport,
    onClose,
  });

  if (!isOpen) return null;

  const Icon = formatConfig.icon;
  const isShareDisabled = loading || selectedFormat === 'xlsx' || !canShareThisFormat;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black bg-opacity-50"
        onClick={onClose}
        aria-label="Close modal"
      />

      {/* Modal */}
      <div className="relative bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-3">
            <Icon className={`w-6 h-6 ${formatConfig.color}`} />
            <div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
                Export {formatConfig.label}
              </h3>
              <p className="text-sm text-gray-500 dark:text-gray-400">
                {itemCount} {itemLabel} ready
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
            aria-label="Close"
          >
            <X className="w-5 h-5 text-gray-500 dark:text-gray-400" />
          </button>
        </div>

        {/* Info Banner */}
        {(!hasShareAPI || !canShareThisFormat) && (
          <div className="mb-4 p-3 bg-gray-50 dark:bg-gray-900/20 border border-gray-200 dark:border-gray-700 rounded-lg">
            <p className="text-xs text-gray-600 dark:text-gray-400">
              ℹ️{' '}
              {shareAPIStatus ||
                'File sharing requires HTTPS and a browser that supports the Web Share API with files'}
            </p>
            <p className="text-xs text-gray-500 dark:text-gray-500 mt-1">
              Protocol: {window.location.protocol} | Secure:{' '}
              {window.isSecureContext ? 'Yes' : 'No'} | Share API:{' '}
              {'share' in navigator ? 'Yes' : 'No'} | {selectedFormat.toUpperCase()}:{' '}
              {canShareThisFormat ? '✓' : '✗'}
            </p>
          </div>
        )}

        {/* Actions */}
        <div className="space-y-3">
          <button
            onClick={handleShare}
            disabled={isShareDisabled}
            className={`w-full py-4 rounded-lg transition-all flex items-center justify-center gap-3 text-lg font-medium ${
              isShareDisabled
                ? 'bg-gray-300 dark:bg-gray-700 text-gray-400 dark:text-gray-500 cursor-not-allowed opacity-50'
                : 'bg-blue-600 hover:bg-blue-700 text-white'
            } disabled:opacity-50 disabled:cursor-not-allowed`}
            title={
              selectedFormat === 'xlsx'
                ? 'Excel sharing is not supported. Use Download instead.'
                : !canShareThisFormat
                  ? 'Sharing not available on this device'
                  : 'Share using system share'
            }
          >
            {loading ? (
              <Loader2 className="w-5 h-5 animate-spin" />
            ) : (
              <>
                <Share2 className="w-5 h-5" />
                Share
                {selectedFormat === 'xlsx' && ' (Not Supported)'}
              </>
            )}
          </button>

          <button
            onClick={handleDownload}
            disabled={loading}
            className="w-full py-4 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-900 dark:text-gray-100 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-3 text-lg font-medium"
          >
            {loading ? (
              <Loader2 className="w-5 h-5 animate-spin" />
            ) : (
              <>
                <Download className="w-5 h-5" />
                Download
              </>
            )}
          </button>

          <button
            onClick={onClose}
            disabled={loading}
            className="w-full py-3 text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
        </div>

        {/* Stats footer (optional) */}
        {statsFooter && (
          <div className="mt-4 pt-4 border-t border-gray-200 dark:border-gray-700">
            {statsFooter}
          </div>
        )}
      </div>
    </div>
  );
}
