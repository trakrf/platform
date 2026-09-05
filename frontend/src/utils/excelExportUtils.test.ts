import { describe, it, expect, vi } from 'vitest';
import * as XLSX from 'xlsx';
import { generateInventoryCSV, generateInventoryExcel } from './excelExportUtils';
import type { TagInfo } from '../stores/tagStore';

// Mock shareUtils
vi.mock('@/utils/shareUtils', () => ({
  getDateString: () => '2026-03-23',
  getTimestamp: () => '3/23/2026, 12:00:00 PM',
}));

// Helper to read blob content in jsdom environment
async function readBlobAsText(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsText(blob);
  });
}

function makeTag(overrides: Partial<TagInfo> = {}): TagInfo {
  return {
    epc: 'DEADBEEF',
    displayEpc: 'DEADBEEF',
    count: 3,
    source: 'rfid' as const,
    type: 'asset' as const,
    ...overrides,
  };
}

describe('generateInventoryCSV', () => {
  it('has correct column headers matching asset export format', async () => {
    const result = generateInventoryCSV([makeTag()], null);
    const text = await readBlobAsText(result.blob);
    const headerLine = text.split('\n')[0];
    expect(headerLine).toBe('Asset ID,Name,Description,Location,Tag ID,PC,TID,User Data,RSSI (dBm),Count,Last Seen');
    expect(result.mimeType).toBe('text/csv');
    expect(result.filename).toContain('inventory_');
  });

  it('includes Asset ID when tag has assetIdentifier', async () => {
    const tag = makeTag({
      assetIdentifier: 'ASSET-0003',
      assetName: 'Laptop',
      locationName: 'Warehouse A',
    });

    const result = generateInventoryCSV([tag], null);
    const text = await readBlobAsText(result.blob);
    const lines = text.trim().split('\n');

    // Check headers
    expect(lines[0]).toBe('Asset ID,Name,Description,Location,Tag ID,PC,TID,User Data,RSSI (dBm),Count,Last Seen');

    // Check data row contains asset info
    expect(lines[1]).toContain('"ASSET-0003"');
    expect(lines[1]).toContain('"Laptop"');
    expect(lines[1]).toContain('"Warehouse A"');
    expect(lines[1]).toContain('"DEADBEEF"');
  });

  it('leaves Asset ID empty when tag has no assetIdentifier', async () => {
    const tag = makeTag({ assetIdentifier: undefined });

    const result = generateInventoryCSV([tag], null);
    const text = await readBlobAsText(result.blob);
    const lines = text.trim().split('\n');

    // First field should be empty (no Asset ID)
    expect(lines[1].startsWith(',')).toBe(true);
  });

  it('puts the description in the Description column, not the Name column', async () => {
    // The Description column used to be a hardcoded empty string, and Name
    // carried `assetName || description` to compensate. That made an asset's
    // description invisible in CSV while Excel and PDF both exported it. The
    // compensation went away with the bug (TRA-1251).
    const tag = makeTag({
      assetName: undefined,
      description: 'From CSV',
    });

    const result = generateInventoryCSV([tag], null);
    const text = await readBlobAsText(result.blob);
    const fields = text.trim().split('\n')[1].split(',');

    expect(fields[1], 'Name must not borrow the description').toBe('');
    expect(fields[2]).toBe('"From CSV"');
  });

  it('keeps Name and Description independent when both are set', async () => {
    const tag = makeTag({ assetName: 'Laptop', description: 'Dev laptop' });

    const result = generateInventoryCSV([tag], null);
    const text = await readBlobAsText(result.blob);
    const fields = text.trim().split('\n')[1].split(',');

    expect(fields[1]).toBe('"Laptop"');
    expect(fields[2]).toBe('"Dev laptop"');
  });

  describe('tag data columns (TRA-1251)', () => {
    it('exports PC, TID and User Data', async () => {
      const result = generateInventoryCSV([makeTag()], null);
      const text = await readBlobAsText(result.blob);

      expect(text.split('\n')[0]).toBe(
        'Asset ID,Name,Description,Location,Tag ID,PC,TID,User Data,RSSI (dBm),Count,Last Seen'
      );
    });

    it('renders PC as four hex digits', async () => {
      // 0x3000 says 96-bit EPC and 0x4000 says 128-bit. Rendering it as 12288
      // would make the one thing this column exists to answer unreadable.
      const tag = makeTag({ pc: 0x3000 });

      const result = generateInventoryCSV([tag], null);
      const text = await readBlobAsText(result.blob);

      expect(text).toContain('0x3000');
      expect(text).not.toContain('12288');
    });

    it('carries bank data through verbatim', async () => {
      const tag = makeTag({
        tid: 'E2801160600002071D3C0B9A',
        userData: 'DEADBEEF12345678',
      });

      const result = generateInventoryCSV([tag], null);
      const text = await readBlobAsText(result.blob);

      expect(text).toContain('"E2801160600002071D3C0B9A"');
      expect(text).toContain('"DEADBEEF12345678"');
    });

    it('leaves the new columns empty for a tag read without capture', async () => {
      const result = generateInventoryCSV([makeTag()], null);
      const text = await readBlobAsText(result.blob);
      const fields = text.trim().split('\n')[1].split(',');

      // PC, TID, User Data — all blank rather than "undefined" or "0x0000"
      expect(fields[5]).toBe('');
      expect(fields[6]).toBe('');
      expect(fields[7]).toBe('');
    });
  });
});

