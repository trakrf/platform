/**
 * Reports Export Utilities
 *
 * Generates PDF, Excel, and CSV exports for report data.
 * Follows patterns from assetExport.ts
 */

import * as XLSX from 'xlsx';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';
import type { CurrentLocationItem, AssetHistoryItem } from '@/types/reports';
import type { ExportResult } from '@/types/export';
import { getDateString, getTimestamp } from '@/utils/shareUtils';
import {
  getFreshnessStatus,
  formatRelativeTime,
  formatDuration,
  formatTimestampForExport,
} from '@/lib/reports/utils';

// ============================================
// Current Locations Export
// ============================================

/**
 * Get freshness label for display
 */
function getFreshnessLabel(lastSeen: string): string {
  const status = getFreshnessStatus(lastSeen);
  switch (status) {
    case 'live':
      return 'Live';
    case 'today':
      return 'Today';
    case 'recent':
      return 'Recent';
    case 'stale':
      return 'Stale';
  }
}

/**
 * Generate PDF report from current locations
 */
export function generateCurrentLocationsPDF(data: CurrentLocationItem[]): ExportResult {
  const doc = new jsPDF();

  // Header
  doc.setFontSize(20);
  doc.text('Current Asset Locations', 14, 20);

  // Metadata
  doc.setFontSize(10);
  doc.setTextColor(100);
  doc.text(`Generated: ${getTimestamp()}`, 14, 30);
  doc.text(`Total Assets: ${data.length}`, 14, 36);

  const liveCount = data.filter((d) => getFreshnessStatus(d.last_seen) === 'live').length;
  const todayCount = data.filter((d) => {
    const status = getFreshnessStatus(d.last_seen);
    return status === 'live' || status === 'today';
  }).length;
  const staleCount = data.filter((d) => getFreshnessStatus(d.last_seen) === 'stale').length;

  doc.text(`Live (< 15 min): ${liveCount}`, 14, 42);
  doc.text(`Seen Today: ${todayCount}`, 14, 48);
  doc.text(`Stale (> 7 days): ${staleCount}`, 14, 54);
  doc.setTextColor(0);

  // Table data
  const tableData = data.map((item) => [
    item.asset_identifier,
    item.asset_name,
    item.location_name || 'Unknown',
    formatRelativeTime(item.last_seen),
    getFreshnessLabel(item.last_seen),
  ]);

  // Add table
  autoTable(doc, {
    head: [['Asset ID', 'Name', 'Location', 'Last Seen', 'Status']],
    body: tableData,
    startY: 62,
    styles: {
      fontSize: 8,
      cellPadding: 2,
    },
    headStyles: {
      fillColor: [37, 99, 235], // blue-600
      textColor: 255,
      fontStyle: 'bold',
    },
    alternateRowStyles: {
      fillColor: [245, 245, 245], // gray-100
    },
    columnStyles: {
      0: { cellWidth: 30 }, // Asset ID
      1: { cellWidth: 40 }, // Name
      2: { cellWidth: 40 }, // Location
      3: { cellWidth: 35 }, // Last Seen
      4: { cellWidth: 25 }, // Status
    },
  });

  // Page numbers
  const pageCount = doc.getNumberOfPages();
  for (let i = 1; i <= pageCount; i++) {
    doc.setPage(i);
    doc.setFontSize(8);
    doc.setTextColor(150);
    doc.text(`Page ${i} of ${pageCount}`, doc.internal.pageSize.width / 2, doc.internal.pageSize.height - 10, {
      align: 'center',
    });
  }

  const blob = doc.output('blob');
  return {
    blob,
    filename: `current-locations_${getDateString()}.pdf`,
    mimeType: 'application/pdf',
  };
}

/**
 * Generate Excel workbook from current locations
 */
