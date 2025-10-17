import { useState, useCallback, useRef } from 'react';
import { useTagStore } from '@/stores';
import { parseReconciliationCSV } from '@/utils/reconciliationUtils';
import { SAMPLE_INVENTORY_DATA } from '@test-utils/constants';

export function useReconciliation() {
  const [error, setError] = useState<string | null>(null);
  const [isProcessingCSV, setIsProcessingCSV] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const mergeReconciliationTags = useTagStore((state) => state.mergeReconciliationTags);

  const handleReconciliationUpload = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    setError(null);
    setIsProcessingCSV(true);

    const reader = new FileReader();
    reader.onload = (e) => {
      try {
        const csvData = e.target?.result as string;
        if (!csvData) {
          setError('Failed to read CSV file');
          return;
        }

        const parseResult = parseReconciliationCSV(csvData);
        if (!parseResult.success) {
          setError(parseResult.error || 'Failed to parse CSV file');
          return;
        }

        mergeReconciliationTags(parseResult.data);
      } finally {
        setIsProcessingCSV(false);
      }
    };

    reader.onerror = () => {
      setError('Error reading file');
      setIsProcessingCSV(false);
    };
    reader.readAsText(file);
    event.target.value = '';
  }, [mergeReconciliationTags]);

  const downloadSampleReconFile = useCallback(() => {
    try {
      const sampleContent = `Tag ID,Description,Location\n${SAMPLE_INVENTORY_DATA.map(item => `${item.epc},${item.description},${item.location}`).join('\n')}`;
      const blob = new Blob([sampleContent], { type: 'text/csv' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = 'reconciliation_sample.csv';
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Error downloading sample file:', error);
      setError('Failed to download sample file');
    }
  }, []);

  return {
    error,
    setError,
    isProcessingCSV,
    fileInputRef,
    handleReconciliationUpload,
    downloadSampleReconFile,
  };
}