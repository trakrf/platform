/**
 * Export-related type definitions for multi-format inventory sharing
 */

import type { TagInfo } from '@/stores/tagStore';

export type ExportFormat = 'csv' | 'pdf' | 'xlsx';

export interface ExportOptions {
  format: ExportFormat;
  includeMetadata: boolean;
  filename?: string;
}

export interface ShareModalProps {
  isOpen: boolean;
  onClose: () => void;
  tags: TagInfo[];
  reconciliationList: string[] | null;
  selectedFormat: ExportFormat;
}

export interface ExportResult {
  blob: Blob;
  filename: string;
  mimeType: string;
}

export interface ShareResult {
  shared: boolean;
  method: 'share' | 'download' | 'cancelled' | 'unsupported' | 'error';
  error?: string;
}

export interface FormatOption {
  id: ExportFormat;
  label: string;
  icon: string;
  description: string;
  mimeType: string;
}