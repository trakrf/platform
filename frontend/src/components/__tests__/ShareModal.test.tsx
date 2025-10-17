/**
 * Tests for ShareModal component
 */

import '@testing-library/jest-dom';
import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act, cleanup } from '@testing-library/react';
import { ShareModal } from '../ShareModal';
import type { TagInfo } from '../../stores/tagStore';

// Mock the toast library
vi.mock('react-hot-toast', () => ({
  default: Object.assign(
    vi.fn((_message, _options) => undefined),
    {
      success: vi.fn(),
      error: vi.fn()
    }
  ),
  Toaster: () => null
}));

// Mock the export utilities
vi.mock('../../utils/pdfExportUtils', () => ({
  generateInventoryPDF: vi.fn().mockReturnValue({
    blob: new Blob(['pdf'], { type: 'application/pdf' }),
    filename: 'inventory.pdf',
    mimeType: 'application/pdf'
  })
}));

vi.mock('../../utils/excelExportUtils', () => ({
  generateInventoryExcel: vi.fn().mockReturnValue({
    blob: new Blob(['excel'], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }),
    filename: 'inventory.xlsx',
    mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  }),
  generateInventoryCSV: vi.fn().mockReturnValue({
    blob: new Blob(['csv'], { type: 'text/csv' }),
    filename: 'inventory.csv',
    mimeType: 'text/csv'
  })
}));

vi.mock('../../utils/shareUtils', () => ({
  canShareFiles: vi.fn().mockReturnValue(false),
  canShareFormat: vi.fn().mockReturnValue(false),
  shareFile: vi.fn().mockResolvedValue({ shared: false, method: 'cancelled' }),
  shareOrDownload: vi.fn().mockResolvedValue({ shared: false, method: 'download' }),
  downloadBlob: vi.fn()
}));

// Sample test data
const mockTags: TagInfo[] = [
  {
    epc: '300833B2DDD9014000000001',
    displayEpc: '300833B2DDD9014000000001',
    rssi: -60,
    count: 5,
    timestamp: Date.now(),
    reconciled: true,
    source: 'rfid'
  }
];

describe('ShareModal', () => {
  const mockOnClose = vi.fn();

  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('renders when isOpen is true', () => {
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="pdf"
      />
    );

    expect(screen.getByText('Export PDF Report')).toBeInTheDocument();
    expect(screen.getByText('Share')).toBeInTheDocument();
    expect(screen.getByText('Download')).toBeInTheDocument();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
  });

  test('does not render when isOpen is false', () => {
    render(
      <ShareModal
        isOpen={false}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="pdf"
      />
    );

    expect(screen.queryByText('Export Inventory')).not.toBeInTheDocument();
  });

  test('closes when backdrop is clicked', () => {
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="pdf"
      />
    );

    const backdrop = screen.getByLabelText('Close modal');
    fireEvent.click(backdrop);

    expect(mockOnClose).toHaveBeenCalled();
  });

  test('closes when X button is clicked', () => {
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="pdf"
      />
    );

    const closeButton = screen.getByLabelText('Close');
    fireEvent.click(closeButton);

    expect(mockOnClose).toHaveBeenCalled();
  });

  test('shows correct format in header', () => {
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="xlsx"
      />
    );

    expect(screen.getByText('Export Excel Spreadsheet')).toBeInTheDocument();
  });

  test('calls export with correct format', async () => {
    const { generateInventoryPDF } = await import('../../utils/pdfExportUtils');
    
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="pdf"
      />
    );

    const downloadButton = screen.getByText('Download').closest('button');
    fireEvent.click(downloadButton!);

    await waitFor(() => {
      expect(generateInventoryPDF).toHaveBeenCalledWith(mockTags, null);
    });
  });

  test('downloads PDF when PDF format is selected', async () => {
    const { downloadBlob } = await import('../../utils/shareUtils');
    const toast = (await import('react-hot-toast')).default;
    
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="pdf"
      />
    );

    // Click download
    const downloadButton = screen.getByText('Download').closest('button');
    fireEvent.click(downloadButton!);

    await waitFor(() => {
      expect(downloadBlob).toHaveBeenCalled();
      expect(toast.success).toHaveBeenCalledWith('Inventory downloaded successfully');
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  test('downloads Excel when Excel format is selected', async () => {
    const { downloadBlob } = await import('../../utils/shareUtils');
    const { generateInventoryExcel } = await import('../../utils/excelExportUtils');
    const toast = (await import('react-hot-toast')).default;
    
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="xlsx"
      />
    );

    // Click download
    const downloadButton = screen.getByText('Download').closest('button');
    fireEvent.click(downloadButton!);

    await waitFor(() => {
      expect(generateInventoryExcel).toHaveBeenCalledWith(mockTags, null);
      expect(downloadBlob).toHaveBeenCalled();
      expect(toast.success).toHaveBeenCalledWith('Inventory downloaded successfully');
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  test('shows item count in info section', () => {
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="pdf"
      />
    );

    expect(screen.getByText('1 items ready')).toBeInTheDocument();
  });

  test('shows reconciliation count when reconciliation list is provided', () => {
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={['300833B2DDD9014000000001']}
        selectedFormat="pdf"
      />
    );

    // Check for the "Found" label which appears in reconciliation stats
    expect(screen.getByText('Found')).toBeInTheDocument();
    // Check for other stats labels that appear with reconciliation
    expect(screen.getByText('Total')).toBeInTheDocument();
    expect(screen.getByText('Missing')).toBeInTheDocument();
  });

  test('has correct button states initially', () => {
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="pdf"
      />
    );

    // Check initial button states
    const shareButton = screen.getByText('Share').closest('button');
    const downloadButton = screen.getByText('Download').closest('button');
    const cancelButton = screen.getByText('Cancel').closest('button');
    
    // Share button should be disabled when canShareFormat returns false (our mock)
    expect(shareButton).toBeDisabled();
    // Download and Cancel should be enabled initially
    expect(downloadButton).toBeEnabled();
    expect(cancelButton).toBeEnabled();
  });

  test('disables buttons while loading', async () => {
    // Mock the download function to be async
    const { downloadBlob } = await import('../../utils/shareUtils');
    
    // Create a promise we can control for the download
    let resolveDownload: () => void;
    const downloadPromise = new Promise<void>((resolve) => {
      resolveDownload = () => resolve();
    });
    
    (downloadBlob as jest.Mock).mockReturnValue(downloadPromise);
    
    render(
      <ShareModal
        isOpen={true}
        onClose={mockOnClose}
        tags={mockTags}
        reconciliationList={null}
        selectedFormat="pdf"
      />
    );

    // Click download button to trigger loading
    const downloadButton = screen.getByText('Download').closest('button');
    
    // Start the download
    fireEvent.click(downloadButton!);
    
    // Wait a tick for the loading state to be set
    await waitFor(() => {
      // Check that buttons are disabled during loading
      const buttons = screen.getAllByRole('button');
      const downloadBtn = buttons.find(btn => btn.textContent?.includes('Download') || btn.querySelector('.animate-spin'));
      const cancelBtn = buttons.find(btn => btn.textContent === 'Cancel');
      
      expect(downloadBtn).toBeDisabled();
      expect(cancelBtn).toBeDisabled();
    });
    
    // Resolve the promise to complete loading
    act(() => {
      resolveDownload!();
    });
    
    // Wait for loading to complete and modal to close
    await waitFor(() => {
      expect(mockOnClose).toHaveBeenCalled();
    });
  });
});