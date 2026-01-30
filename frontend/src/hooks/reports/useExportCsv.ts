import { useState, useCallback } from 'react';
import { generateHistoryCsv, downloadCsv } from '@/lib/reports/exportCsv';
import type { AssetHistoryItem } from '@/types/reports';

interface UseExportCsvReturn {
  exportToCsv: (data: AssetHistoryItem[], assetName: string) => void;
  isExporting: boolean;
}

export function useExportCsv(): UseExportCsvReturn {
  const [isExporting, setIsExporting] = useState(false);

  const exportToCsv = useCallback(
    (data: AssetHistoryItem[], assetName: string) => {
      setIsExporting(true);
      try {
        const csv = generateHistoryCsv(data, assetName);
        const sanitizedName = assetName.replace(/[^a-zA-Z0-9-_]/g, '-');
        const date = new Date().toISOString().split('T')[0];
        const filename = `${sanitizedName}-history-${date}.csv`;
        downloadCsv(csv, filename);
      } finally {
        // Brief delay to show loading state for better UX
        setTimeout(() => setIsExporting(false), 500);
      }
    },
    []
  );

  return { exportToCsv, isExporting };
}
