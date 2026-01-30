import { Download, Loader2 } from 'lucide-react';
import { useExportCsv } from '@/hooks/reports';
import type { AssetHistoryItem } from '@/types/reports';

interface ExportCsvButtonProps {
  data: AssetHistoryItem[];
  assetName: string;
  disabled?: boolean;
}

export function ExportCsvButton({
  data,
  assetName,
  disabled = false,
}: ExportCsvButtonProps) {
  const { exportToCsv, isExporting } = useExportCsv();

  const isDisabled = disabled || data.length === 0 || isExporting;

  const handleClick = () => {
    if (!isDisabled) {
      exportToCsv(data, assetName);
    }
  };

  return (
    <button
      onClick={handleClick}
      disabled={isDisabled}
      className="flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-gray-400
        text-white font-medium rounded-lg transition-colors disabled:cursor-not-allowed"
    >
      {isExporting ? (
        <Loader2 className="w-4 h-4 animate-spin" />
      ) : (
        <Download className="w-4 h-4" />
      )}
      Export CSV
    </button>
  );
}