export function generateCurrentLocationsExcel(data: CurrentLocationItem[]): ExportResult {
  const wb = XLSX.utils.book_new();

  // Main data sheet
  const sheetData = data.map((item) => ({
    'Asset ID': item.asset_identifier,
    Name: item.asset_name,
    Location: item.location_name || 'Unknown',
    'Last Seen': formatTimestampForExport(item.last_seen),
    Status: getFreshnessLabel(item.last_seen),
  }));

  const ws = XLSX.utils.json_to_sheet(sheetData);
  ws['!cols'] = [
    { wch: 20 }, // Asset ID
    { wch: 30 }, // Name
    { wch: 25 }, // Location
    { wch: 22 }, // Last Seen
    { wch: 10 }, // Status
  ];

  XLSX.utils.book_append_sheet(wb, ws, 'Current Locations');

  // Summary sheet
  const liveCount = data.filter((d) => getFreshnessStatus(d.last_seen) === 'live').length;
  const todayCount = data.filter((d) => {
    const status = getFreshnessStatus(d.last_seen);
    return status === 'live' || status === 'today';
  }).length;
  const staleCount = data.filter((d) => getFreshnessStatus(d.last_seen) === 'stale').length;

  const summaryData = [
    { Metric: 'Report Generated', Value: getTimestamp() },
    { Metric: 'Total Assets', Value: data.length },
    { Metric: 'Live (< 15 min)', Value: liveCount },
    { Metric: 'Seen Today', Value: todayCount },
    { Metric: 'Stale (> 7 days)', Value: staleCount },
  ];

  const summaryWS = XLSX.utils.json_to_sheet(summaryData);
  summaryWS['!cols'] = [{ wch: 20 }, { wch: 30 }];
  XLSX.utils.book_append_sheet(wb, summaryWS, 'Summary');

  const wbout = XLSX.write(wb, { bookType: 'xlsx', type: 'array', compression: true });
  const blob = new Blob([wbout], {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  });

  return {
    blob,
    filename: `current-locations_${getDateString()}.xlsx`,
    mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  };
}

/**
 * Generate CSV from current locations
 */
export function generateCurrentLocationsCSV(data: CurrentLocationItem[]): ExportResult {
  const headers = ['Asset ID', 'Name', 'Location', 'Last Seen', 'Status'];
  let content = headers.join(',') + '\n';

  data.forEach((item) => {
    const row = [
      `"${item.asset_identifier}"`,
      `"${(item.asset_name || '').replace(/"/g, '""')}"`,
      `"${(item.location_name || 'Unknown').replace(/"/g, '""')}"`,
      formatTimestampForExport(item.last_seen),
      getFreshnessLabel(item.last_seen),
    ];
    content += row.join(',') + '\n';
  });

  const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' });
  return {
    blob,
    filename: `current-locations_${getDateString()}.csv`,
    mimeType: 'text/csv',
  };
}

// ============================================
// Asset History Export
// ============================================

/**
 * Generate PDF report from asset history
 */
export function generateAssetHistoryPDF(
  data: AssetHistoryItem[],
  assetName: string,
  assetIdentifier: string
): ExportResult {
  const doc = new jsPDF();

  // Header
  doc.setFontSize(20);
  doc.text('Asset Movement History', 14, 20);

  // Asset info
  doc.setFontSize(12);
  doc.setTextColor(0);
  doc.text(`Asset: ${assetName}`, 14, 32);
  doc.setFontSize(10);
  doc.setTextColor(100);
  doc.text(`ID: ${assetIdentifier}`, 14, 38);
  doc.text(`Generated: ${getTimestamp()}`, 14, 44);
  doc.text(`Total Movements: ${data.length}`, 14, 50);
  doc.setTextColor(0);

  // Table data
  const tableData = data.map((item) => [
    formatTimestampForExport(item.timestamp),
    item.location_name || 'Unknown',
    item.duration_seconds ? formatDuration(item.duration_seconds) : 'Ongoing',
  ]);

  // Add table
  autoTable(doc, {
    head: [['Timestamp', 'Location', 'Duration']],
    body: tableData,
    startY: 58,
    styles: {
      fontSize: 9,
      cellPadding: 3,
    },
    headStyles: {
      fillColor: [37, 99, 235], // blue-600
      textColor: 255,
      fontStyle: 'bold',
    },
    alternateRowStyles: {
      fillColor: [245, 245, 245], // gray-100
    },
    columnStyles: {
      0: { cellWidth: 55 }, // Timestamp
      1: { cellWidth: 80 }, // Location
      2: { cellWidth: 35 }, // Duration
    },
  });

  // Page numbers
  const pageCount = doc.getNumberOfPages();
  for (let i = 1; i <= pageCount; i++) {
    doc.setPage(i);
    doc.setFontSize(8);
    doc.setTextColor(150);
    doc.text(`Page ${i} of ${pageCount}`, doc.internal.pageSize.width / 2, doc.internal.pageSize.height - 10, {
      align: 'center',
    });
  }

  const blob = doc.output('blob');
  const sanitizedName = assetName.replace(/[^a-zA-Z0-9-_]/g, '-');
  return {
    blob,
    filename: `${sanitizedName}-history_${getDateString()}.pdf`,
    mimeType: 'application/pdf',
  };
}

