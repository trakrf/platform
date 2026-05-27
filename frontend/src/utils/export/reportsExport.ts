/**
 * Reports Export Utilities
 *
 * Generates PDF, Excel, and CSV exports for report data. After TRA-844 each
 * exporter takes a hydration adapter so the emitted columns include both the
 * resolved asset/location *name* and the public *external_key*, keeping
 * downstream joins working while making the human-readable output usable.
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
// Hydration adapter types (TRA-844)
// ============================================

export interface CurrentLocationsExportOpts {
  getAssetName: (item: CurrentLocationItem) => string;
  getLocationName: (item: CurrentLocationItem) => string;
}

export interface AssetHistoryExportOpts {
  assetName: string;
  assetKey: string;
  getLocationName: (item: AssetHistoryItem) => string;
}

// ============================================
// Locations History Export
// ============================================

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
export function generateCurrentLocationsPDF(
  data: CurrentLocationItem[],
  opts: CurrentLocationsExportOpts
): ExportResult {
  const doc = new jsPDF();

  // Header
  doc.setFontSize(20);
  doc.text('Current Asset Locations', 14, 20);

  // Metadata
  doc.setFontSize(10);
  doc.setTextColor(100);
  doc.text(`Generated: ${getTimestamp()}`, 14, 30);
  doc.text(`Total Assets: ${data.length}`, 14, 36);

  const liveCount = data.filter((d) => getFreshnessStatus(d.asset_last_seen) === 'live').length;
  const todayCount = data.filter((d) => {
    const status = getFreshnessStatus(d.asset_last_seen);
    return status === 'live' || status === 'today';
  }).length;
  const staleCount = data.filter((d) => getFreshnessStatus(d.asset_last_seen) === 'stale').length;

  doc.text(`Live (< 15 min): ${liveCount}`, 14, 42);
  doc.text(`Seen Today: ${todayCount}`, 14, 48);
  doc.text(`Stale (> 7 days): ${staleCount}`, 14, 54);
  doc.setTextColor(0);

  // Table data
  const tableData = data.map((item) => [
    opts.getAssetName(item),
    item.asset_external_key ?? '',
    opts.getLocationName(item),
    item.location_external_key ?? '',
    formatRelativeTime(item.asset_last_seen),
    getFreshnessLabel(item.asset_last_seen),
  ]);

  autoTable(doc, {
    head: [
      [
        'Asset Name',
        'Asset Key',
        'Location Name',
        'Location Key',
        'Last Seen',
        'Status',
      ],
    ],
    body: tableData,
    startY: 62,
    styles: { fontSize: 8, cellPadding: 2 },
    headStyles: {
      fillColor: [37, 99, 235],
      textColor: 255,
      fontStyle: 'bold',
    },
    alternateRowStyles: { fillColor: [245, 245, 245] },
    columnStyles: {
      0: { cellWidth: 36 }, // Asset Name
      1: { cellWidth: 26 }, // Asset Key
      2: { cellWidth: 36 }, // Location Name
      3: { cellWidth: 26 }, // Location Key
      4: { cellWidth: 30 }, // Last Seen
      5: { cellWidth: 18 }, // Status
    },
  });

  // Page numbers
  const pageCount = doc.getNumberOfPages();
  for (let i = 1; i <= pageCount; i++) {
    doc.setPage(i);
    doc.setFontSize(8);
    doc.setTextColor(150);
    doc.text(
      `Page ${i} of ${pageCount}`,
      doc.internal.pageSize.width / 2,
      doc.internal.pageSize.height - 10,
      { align: 'center' }
    );
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
export function generateCurrentLocationsExcel(
  data: CurrentLocationItem[],
  opts: CurrentLocationsExportOpts
): ExportResult {
  const wb = XLSX.utils.book_new();

  const sheetData = data.map((item) => ({
    'Asset Name': opts.getAssetName(item),
    'Asset Key': item.asset_external_key ?? '',
    'Location Name': opts.getLocationName(item),
    'Location Key': item.location_external_key ?? '',
    'Last Seen': formatTimestampForExport(item.asset_last_seen),
    Status: getFreshnessLabel(item.asset_last_seen),
  }));

  const ws = XLSX.utils.json_to_sheet(sheetData);
  ws['!cols'] = [
    { wch: 30 }, // Asset Name
    { wch: 20 }, // Asset Key
    { wch: 30 }, // Location Name
    { wch: 20 }, // Location Key
    { wch: 22 }, // Last Seen
    { wch: 10 }, // Status
  ];

  XLSX.utils.book_append_sheet(wb, ws, 'Locations History');

  const liveCount = data.filter((d) => getFreshnessStatus(d.asset_last_seen) === 'live').length;
  const todayCount = data.filter((d) => {
    const status = getFreshnessStatus(d.asset_last_seen);
    return status === 'live' || status === 'today';
  }).length;
  const staleCount = data.filter((d) => getFreshnessStatus(d.asset_last_seen) === 'stale').length;

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
export function generateCurrentLocationsCSV(
  data: CurrentLocationItem[],
  opts: CurrentLocationsExportOpts
): ExportResult {
  const headers = [
    'Asset Name',
    'Asset Key',
    'Location Name',
    'Location Key',
    'Last Seen',
    'Status',
  ];
  let content = headers.join(',') + '\n';

  data.forEach((item) => {
    const assetName = opts.getAssetName(item);
    const assetKey = item.asset_external_key ?? '';
    const locationName = opts.getLocationName(item);
    const locationKey = item.location_external_key ?? '';
    const row = [
      `"${assetName.replace(/"/g, '""')}"`,
      `"${assetKey.replace(/"/g, '""')}"`,
      `"${locationName.replace(/"/g, '""')}"`,
      `"${locationKey.replace(/"/g, '""')}"`,
      formatTimestampForExport(item.asset_last_seen),
      getFreshnessLabel(item.asset_last_seen),
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
  opts: AssetHistoryExportOpts
): ExportResult {
  const doc = new jsPDF();

  // Header
  doc.setFontSize(20);
  doc.text('Asset Movement History', 14, 20);

  // Asset info
  doc.setFontSize(12);
  doc.setTextColor(0);
  doc.text(`Asset: ${opts.assetName}`, 14, 32);
  doc.setFontSize(10);
  doc.setTextColor(100);
  doc.text(`Key: ${opts.assetKey}`, 14, 38);
  doc.text(`Generated: ${getTimestamp()}`, 14, 44);
  doc.text(`Total Movements: ${data.length}`, 14, 50);
  doc.setTextColor(0);

  const tableData = data.map((item) => [
    opts.assetName,
    opts.assetKey,
    formatTimestampForExport(item.event_observed_at),
    opts.getLocationName(item),
    item.location_external_key ?? '',
    item.duration_seconds ? formatDuration(item.duration_seconds) : 'Ongoing',
  ]);

  autoTable(doc, {
    head: [
      [
        'Asset Name',
        'Asset Key',
        'Timestamp',
        'Location Name',
        'Location Key',
        'Duration',
      ],
    ],
    body: tableData,
    startY: 58,
    styles: { fontSize: 9, cellPadding: 3 },
    headStyles: {
      fillColor: [37, 99, 235],
      textColor: 255,
      fontStyle: 'bold',
    },
    alternateRowStyles: { fillColor: [245, 245, 245] },
    columnStyles: {
      0: { cellWidth: 32 }, // Asset Name
      1: { cellWidth: 24 }, // Asset Key
      2: { cellWidth: 38 }, // Timestamp
      3: { cellWidth: 32 }, // Location Name
      4: { cellWidth: 24 }, // Location Key
      5: { cellWidth: 22 }, // Duration
    },
  });

  // Page numbers
  const pageCount = doc.getNumberOfPages();
  for (let i = 1; i <= pageCount; i++) {
    doc.setPage(i);
    doc.setFontSize(8);
    doc.setTextColor(150);
    doc.text(
      `Page ${i} of ${pageCount}`,
      doc.internal.pageSize.width / 2,
      doc.internal.pageSize.height - 10,
      { align: 'center' }
    );
  }

  const blob = doc.output('blob');
  const sanitizedName = opts.assetName.replace(/[^a-zA-Z0-9-_]/g, '-');
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
  opts: AssetHistoryExportOpts
): ExportResult {
  const wb = XLSX.utils.book_new();

  const sheetData = data.map((item) => ({
    'Asset Name': opts.assetName,
    'Asset Key': opts.assetKey,
    Timestamp: formatTimestampForExport(item.event_observed_at),
    'Location Name': opts.getLocationName(item),
    'Location Key': item.location_external_key ?? '',
    Duration: item.duration_seconds ? formatDuration(item.duration_seconds) : 'Ongoing',
  }));

  const ws = XLSX.utils.json_to_sheet(sheetData);
  ws['!cols'] = [
    { wch: 28 }, // Asset Name
    { wch: 20 }, // Asset Key
    { wch: 22 }, // Timestamp
    { wch: 28 }, // Location Name
    { wch: 20 }, // Location Key
    { wch: 15 }, // Duration
  ];

  XLSX.utils.book_append_sheet(wb, ws, 'Movement History');

  const uniqueLocations = new Set(
    data.map((d) => d.location_external_key || 'Unknown')
  ).size;
  const totalDuration = data.reduce((sum, d) => sum + (d.duration_seconds || 0), 0);

  const summaryData = [
    { Metric: 'Report Generated', Value: getTimestamp() },
    { Metric: 'Asset Name', Value: opts.assetName },
    { Metric: 'Asset Key', Value: opts.assetKey },
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

  const sanitizedName = opts.assetName.replace(/[^a-zA-Z0-9-_]/g, '-');
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
  opts: AssetHistoryExportOpts
): ExportResult {
  const headers = [
    'Asset Name',
    'Asset Key',
    'Timestamp',
    'Location Name',
    'Location Key',
    'Duration',
  ];
  let content = headers.join(',') + '\n';

  data.forEach((item) => {
    const locationName = opts.getLocationName(item);
    const locationKey = item.location_external_key ?? '';
    const row = [
      `"${opts.assetName.replace(/"/g, '""')}"`,
      `"${opts.assetKey.replace(/"/g, '""')}"`,
      formatTimestampForExport(item.event_observed_at),
      `"${locationName.replace(/"/g, '""')}"`,
      `"${locationKey.replace(/"/g, '""')}"`,
      item.duration_seconds ? formatDuration(item.duration_seconds) : 'Ongoing',
    ];
    content += row.join(',') + '\n';
  });

  const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' });
  const sanitizedName = opts.assetName.replace(/[^a-zA-Z0-9-_]/g, '-');
  return {
    blob,
    filename: `${sanitizedName}-history_${getDateString()}.csv`,
    mimeType: 'text/csv',
  };
}
