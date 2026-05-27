import * as XLSX from 'xlsx';

export const RECOGNIZED_COLUMNS = [
  'external_key',
  'name',
  'valid_from',
  'valid_to',
  'is_active',
  'description',
  'tags',
] as const;

export type RecognizedColumn = (typeof RECOGNIZED_COLUMNS)[number];

export interface XlsxParseResult {
  csvFile: File | null;
  warnings: string[];
  errors: string[];
}

const RECOGNIZED_SET = new Set<string>(RECOGNIZED_COLUMNS);

function normalizeHeader(raw: unknown): string {
  return String(raw ?? '').trim().toLowerCase();
}

export async function parseXlsxToCsv(file: File): Promise<XlsxParseResult> {
  const warnings: string[] = [];

  let workbook: XLSX.WorkBook;
  try {
    const buf = await file.arrayBuffer();
    workbook = XLSX.read(buf, { type: 'array' });
  } catch (err) {
    return {
      csvFile: null,
      warnings,
      errors: [`Could not read spreadsheet: ${(err as Error).message ?? 'unknown error'}`],
    };
  }

  if (workbook.SheetNames.length === 0) {
    return { csvFile: null, warnings, errors: ['The spreadsheet is empty.'] };
  }

  const firstSheetName = workbook.SheetNames[0];
  if (workbook.SheetNames.length > 1) {
    const ignored = workbook.SheetNames.slice(1).join(', ');
    warnings.push(
      `Workbook contains multiple sheets; only the first sheet ("${firstSheetName}") will be imported. Ignored: ${ignored}.`
    );
  }

  const sheet = workbook.Sheets[firstSheetName];
  const rows: unknown[][] = XLSX.utils.sheet_to_json(sheet, {
    header: 1,
    blankrows: false,
    defval: '',
  });

  if (rows.length === 0) {
    return { csvFile: null, warnings, errors: ['The spreadsheet is empty.'] };
  }

  const headerRow = rows[0];
  const dataRows = rows.slice(1);
  if (dataRows.length === 0) {
    return {
      csvFile: null,
      warnings,
      errors: ['The spreadsheet has a header row but no data rows.'],
    };
  }

  const keptIndexes: number[] = [];
  const keptHeaders: string[] = [];
  const dropped: string[] = [];
  for (let i = 0; i < headerRow.length; i++) {
    const normalized = normalizeHeader(headerRow[i]);
    if (!normalized) continue;
    if (RECOGNIZED_SET.has(normalized)) {
      keptIndexes.push(i);
      keptHeaders.push(normalized);
    } else {
      dropped.push(String(headerRow[i]).trim());
    }
  }

  if (keptHeaders.length === 0) {
    return {
      csvFile: null,
      warnings,
      errors: [
        `No recognized columns found. Expected one or more of: ${RECOGNIZED_COLUMNS.join(', ')}.`,
      ],
    };
  }

  if (dropped.length > 0) {
    warnings.push(
      `Ignored unknown columns: ${dropped.join(', ')}. Recognized columns: ${RECOGNIZED_COLUMNS.join(', ')}.`
    );
  }

  const pruned: unknown[][] = [keptHeaders];
  for (const row of dataRows) {
    pruned.push(keptIndexes.map((i) => row[i] ?? ''));
  }

  const prunedSheet = XLSX.utils.aoa_to_sheet(pruned);
  const csvString = XLSX.utils.sheet_to_csv(prunedSheet);

  const outName = file.name.replace(/\.xlsx?$/i, '') + '.csv';
  const csvFile = new File([csvString], outName, { type: 'text/csv' });

  return { csvFile, warnings, errors: [] };
}
