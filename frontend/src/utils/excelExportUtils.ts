/**
 * Excel export utilities for inventory data
 */

import * as XLSX from 'xlsx';
import type { TagInfo } from '../stores/tagStore';
import type { ExportResult } from '../types/export';
import { getDateString, getTimestamp } from './shareUtils';

/**
 * Generate an Excel workbook from inventory tags
 */
export function generateInventoryExcel(
  tags: TagInfo[],
  reconciliationList: string[] | null
): ExportResult {
  // Create a new workbook
  const wb = XLSX.utils.book_new();
  
  // Prepare inventory data — column order matches asset export for round-trip
  const inventoryData = tags.map(tag => {
    const row: Record<string, string | number> = {
      'Asset ID': tag.assetIdentifier || '',
      'Name': tag.assetName || tag.description || '',
      'Description': '',
      'Location': tag.locationName || tag.location || '',
      'Tag ID': tag.displayEpc || tag.epc,
      'RSSI (dBm)': tag.rssi ?? 'N/A',
      'Count': tag.count,
      'Last Seen': tag.timestamp ? new Date(tag.timestamp).toLocaleString() : 'N/A',
    };

    return row;
  });

  // Create inventory worksheet
  const inventoryWS = XLSX.utils.json_to_sheet(inventoryData);

  // Set column widths
  const columnWidths = [
    { wch: 15 }, // Asset ID
    { wch: 25 }, // Name
    { wch: 20 }, // Description
    { wch: 20 }, // Location
    { wch: 30 }, // Tag ID
    { wch: 12 }, // RSSI
    { wch: 8 },  // Count
    { wch: 20 }, // Last Seen
  ];

  inventoryWS['!cols'] = columnWidths;
  
  // Add inventory worksheet to workbook
  XLSX.utils.book_append_sheet(wb, inventoryWS, 'Inventory');
  
  // Create summary worksheet if reconciliation is active
  if (reconciliationList) {
    const summaryData = [
      { 'Metric': 'Report Generated', 'Value': getTimestamp() },
      { 'Metric': 'Total Tags Scanned', 'Value': tags.length },
      { 'Metric': 'Reconciliation List Size', 'Value': reconciliationList.length },
      { 'Metric': 'Tags Found', 'Value': tags.filter(t => t.reconciled === true).length },
      { 'Metric': 'Tags Missing', 'Value': tags.filter(t => t.reconciled === false).length },
      { 'Metric': 'Tags Not on List', 'Value': tags.filter(t => t.reconciled === null).length },
      { 'Metric': 'Found Percentage', 'Value': `${Math.round((tags.filter(t => t.reconciled === true).length / reconciliationList.length) * 100)}%` },
    ];
    
    const summaryWS = XLSX.utils.json_to_sheet(summaryData);
    summaryWS['!cols'] = [{ wch: 25 }, { wch: 30 }];
    XLSX.utils.book_append_sheet(wb, summaryWS, 'Summary');
    
    // Create missing items worksheet
    const missingTags = tags.filter(t => t.reconciled === false);
    if (missingTags.length > 0) {
      const missingData = missingTags.map(tag => ({
        'Tag ID': tag.displayEpc || tag.epc,
        'Description': tag.description || 'N/A',
        'Location': tag.location || 'N/A',
      }));
      
      const missingWS = XLSX.utils.json_to_sheet(missingData);
      missingWS['!cols'] = [{ wch: 30 }, { wch: 30 }, { wch: 20 }];
      XLSX.utils.book_append_sheet(wb, missingWS, 'Missing Items');
    }
  }
  
  // Generate Excel file with more compatible settings
  const wbout = XLSX.write(wb, { 
    bookType: 'xlsx', 
    type: 'array',
    compression: true // Ensure compression is enabled
  });
  
  // Create blob with proper MIME type
  // Note: Some browsers have issues with the long Excel MIME type
  const blob = new Blob([wbout], { 
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' 
  });
  
  // Log for debugging
  console.log('[Excel Export] Generated Excel file:', {
    size: blob.size,
    type: blob.type,
    sheets: Object.keys(wb.Sheets)
  });
  
  const filename = `inventory_${getDateString()}.xlsx`;
  
  return {
    blob,
    filename,
    mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  };
}

/**
 * Generate a CSV export — column order matches asset export for round-trip
 */
export function generateInventoryCSV(
  tags: TagInfo[],
  _reconciliationList: string[] | null
): ExportResult {
  const headers = ['Asset ID', 'Name', 'Description', 'Location', 'Tag ID', 'RSSI (dBm)', 'Count', 'Last Seen'];

  let csvContent = headers.join(',') + '\n';

  tags.forEach(tag => {
    const escapeCSV = (val: string) => val ? `"${val.replace(/"/g, '""')}"` : '';
    const row = [
      escapeCSV(tag.assetIdentifier || ''),
      escapeCSV(tag.assetName || tag.description || ''),
      '',
      escapeCSV(tag.locationName || tag.location || ''),
      `"${tag.displayEpc || tag.epc}"`,
      tag.rssi != null ? String(tag.rssi) : '',
      String(tag.count),
      tag.timestamp ? `"${new Date(tag.timestamp).toLocaleString()}"` : '',
    ];

    csvContent += row.join(',') + '\n';
  });

  const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
  const filename = `inventory_${getDateString()}.csv`;

  return {
    blob,
    filename,
    mimeType: 'text/csv'
  };
}