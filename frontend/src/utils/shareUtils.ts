/**
 * Web Share API wrapper with fallback support
 */

import type { ShareResult } from '../types/export';

/**
 * Check if the Web Share API is available
 */
export function canShare(): boolean {
  // Check for secure context (HTTPS or localhost)
  const isSecureContext = window.isSecureContext;
  
  // Check for share API
  const hasShareAPI = 'share' in navigator;
  
  
  return isSecureContext && hasShareAPI;
}

/**
 * Check if the Web Share API supports file sharing specifically
 */
export function canShareFiles(): boolean {
  // Must have share API first
  if (!canShare()) {
    return false;
  }
  
  // If canShare method exists, test file support
  if ('canShare' in navigator && typeof navigator.canShare === 'function') {
    try {
      // Test with empty files array first
      const testData = { files: [] as File[] };
      const supportsEmptyFiles = navigator.canShare(testData);
      
      // Test with actual CSV file
      try {
        const csvFile = new File(['Name,Count\nTest,1'], 'test.csv', { type: 'text/csv' });
        const canShareCSV = navigator.canShare({ files: [csvFile] });
        
        // Test with text file for comparison
        const txtFile = new File(['test'], 'test.txt', { type: 'text/plain' });
        const canShareTxt = navigator.canShare({ files: [txtFile] });
        
        // If any test returned true, files are supported
        const filesSupported = supportsEmptyFiles || canShareCSV || canShareTxt;
        return filesSupported;
      } catch (fileErr) {
        // Some browsers throw when testing with actual files
        return true; // Assume support exists
      }
    } catch (err) {
      // canShare might not support files parameter at all
      // Fall through to UA detection
    }
  }
  
  // Fallback: check user agent for known file sharing support
  
  if (typeof navigator !== 'undefined' && navigator && 'userAgent' in navigator) {
    const ua = navigator.userAgent;
    
    // iOS Safari 15+ supports file sharing
    const isIOSSafari = /iPhone|iPad|iPod/.test(ua) && 
                        (/Version\/1[5-9]/.test(ua) || /Version\/[2-9]\d/.test(ua));
    
    // Chrome/Edge on Android 89+ supports file sharing  
    const isChromeAndroid = /Android/.test(ua) && 
                           (/Chrome\/([8-9]\d|\d{3,})/.test(ua) || /Edg\/([8-9]\d|\d{3,})/.test(ua));
    
    // Modern desktop Chrome/Edge also support it
    const isModernDesktopChrome = !(/Android|iPhone|iPad|iPod/.test(ua)) &&
                                  (/Chrome\/(9[0-9]|\d{3,})/.test(ua) || /Edg\/(9[0-9]|\d{3,})/.test(ua));
    
    const hasFileSupport = isIOSSafari || isChromeAndroid || isModernDesktopChrome;
    
    
    return hasFileSupport;
  }
  
  return false;
}

/**
 * Share a file blob using Web Share API
 * Only shares, does not download as fallback
 */
