/**
 * Asset Export Utilities
 *
 * Generates PDF, Excel, and CSV exports for asset lists.
 * Follows patterns from excelExportUtils.ts and pdfExportUtils.ts
 */

import * as XLSX from 'xlsx';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';
import type { Asset } from '@/types/assets';
import type { ExportResult } from '@/types/export';
import { getDateString, getTimestamp } from '@/utils/shareUtils';
import { useLocationStore } from '@/stores/locations/locationStore';

/**
 * Resolve location name from cache
 */
function getLocationName(locationId: number | null): string {
  if (!locationId) return '';
  const location = useLocationStore.getState().cache.byId.get(locationId);
  return location?.name || '';
}

/**
 * Generate PDF report from assets
 */
export function generateAssetPDF(assets: Asset[]): ExportResult {
  const doc = new jsPDF();

  // Header
  doc.setFontSize(20);
  doc.text('Asset Report', 14, 20);

  // Metadata
  doc.setFontSize(10);
  doc.setTextColor(100);
  doc.text(`Generated: ${getTimestamp()}`, 14, 30);
  doc.text(`Total Assets: ${assets.length}`, 14, 36);
  doc.text(`Active: ${assets.filter((a) => a.is_active).length}`, 14, 42);
  doc.text(`Inactive: ${assets.filter((a) => !a.is_active).length}`, 14, 48);
  doc.setTextColor(0);

  // Table data
  const tableData = assets.map((asset) => [
    asset.identifier,
    asset.name || '',
    asset.type,
    asset.identifiers?.map((t) => t.value).join(', ') || '',
    getLocationName(asset.current_location_id),
    asset.is_active ? 'Active' : 'Inactive',
    asset.description || '',
  ]);

  // Add table
  autoTable(doc, {
    head: [['Asset ID', 'Name', 'Type', 'Tag ID(s)', 'Location', 'Status', 'Description']],
    body: tableData,
    startY: 55,
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
      0: { cellWidth: 25 }, // Asset ID
      1: { cellWidth: 30 }, // Name
      2: { cellWidth: 18 }, // Type
      3: { cellWidth: 35 }, // Tag ID(s)
      4: { cellWidth: 25 }, // Location
      5: { cellWidth: 15 }, // Status
      6: { cellWidth: 'auto' }, // Description
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
    filename: `assets_${getDateString()}.pdf`,
    mimeType: 'application/pdf',
  };
}

/**
 * Generate Excel workbook from assets
 */
export function generateAssetExcel(assets: Asset[]): ExportResult {
  const wb = XLSX.utils.book_new();

  // Main assets sheet
  const data = assets.map((asset) => ({
    'Asset ID': asset.identifier,
    Name: asset.name || '',
    Type: asset.type,
    'Tag ID(s)': asset.identifiers?.map((t) => t.value).join(', ') || '',
    Location: getLocationName(asset.current_location_id),
    Status: asset.is_active ? 'Active' : 'Inactive',
    Description: asset.description || '',
    Created: asset.created_at ? new Date(asset.created_at).toLocaleDateString() : '',
  }));

  const ws = XLSX.utils.json_to_sheet(data);
  ws['!cols'] = [
    { wch: 15 }, // Asset ID
    { wch: 25 }, // Name
    { wch: 12 }, // Type
    { wch: 40 }, // Tag ID(s)
    { wch: 20 }, // Location
    { wch: 10 }, // Status
    { wch: 35 }, // Description
    { wch: 12 }, // Created
  ];

  XLSX.utils.book_append_sheet(wb, ws, 'Assets');

  // Summary sheet
  const summaryData = [
    { Metric: 'Report Generated', Value: getTimestamp() },
    { Metric: 'Total Assets', Value: assets.length },
    { Metric: 'Active', Value: assets.filter((a) => a.is_active).length },
    { Metric: 'Inactive', Value: assets.filter((a) => !a.is_active).length },
    { Metric: 'With Tags', Value: assets.filter((a) => a.identifiers && a.identifiers.length > 0).length },
    { Metric: 'Without Tags', Value: assets.filter((a) => !a.identifiers || a.identifiers.length === 0).length },
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
    filename: `assets_${getDateString()}.xlsx`,
    mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  };
}

/**
 * Generate CSV from assets
 */
export function generateAssetCSV(assets: Asset[]): ExportResult {
  const headers = ['Asset ID', 'Name', 'Type', 'Tag ID(s)', 'Location', 'Status', 'Description', 'Created'];
  let content = headers.join(',') + '\n';

  assets.forEach((asset) => {
    const row = [
      `"${asset.identifier}"`,
      `"${(asset.name || '').replace(/"/g, '""')}"`,
      asset.type,
      `"${asset.identifiers?.map((t) => t.value).join('; ') || ''}"`,
      `"${getLocationName(asset.current_location_id).replace(/"/g, '""')}"`,
      asset.is_active ? 'Active' : 'Inactive',
      `"${(asset.description || '').replace(/"/g, '""')}"`,
      asset.created_at ? new Date(asset.created_at).toLocaleDateString() : '',
    ];
    content += row.join(',') + '\n';
  });

  const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' });
  return {
    blob,
    filename: `assets_${getDateString()}.csv`,
    mimeType: 'text/csv',
  };
}
