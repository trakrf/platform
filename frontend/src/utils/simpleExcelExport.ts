/**
 * Alternative Excel export using a simpler approach
 * This creates a basic CSV that Excel can open, with .xlsx extension
 */

import type { TagInfo } from '../stores/tagStore';
import type { ExportResult } from '../types/export';
import { getDateString } from './shareUtils';

/**
 * Generate a simple Excel-compatible file
 * Uses tab-separated values which Excel handles well
 */
export function generateSimpleExcel(
  tags: TagInfo[],
  reconciliationList: string[] | null
): ExportResult {
  const separator = '\t'; // Tab separator works better with Excel
  const headers = ['Tag ID', 'RSSI (dBm)', 'Count', 'Last Seen'];
  
  if (reconciliationList) {
    headers.push('Status');
  }
  
  if (tags.some(t => t.description)) {
    headers.push('Description');
  }
  
  if (tags.some(t => t.location)) {
    headers.push('Location');
  }
  
  headers.push('Source');
  
  // Build the content
  let content = headers.join(separator) + '\n';
  
  tags.forEach(tag => {
    const row = [
      tag.displayEpc || tag.epc,
      tag.rssi?.toString() ?? 'N/A',
      tag.count.toString(),
      tag.timestamp ? new Date(tag.timestamp).toLocaleString() : 'N/A'
    ];
    
    if (reconciliationList) {
      const status = tag.reconciled === true ? 'Found' : 
                     tag.reconciled === false ? 'Missing' : 
                     'Not on list';
      row.push(status);
    }
    
    if (tags.some(t => t.description)) {
      row.push(tag.description || '');
    }
    
    if (tags.some(t => t.location)) {
      row.push(tag.location || '');
    }
    
    row.push(tag.source);
    
    content += row.join(separator) + '\n';
  });
  
  // Add BOM for Excel to recognize UTF-8
  const BOM = '\uFEFF';
  const fullContent = BOM + content;
  
  // Create blob with a MIME type that works better for sharing
  // Using text/tab-separated-values which is more widely supported
  const blob = new Blob([fullContent], { 
    type: 'text/tab-separated-values;charset=utf-8' 
  });
  
  console.log('[Simple Excel Export] Generated file:', {
    size: blob.size,
    type: blob.type,
    rows: tags.length + 1
  });
  
  const filename = `inventory_${getDateString()}.xlsx`;
  
  return {
    blob,
    filename,
    mimeType: 'text/tab-separated-values'
  };
}

/**
 * Try multiple approaches to generate shareable Excel
 */
export function generateShareableExcel(
  tags: TagInfo[],
  reconciliationList: string[] | null
): ExportResult {
  // For now, use the simple approach
  // This can be extended to try multiple formats
  return generateSimpleExcel(tags, reconciliationList);
}