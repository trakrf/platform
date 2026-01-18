/**
 * Tests for ExportModal component
 */

import '@testing-library/jest-dom';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act, cleanup } from '@testing-library/react';
import { ExportModal } from './ExportModal';
import type { ExportResult } from '@/types/export';

// Mock the toast library
vi.mock('react-hot-toast', () => ({
  default: Object.assign(vi.fn((_message, _options) => undefined), {
    success: vi.fn(),
    error: vi.fn(),
  }),
  Toaster: () => null,
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
  getFormatConfig: vi.fn((format: string) => {
    const configs: Record<string, { label: string; icon: () => null; color: string }> = {
      pdf: { label: 'PDF Report', icon: () => null, color: 'text-red-500' },
      xlsx: { label: 'Excel Spreadsheet', icon: () => null, color: 'text-green-500' },
      csv: { label: 'CSV File', icon: () => null, color: 'text-blue-500' },
    };
    return configs[format] || configs.pdf;
  }),
}));

// Mock export generator
const mockGenerateExport = vi.fn().mockReturnValue({
  blob: new Blob(['test'], { type: 'application/pdf' }),
  filename: 'test.pdf',
  mimeType: 'application/pdf',
} as ExportResult);

describe('ExportModal', () => {
  const mockOnClose = vi.fn();

  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    vi.clearAllMocks();
    mockGenerateExport.mockReturnValue({
      blob: new Blob(['test'], { type: 'application/pdf' }),
      filename: 'test.pdf',
      mimeType: 'application/pdf',
    });
  });

  test('renders when isOpen is true', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        itemLabel="assets"
        generateExport={mockGenerateExport}
      />
    );

    expect(screen.getByText('Export PDF Report')).toBeInTheDocument();
    expect(screen.getByText('Share')).toBeInTheDocument();
    expect(screen.getByText('Download')).toBeInTheDocument();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
  });

  test('does not render when isOpen is false', () => {
    render(
      <ExportModal
        isOpen={false}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        itemLabel="assets"
        generateExport={mockGenerateExport}
      />
    );

    expect(screen.queryByText('Export PDF Report')).not.toBeInTheDocument();
  });

  test('shows correct item count', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={42}
        itemLabel="assets"
        generateExport={mockGenerateExport}
      />
    );

    expect(screen.getByText('42 assets ready')).toBeInTheDocument();
  });

  test('shows custom item label', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={10}
        itemLabel="locations"
        generateExport={mockGenerateExport}
      />
    );

    expect(screen.getByText('10 locations ready')).toBeInTheDocument();
  });

  test('defaults item label to "items"', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={3}
        generateExport={mockGenerateExport}
      />
    );

    expect(screen.getByText('3 items ready')).toBeInTheDocument();
  });

  test('closes when backdrop is clicked', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    const backdrop = screen.getByLabelText('Close modal');
    fireEvent.click(backdrop);

    expect(mockOnClose).toHaveBeenCalled();
  });

  test('closes when X button is clicked', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    const closeButton = screen.getByLabelText('Close');
    fireEvent.click(closeButton);

    expect(mockOnClose).toHaveBeenCalled();
  });

  test('closes when Cancel button is clicked', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    fireEvent.click(screen.getByText('Cancel'));

    expect(mockOnClose).toHaveBeenCalled();
  });

  test('shows correct format label for xlsx', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="xlsx"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    expect(screen.getByText('Export Excel Spreadsheet')).toBeInTheDocument();
  });

  test('shows correct format label for csv', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="csv"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    expect(screen.getByText('Export CSV File')).toBeInTheDocument();
  });

  test('calls generateExport with correct format on download', async () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    const downloadButton = screen.getByText('Download').closest('button');
    fireEvent.click(downloadButton!);

    await waitFor(() => {
      expect(mockGenerateExport).toHaveBeenCalledWith('pdf');
    });
  });

  test('calls downloadBlob on download', async () => {
    const { downloadBlob } = await import('@/utils/shareUtils');

    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    const downloadButton = screen.getByText('Download').closest('button');
    fireEvent.click(downloadButton!);

    await waitFor(() => {
      expect(downloadBlob).toHaveBeenCalled();
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  test('shows success toast on download', async () => {
    const toast = (await import('react-hot-toast')).default;

    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        itemLabel="assets"
        generateExport={mockGenerateExport}
      />
    );

    const downloadButton = screen.getByText('Download').closest('button');
    fireEvent.click(downloadButton!);

    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith('Assets downloaded successfully');
    });
  });

  test('disables share button when canShareFormat returns false', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    const shareButton = screen.getByText('Share').closest('button');
    expect(shareButton).toBeDisabled();
  });

  test('disables share button for xlsx format', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="xlsx"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    // Find the button that contains "Share" text (not the info banner)
    const shareButton = screen.getByRole('button', { name: /Share/ });
    expect(shareButton).toBeDisabled();
  });

  test('shows info banner when sharing not available', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    // Should show info about sharing status
    expect(screen.getByText(/Protocol:/)).toBeInTheDocument();
  });

  test('renders stats footer when provided', () => {
    const statsFooter = (
      <div>
        <span data-testid="stats-total">10</span>
        <span data-testid="stats-found">8</span>
      </div>
    );

    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={10}
        generateExport={mockGenerateExport}
        statsFooter={statsFooter}
      />
    );

    expect(screen.getByTestId('stats-total')).toBeInTheDocument();
    expect(screen.getByTestId('stats-found')).toBeInTheDocument();
  });

  test('does not render stats footer when not provided', () => {
    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={10}
        generateExport={mockGenerateExport}
      />
    );

    // Look for border-t which is the stats footer container class
    const container = document.querySelector('.border-t.border-gray-200');
    expect(container).not.toBeInTheDocument();
  });

  test('disables buttons while loading', async () => {
    const { downloadBlob } = await import('@/utils/shareUtils');

    let resolveDownload: () => void;
    const downloadPromise = new Promise<void>((resolve) => {
      resolveDownload = () => resolve();
    });

    (downloadBlob as unknown as ReturnType<typeof vi.fn>).mockReturnValue(downloadPromise);

    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        generateExport={mockGenerateExport}
      />
    );

    const downloadButton = screen.getByText('Download').closest('button');
    fireEvent.click(downloadButton!);

    await waitFor(() => {
      const cancelButton = screen.getByText('Cancel').closest('button');
      expect(cancelButton).toBeDisabled();
    });

    act(() => {
      resolveDownload!();
    });

    await waitFor(() => {
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  test('handles export error gracefully', async () => {
    const toast = (await import('react-hot-toast')).default;
    mockGenerateExport.mockImplementation(() => {
      throw new Error('Export failed');
    });

    render(
      <ExportModal
        isOpen={true}
        onClose={mockOnClose}
        selectedFormat="pdf"
        itemCount={5}
        itemLabel="assets"
        generateExport={mockGenerateExport}
      />
    );

    const downloadButton = screen.getByText('Download').closest('button');
    fireEvent.click(downloadButton!);

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('Failed to download assets');
    });
  });
});
