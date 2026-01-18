/**
 * Tests for useExportModal hook
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useExportModal } from './useExportModal';
import type { ExportResult } from '@/types/export';

// Mock toast
vi.mock('react-hot-toast', () => ({
  default: Object.assign(vi.fn(), {
    success: vi.fn(),
    error: vi.fn(),
  }),
}));

// Mock shareUtils
vi.mock('@/utils/shareUtils', () => ({
  canShareFiles: vi.fn().mockReturnValue(false),
  canShareFormat: vi.fn().mockReturnValue(false),
  shareFile: vi.fn().mockResolvedValue({ shared: false, method: 'cancelled' }),
  downloadBlob: vi.fn(),
}));

// Mock exportFormats
vi.mock('@/utils/exportFormats', () => ({
  getFormatConfig: vi.fn((format: string) => ({
    label: format === 'pdf' ? 'PDF Report' : format === 'xlsx' ? 'Excel Spreadsheet' : 'CSV File',
    icon: () => null,
    color: 'text-gray-500',
  })),
}));

describe('useExportModal', () => {
  const mockGenerateExport = vi.fn().mockReturnValue({
    blob: new Blob(['test'], { type: 'text/plain' }),
    filename: 'test.csv',
    mimeType: 'text/csv',
  } as ExportResult);

  const mockOnClose = vi.fn();

  const defaultParams = {
    selectedFormat: 'csv' as const,
    itemCount: 10,
    itemLabel: 'assets',
    shareTitle: 'Asset Export',
    generateExport: mockGenerateExport,
    onClose: mockOnClose,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('initializes with loading false', () => {
    const { result } = renderHook(() => useExportModal(defaultParams));

    expect(result.current.loading).toBe(false);
  });

  it('returns format config for selected format', () => {
    const { result } = renderHook(() => useExportModal(defaultParams));

    expect(result.current.formatConfig.label).toBe('CSV File');
  });

  it('returns PDF config when pdf format selected', () => {
    const { result } = renderHook(() =>
      useExportModal({ ...defaultParams, selectedFormat: 'pdf' })
    );

    expect(result.current.formatConfig.label).toBe('PDF Report');
  });

  it('disables sharing for xlsx format', () => {
    const { result } = renderHook(() =>
      useExportModal({ ...defaultParams, selectedFormat: 'xlsx' })
    );

    expect(result.current.canShareThisFormat).toBe(false);
    expect(result.current.shareAPIStatus).toContain('Excel');
  });

  it('returns share status when share API not available', () => {
    const { result } = renderHook(() => useExportModal(defaultParams));

    expect(result.current.hasShareAPI).toBe(false);
    expect(result.current.canShareThisFormat).toBe(false);
  });

  it('calls generateExport and downloadBlob on handleDownload', async () => {
    const { downloadBlob } = await import('@/utils/shareUtils');
    const { result } = renderHook(() => useExportModal(defaultParams));

    await act(async () => {
      await result.current.handleDownload();
    });

    expect(mockGenerateExport).toHaveBeenCalledWith('csv');
    expect(downloadBlob).toHaveBeenCalled();
    expect(mockOnClose).toHaveBeenCalled();
  });

  it('sets loading during download', async () => {
    const { result } = renderHook(() => useExportModal(defaultParams));

    expect(result.current.loading).toBe(false);

    // Start download - loading should be true during async operation
    const downloadPromise = act(async () => {
      await result.current.handleDownload();
    });

    await downloadPromise;

    // After completion, loading should be false
    expect(result.current.loading).toBe(false);
  });

  it('shows success toast on download', async () => {
    const toast = (await import('react-hot-toast')).default;
    const { result } = renderHook(() => useExportModal(defaultParams));

    await act(async () => {
      await result.current.handleDownload();
    });

    expect(toast.success).toHaveBeenCalledWith('Assets downloaded successfully');
  });

  it('shows error toast when xlsx share attempted', async () => {
    const toast = (await import('react-hot-toast')).default;
    const { result } = renderHook(() =>
      useExportModal({ ...defaultParams, selectedFormat: 'xlsx' })
    );

    await act(async () => {
      await result.current.handleShare();
    });

    expect(toast.error).toHaveBeenCalledWith(
      'Excel files cannot be shared. Please use Download instead.',
      expect.any(Object)
    );
  });

  it('calls onClose after successful download', async () => {
    const { result } = renderHook(() => useExportModal(defaultParams));

    await act(async () => {
      await result.current.handleDownload();
    });

    expect(mockOnClose).toHaveBeenCalled();
  });

  it('handles download error gracefully', async () => {
    const toast = (await import('react-hot-toast')).default;
    mockGenerateExport.mockImplementationOnce(() => {
      throw new Error('Export failed');
    });

    const { result } = renderHook(() => useExportModal(defaultParams));

    await act(async () => {
      await result.current.handleDownload();
    });

    expect(toast.error).toHaveBeenCalledWith('Failed to download assets');
  });
});
