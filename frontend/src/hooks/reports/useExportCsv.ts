import { useState, useCallback, useMemo } from 'react';
import { generateHistoryCsv, downloadCsv } from '@/lib/reports/exportCsv';
import type { AssetHistoryItem } from '@/types/reports';

interface UseExportCsvProps {
  data: AssetHistoryItem[];
  assetName: string;
  disabled?: boolean;
}

interface UseExportCsvReturn {
  isExporting: boolean;
  isDisabled: boolean;
  handleExport: () => void;
}

export function useExportCsv({
  data,
  assetName,
  disabled = false,
}: UseExportCsvProps): UseExportCsvReturn {
  const [isExporting, setIsExporting] = useState(false);

  const isDisabled = useMemo(
    () => disabled || data.length === 0 || isExporting,
    [disabled, data.length, isExporting]
  );

  const handleExport = useCallback(() => {
    if (isDisabled) return;

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
  }, [data, assetName, isDisabled]);

  return { isExporting, isDisabled, handleExport };
}
