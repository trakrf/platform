/**
 * ExportModal - Generic modal for download/share actions
 *
 * A reusable export modal component that can be used across screens
 * (Inventory, Assets, Locations). The caller provides the data generators.
 */

import { useState, useEffect, type ReactNode } from 'react';
import { Download, Share2, X, Loader2 } from 'lucide-react';
import toast from 'react-hot-toast';
import type { ExportFormat, ExportResult } from '@/types/export';
import { shareFile, downloadBlob, canShareFiles, canShareFormat } from '@/utils/shareUtils';
import { getFormatConfig } from '@/utils/exportFormats';

export interface ExportModalProps {
  /** Whether the modal is open */
  isOpen: boolean;
  /** Callback to close the modal */
  onClose: () => void;
  /** Selected export format */
  selectedFormat: ExportFormat;
  /** Number of items being exported (for display) */
  itemCount: number;
  /** Label for the type of items (e.g., "assets", "tags", "locations") */
  itemLabel?: string;
  /** Function to generate the export file for the selected format */
  generateExport: (format: ExportFormat) => ExportResult;
  /** Title shown in share dialog */
  shareTitle?: string;
  /** Optional stats footer content */
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
  const [loading, setLoading] = useState(false);
  const [shareAPIStatus, setShareAPIStatus] = useState<string>('');
  const [canShareThisFormat, setCanShareThisFormat] = useState(false);
  const hasShareAPI = canShareFiles();

  // Check if the specific format can be shared
  useEffect(() => {
    // Excel is permanently disabled for sharing
    if (selectedFormat === 'xlsx') {
      setCanShareThisFormat(false);
      setShareAPIStatus('Excel files cannot be shared. Use Download instead.');

      if (isOpen) {
        toast('Excel sharing not supported. Use Download instead.', {
          duration: 3000,
          icon: '📥',
        });
      }
    } else {
      // Check if this specific format can be shared
      const formatCanBeShared = canShareFormat(selectedFormat);
      setCanShareThisFormat(formatCanBeShared);

      if (!hasShareAPI) {
        const reason = !window.isSecureContext
          ? 'Not in secure context (HTTPS required)'
          : !('share' in navigator)
            ? 'Web Share API not available'
            : 'File sharing not supported';

        setShareAPIStatus(reason);

        if (isOpen) {
          toast.error(`Sharing disabled: ${reason}`, {
            duration: 5000,
            icon: '⚠️',
          });
        }
      } else if (!formatCanBeShared) {
        const formatName = selectedFormat.toUpperCase();
        const reason = `${formatName} files cannot be shared on this device`;
        setShareAPIStatus(reason);

        if (isOpen) {
          toast(`${formatName} sharing not supported. Use download instead.`, {
            duration: 4000,
            icon: '📥',
          });
        }
      } else {
        setShareAPIStatus('Share API available');
      }
    }
  }, [hasShareAPI, selectedFormat, isOpen]);

  const formatConfig = getFormatConfig(selectedFormat);
  const Icon = formatConfig.icon;

  // Handle share action
  const performShare = async (result: ExportResult) => {
    try {
      const shareResult = await shareFile(
        result.blob,
        result.filename,
        shareTitle,
        `${itemCount} ${itemLabel} exported as ${formatConfig.label}`
      );

      if (shareResult.shared) {
        toast.success(`${itemLabel.charAt(0).toUpperCase() + itemLabel.slice(1)} shared successfully`);
        return true;
      } else if (shareResult.method === 'cancelled') {
        return false;
      } else if (shareResult.method === 'unsupported') {
        toast.error('Sharing is not supported on this device');
        return false;
      } else {
        toast.error(`Failed to share ${itemLabel}`);
        return false;
      }
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Share failed';
      toast.error(`Share failed: ${errorMessage}`);
      return false;
    }
  };

  // Handle download action
  const performDownload = async (result: ExportResult) => {
    downloadBlob(result.blob, result.filename);
    toast.success(`${itemLabel.charAt(0).toUpperCase() + itemLabel.slice(1)} downloaded successfully`);
  };

  const handleExport = async (action: 'download' | 'share') => {
    // Block Excel sharing
    if (action === 'share' && selectedFormat === 'xlsx') {
      toast.error('Excel files cannot be shared. Please use Download instead.', {
        duration: 3000,
        icon: '📥',
      });
      return;
    }

    setLoading(true);
    try {
      const result = generateExport(selectedFormat);

      let actionCompleted = false;
      if (action === 'share') {
        actionCompleted = await performShare(result);
      } else {
        await performDownload(result);
        actionCompleted = true;
      }

      if (actionCompleted) {
        onClose();
      }
    } catch (error) {
      toast.error(`Failed to ${action} ${itemLabel}`);
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen) return null;

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
            onClick={() => handleExport('share')}
            disabled={loading || selectedFormat === 'xlsx' || !canShareThisFormat}
            className={`w-full py-4 rounded-lg transition-all flex items-center justify-center gap-3 text-lg font-medium ${
              selectedFormat === 'xlsx' || !canShareThisFormat
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
            onClick={() => handleExport('download')}
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
