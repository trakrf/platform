/**
 * PDF export utilities for inventory reports
 */

import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';
import type { TagInfo } from '../stores/tagStore';
import type { ExportResult } from '../types/export';
import { getDateString, getTimestamp } from './shareUtils';

/**
 * Generate a PDF report from inventory tags
 */
export function generateInventoryPDF(
  tags: TagInfo[],
  reconciliationList: string[] | null
): ExportResult {
  const doc = new jsPDF();
  
  // Add header
  doc.setFontSize(20);
  doc.text('Inventory Report', 14, 20);
  
  // Add metadata
  doc.setFontSize(10);
  doc.setTextColor(100);
  doc.text(`Generated: ${getTimestamp()}`, 14, 30);
  doc.text(`Total Tags: ${tags.length}`, 14, 36);
  
  // Add reconciliation info if available
  if (reconciliationList) {
    const reconciledCount = tags.filter(t => t.reconciled === true).length;
    const missingCount = tags.filter(t => t.reconciled === false).length;
    const notOnListCount = tags.filter(t => t.reconciled === null).length;
    
    doc.text(`Reconciliation Status:`, 14, 42);
    doc.text(`  • Found: ${reconciledCount}`, 14, 48);
    doc.text(`  • Missing: ${missingCount}`, 14, 54);
    doc.text(`  • Not on list: ${notOnListCount}`, 14, 60);
  }
  
  // Reset text color
  doc.setTextColor(0);
  
  // Prepare table data
  const tableData = tags.map(tag => {
    const row = [
      tag.displayEpc || tag.epc,
      tag.rssi ? `${tag.rssi}` : 'N/A',
      tag.count.toString(),
      tag.timestamp ? new Date(tag.timestamp).toLocaleTimeString() : 'N/A'
    ];
    
    // Add reconciliation status if available
    if (reconciliationList) {
      const status = tag.reconciled === true ? 'Found' : 
                     tag.reconciled === false ? 'Missing' : 
                     'Not on list';
      row.push(status);
    }
    
    // Add description if available
    if (tag.description) {
      row.push(tag.description);
    }
    
    return row;
  });
  
  // Prepare headers
  const headers = ['Tag ID', 'RSSI (dBm)', 'Count', 'Last Seen'];
  if (reconciliationList) {
    headers.push('Status');
  }
  if (tags.some(t => t.description)) {
    headers.push('Description');
  }
  
  // Add table using autoTable
  autoTable(doc, {
    head: [headers],
    body: tableData,
    startY: reconciliationList ? 65 : 45,
    styles: { 
      fontSize: 8,
      cellPadding: 2
    },
    headStyles: { 
      fillColor: [37, 99, 235], // blue-600
      textColor: 255,
      fontStyle: 'bold'
    },
    alternateRowStyles: {
      fillColor: [245, 245, 245] // gray-100
    },
    columnStyles: {
      0: { cellWidth: 'auto' }, // Tag ID
      1: { cellWidth: 25, halign: 'right' }, // RSSI
      2: { cellWidth: 20, halign: 'center' }, // Count
      3: { cellWidth: 30 }, // Last Seen
      4: { cellWidth: 25 }, // Status (if present)
      5: { cellWidth: 'auto' } // Description (if present)
    }
  });
  
  // Add page numbers
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
  
  // Generate blob
  const blob = doc.output('blob');
  const filename = `inventory_${getDateString()}.pdf`;
  
  return {
    blob,
    filename,
    mimeType: 'application/pdf'
  };
}