/**
 * The xlsx download path.
 *
 * ⚠ This is a SEPARATE function from generateInventoryCSV, in the same file,
 * and it carried its own copy of the same two defects — a hardcoded empty
 * Description and a Name column falling back to the description to compensate.
 * The CSV half was fixed first and the Excel half was missed entirely, which is
 * exactly what an export of real scan data then showed: eight columns, no bank
 * data. ShareModal routes 'xlsx' downloads here and only CSV to the other
 * function, so testing one proves nothing about the other.
 */
describe('generateInventoryExcel', () => {
  /**
   * Read the Inventory sheet back out of the generated workbook.
   *
   * Goes through FileReader rather than Blob.arrayBuffer() because jsdom's Blob
   * does not implement the latter usefully — it yields an empty buffer, which
   * parses into a workbook with no rows and makes every assertion below fail
   * with "cannot read properties of undefined" no matter what the export does.
   */
  async function readInventorySheet(blob: Blob): Promise<Record<string, unknown>[]> {
    const buffer = await new Promise<ArrayBuffer>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as ArrayBuffer);
      reader.onerror = () => reject(reader.error);
      reader.readAsArrayBuffer(blob);
    });
    const wb = XLSX.read(new Uint8Array(buffer), { type: 'array' });
    return XLSX.utils.sheet_to_json(wb.Sheets['Inventory'], { defval: '' });
  }

  it('exports PC, TID and User Data', async () => {
    const tag = makeTag({
      pc: 0x3000,
      tid: 'E2801160600002071D3C0B9A',
      userData: 'DEADBEEF12345678',
    });

    const rows = await readInventorySheet(generateInventoryExcel([tag], null).blob);

    expect(Object.keys(rows[0])).toEqual([
      'Asset ID', 'Name', 'Description', 'Location', 'Tag ID',
      'PC', 'TID', 'User Data',
      'RSSI (dBm)', 'Count', 'Last Seen'
    ]);
    expect(rows[0]['PC']).toBe('0x3000');
    expect(rows[0]['TID']).toBe('E2801160600002071D3C0B9A');
    expect(rows[0]['User Data']).toBe('DEADBEEF12345678');
  });

  it('puts the description in the Description column, not the Name column', async () => {
    const tag = makeTag({ assetName: undefined, description: 'From CSV' });

    const rows = await readInventorySheet(generateInventoryExcel([tag], null).blob);

    expect(rows[0]['Name'], 'Name must not borrow the description').toBe('');
    expect(rows[0]['Description']).toBe('From CSV');
  });

  it('keeps Name and Description independent when both are set', async () => {
    const tag = makeTag({ assetName: 'Laptop', description: 'Dev laptop' });

    const rows = await readInventorySheet(generateInventoryExcel([tag], null).blob);

    expect(rows[0]['Name']).toBe('Laptop');
    expect(rows[0]['Description']).toBe('Dev laptop');
  });

  it('leaves the capture columns blank for a scan without them', async () => {
    const rows = await readInventorySheet(generateInventoryExcel([makeTag()], null).blob);

    expect(rows[0]['PC']).toBe('');
    expect(rows[0]['TID']).toBe('');
    expect(rows[0]['User Data']).toBe('');
  });
});
