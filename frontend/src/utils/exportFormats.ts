/**
 * Shared configuration for export formats
 */

import { FileText, FileSpreadsheet, FileDown } from 'lucide-react';
import type { ExportFormat } from '../types/export';

export interface FormatConfig {
  id: ExportFormat;
  label: string;
  shortLabel: string;
  icon: typeof FileText;
  description: string;
  color: string;
  mimeType: string;
}

/**
 * Export format configurations used across ShareButton and ShareModal
 */
export const EXPORT_FORMATS: Record<ExportFormat, FormatConfig> = {
  pdf: {
    id: 'pdf',
    label: 'PDF Report',
    shortLabel: 'PDF',
    icon: FileText,
    description: 'Professional report with formatting',
    color: 'text-red-600 dark:text-red-400',
    mimeType: 'application/pdf'
  },
  xlsx: {
    id: 'xlsx',
    label: 'Excel Spreadsheet',
    shortLabel: 'Excel',
    icon: FileSpreadsheet,
    description: 'Spreadsheet for data analysis',
    color: 'text-green-600 dark:text-green-400',
    mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  },
  csv: {
    id: 'csv',
    label: 'CSV File',
    shortLabel: 'CSV',
    icon: FileDown,
    description: 'Simple comma-separated values',
    color: 'text-blue-600 dark:text-blue-400',
    mimeType: 'text/csv'
  }
};

/**
 * Get all format options as an array (useful for dropdowns)
 */
export const getFormatOptions = (): FormatConfig[] => {
  return Object.values(EXPORT_FORMATS);
};

/**
 * Get a specific format configuration
 */
export const getFormatConfig = (format: ExportFormat): FormatConfig => {
  return EXPORT_FORMATS[format];
};