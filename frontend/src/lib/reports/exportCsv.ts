/**
 * CSV Export Utilities for Reports
 */

import { formatDuration, formatTimestampForExport } from './utils';
import type { AssetHistoryItem } from '@/types/reports';

/**
 * Escape CSV field value
 * Handles commas, quotes, and newlines
 */
function escapeField(value: string): string {
  if (value.includes(',') || value.includes('"') || value.includes('\n')) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

/**
 * Generate CSV content from asset history data
 */
export function generateHistoryCsv(
  data: AssetHistoryItem[],
  assetName: string
): string {
  const headers = ['Asset', 'Timestamp', 'Location', 'Duration'];

  const rows = data.map((item) => [
    escapeField(assetName),
    formatTimestampForExport(item.timestamp),
    escapeField(item.location_name || 'Unknown'),
    item.duration_seconds ? formatDuration(item.duration_seconds) : 'ongoing',
  ]);

  return [headers.join(','), ...rows.map((row) => row.join(','))].join('\n');
}

/**
 * Trigger browser download of CSV file
 */
export function downloadCsv(content: string, filename: string): void {
  const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.style.display = 'none';
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}
