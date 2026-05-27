import { describe, it, expect } from 'vitest';
import * as XLSX from 'xlsx';
import { parseXlsxToCsv, RECOGNIZED_COLUMNS } from './xlsxToCsv';

// jsdom (as of v26) does not implement Blob.prototype.arrayBuffer. All
// supported browsers do. Polyfill via FileReader for the test env only.
if (typeof Blob.prototype.arrayBuffer !== 'function') {
  Blob.prototype.arrayBuffer = function (): Promise<ArrayBuffer> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as ArrayBuffer);
      reader.onerror = () => reject(reader.error);
      reader.readAsArrayBuffer(this);
    });
  };
}
if (typeof Blob.prototype.text !== 'function') {
  Blob.prototype.text = function (): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as string);
      reader.onerror = () => reject(reader.error);
      reader.readAsText(this);
    });
  };
}

function buildXlsxFile(
  rows: (string | number | boolean)[][],
  opts?: { extraSheets?: string[]; filename?: string }
): File {
  const wb = XLSX.utils.book_new();
  const ws = XLSX.utils.aoa_to_sheet(rows);
  XLSX.utils.book_append_sheet(wb, ws, 'Sheet1');
  for (const name of opts?.extraSheets ?? []) {
    XLSX.utils.book_append_sheet(wb, XLSX.utils.aoa_to_sheet([['foo']]), name);
  }
  const buf = XLSX.write(wb, { type: 'array', bookType: 'xlsx' }) as ArrayBuffer;
  return new File([buf], opts?.filename ?? 'test.xlsx', {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  });
}

async function readBlobText(blob: Blob): Promise<string> {
  return await blob.text();
}

describe('parseXlsxToCsv', () => {
  it('converts a valid single-sheet xlsx to a CSV File with recognized columns', async () => {
    const file = buildXlsxFile([
      ['external_key', 'name', 'is_active'],
      ['A-1', 'Asset One', 'true'],
      ['A-2', 'Asset Two', 'false'],
    ]);
    const result = await parseXlsxToCsv(file);
    expect(result.errors).toEqual([]);
    expect(result.warnings).toEqual([]);
    expect(result.csvFile).not.toBeNull();
    expect(result.csvFile!.type).toBe('text/csv');
    const text = await readBlobText(result.csvFile!);
    const lines = text.trim().split('\n');
    expect(lines[0]).toBe('external_key,name,is_active');
    expect(lines[1]).toBe('A-1,Asset One,true');
    expect(lines[2]).toBe('A-2,Asset Two,false');
  });

  it('matches headers case-insensitively and trims whitespace', async () => {
    const file = buildXlsxFile([
      ['  External_Key ', 'NAME'],
      ['A-1', 'Asset One'],
    ]);
    const result = await parseXlsxToCsv(file);
    expect(result.errors).toEqual([]);
    const text = await readBlobText(result.csvFile!);
    expect(text.trim().split('\n')[0]).toBe('external_key,name');
  });

  it('warns about a multi-sheet workbook and uses the first sheet', async () => {
    const file = buildXlsxFile(
      [
        ['external_key', 'name'],
        ['A-1', 'Asset One'],
      ],
      { extraSheets: ['Notes', 'Pricing'] }
    );
    const result = await parseXlsxToCsv(file);
    expect(result.errors).toEqual([]);
    expect(result.warnings.some((w) => w.includes('Notes') && w.includes('Pricing'))).toBe(true);
    expect(result.csvFile).not.toBeNull();
  });

  it('drops unknown columns and warns, listing them', async () => {
    const file = buildXlsxFile([
      ['external_key', 'name', 'location', 'asset_type'],
      ['A-1', 'Asset One', 'L-1', 'asset'],
    ]);
    const result = await parseXlsxToCsv(file);
    expect(result.errors).toEqual([]);
    const warning = result.warnings.find((w) => w.toLowerCase().includes('ignored'));
    expect(warning).toBeDefined();
    expect(warning).toMatch(/location/);
    expect(warning).toMatch(/asset_type/);
    const text = await readBlobText(result.csvFile!);
    expect(text.trim().split('\n')[0]).toBe('external_key,name');
    expect(text).not.toMatch(/location/);
    expect(text).not.toMatch(/asset_type/);
  });

  it('errors on a sheet with no recognized columns', async () => {
    const file = buildXlsxFile([
      ['foo', 'bar'],
      ['1', '2'],
    ]);
    const result = await parseXlsxToCsv(file);
    expect(result.csvFile).toBeNull();
    expect(result.errors.some((e) => e.toLowerCase().includes('no recognized'))).toBe(true);
  });

  it('errors on an empty sheet (no rows at all)', async () => {
    const file = buildXlsxFile([]);
    const result = await parseXlsxToCsv(file);
    expect(result.csvFile).toBeNull();
    expect(result.errors.some((e) => e.toLowerCase().includes('empty'))).toBe(true);
  });

  it('errors on header-only sheet (no data rows)', async () => {
    const file = buildXlsxFile([['external_key', 'name']]);
    const result = await parseXlsxToCsv(file);
    expect(result.csvFile).toBeNull();
    expect(result.errors.some((e) => e.toLowerCase().includes('no data'))).toBe(true);
  });

  it('exports RECOGNIZED_COLUMNS matching the spec', () => {
    expect(RECOGNIZED_COLUMNS).toEqual([
      'external_key',
      'name',
      'valid_from',
      'valid_to',
      'is_active',
      'description',
      'tags',
    ]);
  });
});