export async function shareFile(
  blob: Blob,
  filename: string,
  title?: string,
  text?: string
): Promise<ShareResult> {
  // First check basic share API
  if (!canShare()) {
    return { shared: false, method: 'unsupported' };
  }
  
  // Check if we can share files specifically
  const canDoFileShare = canShareFiles();
  
  // Detect file types
  const isCSV = blob.type === 'text/csv' || filename.endsWith('.csv');
  const isExcel = (blob.type && (blob.type.includes('spreadsheetml') || blob.type.includes('excel'))) || 
                  filename.endsWith('.xlsx') || filename.endsWith('.xls');
  const isTabSeparated = blob.type && blob.type.includes('tab-separated');
  
  
  try {
    // If file sharing is supported, use files
    if (canDoFileShare) {
      let mimeType = blob.type || 'application/octet-stream';
      
      // Special handling for different file types
      if (isCSV) {
        mimeType = 'text/csv';
      } else if (isExcel && !isTabSeparated) {
        mimeType = 'application/octet-stream';
      }
      
      // Create file with appropriate MIME type
      const file = new File([blob], filename, { 
        type: mimeType,
        lastModified: Date.now()
      });
      
      // Build share data with just files
      const shareData: ShareData = {
        files: [file]
      };
      
      // Try to share
      await navigator.share(shareData);
      return { shared: true, method: 'share' };
      
    } else {
      // Fallback: Try text-based sharing
      
      const shareData: ShareData = {
        title: title || 'Inventory Export',
        text: text || `Export file: ${filename}`
      };
      
      // Try to share just the text
      await navigator.share(shareData);
      
      // Return that we couldn't share the actual file
      return { shared: false, method: 'unsupported' };
    }
    
  } catch (err) {
    if (err instanceof Error) {
      
      
      // Check for various cancellation error names across browsers
      if (err.name === 'AbortError' || 
          err.name === 'NotAllowedError' || 
          err.message?.includes('cancel')) {
        return { shared: false, method: 'cancelled' };
      }
      
      // Special handling for permission errors
      if (err.name === 'SecurityError' || err.message?.includes('permission')) {
        return { shared: false, method: 'unsupported' };
      }
      
      // Type error usually means the data format is wrong
      if (err.name === 'TypeError') {
        return { shared: false, method: 'unsupported' };
      }
    }
    
    // Re-throw to let the caller handle it with more context
    throw err;
  }
}

/**
 * Share or download a file blob
 * Uses Web Share API when available, falls back to download
 * @deprecated Use shareFile() or downloadBlob() directly
 */
export async function shareOrDownload(
  blob: Blob,
  filename: string,
  title?: string,
  text?: string
): Promise<ShareResult> {
  const result = await shareFile(blob, filename, title, text);
  if (result.method === 'unsupported' || result.method === 'error') {
    // Only fallback to download if share is not supported or errored
    downloadBlob(blob, filename);
    return { shared: false, method: 'download' };
  }
  return result;
}

/**
 * Download a blob as a file
 */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.style.display = 'none';
  document.body.appendChild(a);
  a.click();
  
  // Cleanup
  setTimeout(() => {
    URL.revokeObjectURL(url);
    a.remove();
  }, 100);
}

/**
 * Check if a specific file format can be shared
 */
export function canShareFormat(format: 'pdf' | 'xlsx' | 'csv'): boolean {
  // First check if basic share API is available
  if (!canShare()) {
    return false;
  }
  
  // If canShare method doesn't exist, fall back to general file sharing check
  if (!('canShare' in navigator) || typeof navigator.canShare !== 'function') {
    return canShareFiles();
  }
  
  try {
    // Create test files for the specific format
    let canShareThis = false;
    
    switch (format) {
      case 'pdf': {
        const pdfFile = new File(['test'], 'test.pdf', { type: 'application/pdf' });
        canShareThis = navigator.canShare({ files: [pdfFile] });
        break;
      }
        
      case 'xlsx': {
        // Excel is tricky - try multiple MIME types
        const mimeTypes = [
          'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
          'application/vnd.ms-excel', 
          'application/octet-stream'
        ];
        
        for (const mimeType of mimeTypes) {
          try {
            const excelFile = new File(['test'], 'test.xlsx', { type: mimeType });
            const result = navigator.canShare({ files: [excelFile] });
            if (result) {
              canShareThis = true;
              break; // Found a working MIME type
            }
          } catch (err) {
            // Continue to next MIME type
          }
        }
        break;
      }
        
      case 'csv': {
        const csvFile = new File(['Name,Count\nTest,1'], 'test.csv', { type: 'text/csv' });
        canShareThis = navigator.canShare({ files: [csvFile] });
        break;
      }
        
      default:
        return false;
    }
    
    return canShareThis;
    
  } catch (err) {
    // Don't assume it works if testing throws - be conservative
    return false;
  }
}

/**
 * Get formatted date string for file naming
 */
export function getDateString(): string {
  const now = new Date();
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

/**
 * Get formatted timestamp for reports
 */
export function getTimestamp(): string {
  return new Date().toLocaleString();
}