/**
 * Generate Excel workbook from asset history
 */
export function generateAssetHistoryExcel(
  data: AssetHistoryItem[],
  assetName: string,
  assetIdentifier: string
): ExportResult {
  const wb = XLSX.utils.book_new();

  // Main data sheet
  const sheetData = data.map((item) => ({
    Timestamp: formatTimestampForExport(item.timestamp),
    Location: item.location_name || 'Unknown',
    Duration: item.duration_seconds ? formatDuration(item.duration_seconds) : 'Ongoing',
  }));

  const ws = XLSX.utils.json_to_sheet(sheetData);
  ws['!cols'] = [
    { wch: 22 }, // Timestamp
    { wch: 30 }, // Location
    { wch: 15 }, // Duration
  ];

  XLSX.utils.book_append_sheet(wb, ws, 'Movement History');

  // Summary sheet
  const uniqueLocations = new Set(data.map((d) => d.location_name || 'Unknown')).size;
  const totalDuration = data.reduce((sum, d) => sum + (d.duration_seconds || 0), 0);

  const summaryData = [
    { Metric: 'Report Generated', Value: getTimestamp() },
    { Metric: 'Asset Name', Value: assetName },
    { Metric: 'Asset ID', Value: assetIdentifier },
    { Metric: 'Total Movements', Value: data.length },
    { Metric: 'Unique Locations', Value: uniqueLocations },
    { Metric: 'Total Time Tracked', Value: formatDuration(totalDuration) },
  ];

  const summaryWS = XLSX.utils.json_to_sheet(summaryData);
  summaryWS['!cols'] = [{ wch: 20 }, { wch: 30 }];
  XLSX.utils.book_append_sheet(wb, summaryWS, 'Summary');

  const wbout = XLSX.write(wb, { bookType: 'xlsx', type: 'array', compression: true });
  const blob = new Blob([wbout], {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  });

  const sanitizedName = assetName.replace(/[^a-zA-Z0-9-_]/g, '-');
  return {
    blob,
    filename: `${sanitizedName}-history_${getDateString()}.xlsx`,
    mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  };
}

/**
 * Generate CSV from asset history
 */
export function generateAssetHistoryCSV(
  data: AssetHistoryItem[],
  assetName: string
): ExportResult {
  const headers = ['Asset', 'Timestamp', 'Location', 'Duration'];
  let content = headers.join(',') + '\n';

  data.forEach((item) => {
    const row = [
      `"${assetName.replace(/"/g, '""')}"`,
      formatTimestampForExport(item.timestamp),
      `"${(item.location_name || 'Unknown').replace(/"/g, '""')}"`,
      item.duration_seconds ? formatDuration(item.duration_seconds) : 'Ongoing',
    ];
    content += row.join(',') + '\n';
  });

  const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' });
  const sanitizedName = assetName.replace(/[^a-zA-Z0-9-_]/g, '-');
  return {
    blob,
    filename: `${sanitizedName}-history_${getDateString()}.csv`,
    mimeType: 'text/csv',
  };
}
