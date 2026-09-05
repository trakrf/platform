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
      // Name no longer falls back to the description — see the Description
      // column below, which used to be a hardcoded empty string.
      'Name': tag.assetName || '',
      'Description': tag.description || '',
      'Location': tag.locationName || tag.location || '',
      'Tag ID': tag.displayEpc || tag.epc,
      // Capture columns (TRA-1251). Always present, blank when the scan did not
      // capture any, so a column that is empty and a column that is missing
      // cannot be confused for one another.
      'PC': tag.pc != null ? `0x${tag.pc.toString(16).toUpperCase().padStart(4, '0')}` : '',
      'TID': tag.tid || '',
      'User Data': tag.userData || '',
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
    { wch: 8 },  // PC
    { wch: 28 }, // TID — 6 words is 24 hex characters
    { wch: 28 }, // User Data
    { wch: 12 }, // RSSI
    { wch: 8 },  // Count
    { wch: 20 }, // Last Seen
  ];

  inventoryWS['!cols'] = columnWidths;
  
  // Add inventory worksheet to workbook
  XLSX.utils.book_append_sheet(wb, inventoryWS, 'Inventory');
  
  // Create summary worksheet if reconciliation is active
  if (reconciliationList) {
    // Asset-level summary: group by assetIdentifier to count unique assets
    const assetStatus = new Map<string, boolean>();
    for (const t of tags) {
      if (t.reconciled == null) continue;
      const key = t.assetIdentifier ?? t.epc;
      assetStatus.set(key, assetStatus.get(key) || t.reconciled === true);
    }
    const assetsFound = [...assetStatus.values()].filter(Boolean).length;
    const assetsMissing = assetStatus.size - assetsFound;

    const summaryData = [
      { 'Metric': 'Report Generated', 'Value': getTimestamp() },
      { 'Metric': 'Total Tags Scanned', 'Value': tags.filter(t => t.source !== 'reconciliation').length },
      { 'Metric': 'Assets in Reconciliation List', 'Value': assetStatus.size },
      { 'Metric': 'Assets Found', 'Value': assetsFound },
      { 'Metric': 'Assets Missing', 'Value': assetsMissing },
      { 'Metric': 'Tags Not on List', 'Value': tags.filter(t => t.reconciled == null).length },
      { 'Metric': 'Found Percentage', 'Value': assetStatus.size > 0 ? `${Math.round((assetsFound / assetStatus.size) * 100)}%` : '0%' },
    ];
    
    const summaryWS = XLSX.utils.json_to_sheet(summaryData);
    summaryWS['!cols'] = [{ wch: 25 }, { wch: 30 }];
    XLSX.utils.book_append_sheet(wb, summaryWS, 'Summary');
    
    // Create missing items worksheet
    const missingTags = tags.filter(t => t.reconciled === false);
    if (missingTags.length > 0) {
      const missingData = missingTags.map(tag => ({
        'Asset ID': tag.assetIdentifier || '',
        'Tag ID': tag.displayEpc || tag.epc,
        'Name': tag.assetName || tag.description || 'N/A',
        'Location': tag.locationName || tag.location || 'N/A',
      }));

      const missingWS = XLSX.utils.json_to_sheet(missingData);
      missingWS['!cols'] = [{ wch: 15 }, { wch: 30 }, { wch: 25 }, { wch: 20 }];
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
  const headers = [
    'Asset ID', 'Name', 'Description', 'Location', 'Tag ID',
    // PC, TID and User Data come from a "capture all tag data" scan and are
    // blank otherwise (TRA-1251).
    'PC', 'TID', 'User Data',
    'RSSI (dBm)', 'Count', 'Last Seen'
  ];

  let csvContent = headers.join(',') + '\n';

  tags.forEach(tag => {
    const escapeCSV = (val: string) => val ? `"${val.replace(/"/g, '""')}"` : '';
    const row = [
      escapeCSV(tag.assetIdentifier || ''),
      // Name no longer falls back to description. That fallback existed only
      // because the Description column below was a hardcoded empty string, so
      // an asset's description never reached the CSV at all — while the Excel
      // and PDF exports carried it. Both halves are fixed together; keeping the
      // fallback would now duplicate the value across two columns.
      escapeCSV(tag.assetName || ''),
      escapeCSV(tag.description || ''),
      escapeCSV(tag.locationName || tag.location || ''),
      `"${tag.displayEpc || tag.epc}"`,
      // Hex, because that is the form the value is reasoned about in: 0x3000
      // is a 96-bit EPC and 0x4000 a 128-bit one. "12288" answers nothing.
      tag.pc != null ? `0x${tag.pc.toString(16).toUpperCase().padStart(4, '0')}` : '',
      escapeCSV(tag.tid || ''),
      escapeCSV(tag.userData || ''),
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