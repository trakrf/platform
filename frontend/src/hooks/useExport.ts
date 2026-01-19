/**
 * useExport - Generic export state management hook
 *
 * Manages modal state and format selection for export functionality.
 * Used by screens that need export capabilities (Assets, Locations, etc.)
 */

import { useState, useCallback } from 'react';
import type { ExportFormat } from '@/types/export';

export interface UseExportReturn {
  /** Whether the export modal is open */
  isModalOpen: boolean;
  /** Currently selected export format */
  selectedFormat: ExportFormat;
  /** Open the export modal with a specific format */
  openExport: (format: ExportFormat) => void;
  /** Close the export modal */
  closeExport: () => void;
}

/**
 * Hook for managing export modal state
 * @param defaultFormat - Default export format (defaults to 'csv')
 */
export function useExport(defaultFormat: ExportFormat = 'csv'): UseExportReturn {
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [selectedFormat, setSelectedFormat] = useState<ExportFormat>(defaultFormat);

  const openExport = useCallback((format: ExportFormat) => {
    setSelectedFormat(format);
    setIsModalOpen(true);
  }, []);

  const closeExport = useCallback(() => {
    setIsModalOpen(false);
  }, []);

  return {
    isModalOpen,
    selectedFormat,
    openExport,
    closeExport,
  };
}
