/**
 * useExportModal - Logic hook for ExportModal
 *
 * Handles all export modal state and actions:
 * - Share API availability detection
 * - Export generation and download/share execution
 * - Loading states and error handling
 */

import { useState, useCallback, useMemo } from 'react';
import toast from 'react-hot-toast';
import type { ExportFormat, ExportResult } from '@/types/export';
import { shareFile, downloadBlob, canShareFiles, canShareFormat } from '@/utils/shareUtils';
import { getFormatConfig } from '@/utils/exportFormats';

export interface UseExportModalParams {
  selectedFormat: ExportFormat;
  itemCount: number;
  itemLabel: string;
  shareTitle: string;
  generateExport: (format: ExportFormat) => ExportResult;
  onClose: () => void;
}

export interface UseExportModalReturn {
  loading: boolean;
  shareAPIStatus: string;
  canShareThisFormat: boolean;
  hasShareAPI: boolean;
  formatConfig: ReturnType<typeof getFormatConfig>;
  handleShare: () => void;
  handleDownload: () => void;
}

export function useExportModal({
  selectedFormat,
  itemCount,
  itemLabel,
  shareTitle,
  generateExport,
  onClose,
}: UseExportModalParams): UseExportModalReturn {
  const [loading, setLoading] = useState(false);

  const hasShareAPI = canShareFiles();
  const formatConfig = getFormatConfig(selectedFormat);

  // Compute share status - no useEffect needed, just derived state
  const { canShareThisFormat, shareAPIStatus } = useMemo(() => {
    // Excel is permanently disabled for sharing
    if (selectedFormat === 'xlsx') {
      return {
        canShareThisFormat: false,
        shareAPIStatus: 'Excel files cannot be shared. Use Download instead.',
      };
    }

    const formatCanBeShared = canShareFormat(selectedFormat);

    if (!hasShareAPI) {
      const reason = !window.isSecureContext
        ? 'Not in secure context (HTTPS required)'
        : !('share' in navigator)
          ? 'Web Share API not available'
          : 'File sharing not supported';

      return {
        canShareThisFormat: false,
        shareAPIStatus: reason,
      };
    }

    if (!formatCanBeShared) {
      const formatName = selectedFormat.toUpperCase();
      return {
        canShareThisFormat: false,
        shareAPIStatus: `${formatName} files cannot be shared on this device`,
      };
    }

    return {
      canShareThisFormat: true,
      shareAPIStatus: 'Share API available',
    };
  }, [selectedFormat, hasShareAPI]);

  // Capitalize first letter helper
  const capitalize = (str: string) => str.charAt(0).toUpperCase() + str.slice(1);

  // Share action
  const handleShare = useCallback(async () => {
    if (selectedFormat === 'xlsx') {
      toast.error('Excel files cannot be shared. Please use Download instead.', {
        duration: 3000,
        icon: '📥',
      });
      return;
    }

    setLoading(true);
    try {
      const result = generateExport(selectedFormat);
      const shareResult = await shareFile(
        result.blob,
        result.filename,
        shareTitle,
        `${itemCount} ${itemLabel} exported as ${formatConfig.label}`
      );

      if (shareResult.shared) {
        toast.success(`${capitalize(itemLabel)} shared successfully`);
        onClose();
      } else if (shareResult.method === 'cancelled') {
        // User cancelled - do nothing
      } else if (shareResult.method === 'unsupported') {
        toast.error('Sharing is not supported on this device');
      } else {
        toast.error(`Failed to share ${itemLabel}`);
      }
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Share failed';
      toast.error(`Share failed: ${errorMessage}`);
    } finally {
      setLoading(false);
    }
  }, [selectedFormat, generateExport, shareTitle, itemCount, itemLabel, formatConfig.label, onClose]);

  // Download action
  const handleDownload = useCallback(async () => {
    setLoading(true);
    try {
      const result = generateExport(selectedFormat);
      downloadBlob(result.blob, result.filename);
      toast.success(`${capitalize(itemLabel)} downloaded successfully`);
      onClose();
    } catch (error) {
      toast.error(`Failed to download ${itemLabel}`);
    } finally {
      setLoading(false);
    }
  }, [selectedFormat, generateExport, itemLabel, onClose]);

  return {
    loading,
    shareAPIStatus,
    canShareThisFormat,
    hasShareAPI,
    formatConfig,
    handleShare,
    handleDownload,
  };
}
