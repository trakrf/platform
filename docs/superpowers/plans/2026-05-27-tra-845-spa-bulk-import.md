# TRA-845 SPA Bulk Import Re-expose — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Re-expose the existing (orphaned) bulk asset import UI in the SPA with an Import button in the Assets header, add client-side `.xlsx` support via SheetJS, and bring the sample template back into sync with the current backend schema.

**Architecture:** A new pure adapter `xlsxToCsv.ts` converts `.xlsx` files to CSV `File` blobs that the existing `assetsApi.uploadCSV(file)` call already accepts unchanged. The orphaned `BulkUploadModal` is wired up via a new Import button in `AssetsScreen` header, sitting next to the existing `ShareButton`. Sample templates (`.csv` and new `.xlsx`) are rewritten to match the current backend schema (`external_key, name, valid_from, valid_to, is_active, description, tags` — no `identifier`, no `type`, no `location`).

**Tech Stack:** React 18 + TypeScript, Vitest, Tailwind, `xlsx` (SheetJS, already in `frontend/package.json` for exports), `lucide-react` icons.

**Spec:** `docs/superpowers/specs/2026-05-27-tra-845-spa-bulk-import-design.md`

---

## Task 1: Rewrite stale sample CSV

The existing `frontend/public/bulk_assets_sample.csv` uses `identifier`, `type` columns that no longer match the backend (`backend/internal/util/csv/helpers.go:153-234` defines `external_key, name, valid_from, valid_to, is_active, description, tags`). Replace it.

**Files:**
- Modify: `frontend/public/bulk_assets_sample.csv`

- [ ] **Step 1: Replace file contents**

Write the file with this exact content:

```csv
external_key,name,description,valid_from,valid_to,is_active,tags
LAPTOP-001,Dell XPS 15 - Engineering,Development laptop for software engineering team,2024-01-15,2026-12-31,true,E280119020004F3D94E00C91
LAPTOP-002,MacBook Pro 16 - Design,High-performance laptop for graphic design team,2024-02-01,2026-12-31,true,E280119020004F3D94E00C92
RFID-TAG-1001,RFID Tag #1001,Passive RFID tag for asset tracking,2024-01-01,2027-12-31,true,
DESK-A-101,Standing Desk - Office A101,Ergonomic standing desk in office suite A,2024-03-15,2029-12-31,true,E280119020004F3D94E00C93
MONITOR-LG-001,LG UltraWide 34 inch,Curved ultrawide monitor for development,2024-04-10,2027-06-30,true,
BADGE-EMP-001,Employee Badge - John Doe,RFID-enabled employee access badge,2024-02-15,2025-02-15,true,"E280119020004F3D94E00C96,E280119020004F3D94E00C97"
PALLET-WH-1001,Warehouse Pallet #1001,Standard 48x40 wooden pallet,2024-01-01,2026-12-31,true,
```

Notes for the engineer:
- Drop the `type` column (asset_type dropped per project memory).
- Rename `identifier` → `external_key`.
- Normalize `is_active` to `true`/`false` (the old file mixed `yes`/`1`/`true`; backend accepts true/false reliably, so use that to model the canonical sample).
- Keep the multi-tag quoted example to demonstrate the format.

- [ ] **Step 2: Commit**

```bash
git add frontend/public/bulk_assets_sample.csv
git commit -m "fix(bulk-import): refresh sample CSV to current schema (TRA-845)"
```

---

## Task 2: Generate sample XLSX from the CSV

Produce `frontend/public/bulk_assets_sample.xlsx` containing exactly the same rows as the CSV. Use a one-shot Node command via the installed `xlsx` package — no scripts directory addition; the binary is the only output committed.

**Files:**
- Create: `frontend/public/bulk_assets_sample.xlsx`

- [ ] **Step 1: Generate the xlsx**

Run from project root:

```bash
cd frontend && node -e "
const XLSX = require('xlsx');
const fs = require('fs');
const csv = fs.readFileSync('public/bulk_assets_sample.csv', 'utf8');
const wb = XLSX.read(csv, { type: 'string' });
wb.SheetNames[0] = 'Assets';
wb.Sheets['Assets'] = wb.Sheets[wb.SheetNames[0]] || wb.Sheets['Sheet1'];
XLSX.writeFile(wb, 'public/bulk_assets_sample.xlsx', { bookType: 'xlsx' });
console.log('wrote public/bulk_assets_sample.xlsx');
" && cd ..
```

- [ ] **Step 2: Verify file exists and is non-trivial**

```bash
ls -l frontend/public/bulk_assets_sample.xlsx
```
Expected: file present, size > 4KB.

- [ ] **Step 3: Spot-check by re-reading it**

```bash
cd frontend && node -e "
const XLSX = require('xlsx');
const wb = XLSX.readFile('public/bulk_assets_sample.xlsx');
console.log('sheets:', wb.SheetNames);
console.log(XLSX.utils.sheet_to_csv(wb.Sheets[wb.SheetNames[0]]).split('\n').slice(0, 3).join('\n'));
" && cd ..
```
Expected: one sheet (`Assets`), header row begins `external_key,name,description,...`.

- [ ] **Step 4: Commit**

```bash
git add frontend/public/bulk_assets_sample.xlsx
git commit -m "feat(bulk-import): add sample XLSX template (TRA-845)"
```

---

## Task 3: Adapter `xlsxToCsv` — failing test first

Pure adapter that converts an `.xlsx` `File` into a CSV `File` (so it slots into `assetsApi.uploadCSV(file: File)` unchanged), plus a list of soft warnings and hard errors.

**Files:**
- Create: `frontend/src/utils/bulkImport/xlsxToCsv.ts`
- Create: `frontend/src/utils/bulkImport/xlsxToCsv.test.ts`

- [ ] **Step 1: Write the failing test file**

Create `frontend/src/utils/bulkImport/xlsxToCsv.test.ts`:

```typescript
import { describe, it, expect } from 'vitest';
import * as XLSX from 'xlsx';
import { parseXlsxToCsv, RECOGNIZED_COLUMNS } from './xlsxToCsv';

/**
 * Build an in-memory xlsx File from a 2D array of rows. Row 0 is headers.
 */
function buildXlsxFile(rows: (string | number | boolean)[][], opts?: { extraSheets?: string[]; filename?: string }): File {
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
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
just frontend test src/utils/bulkImport/xlsxToCsv.test.ts
```
Expected: FAIL with module not found error for `./xlsxToCsv`.

---

## Task 4: Adapter `xlsxToCsv` — implementation

**Files:**
- Create: `frontend/src/utils/bulkImport/xlsxToCsv.ts`

- [ ] **Step 1: Implement the adapter**

Create `frontend/src/utils/bulkImport/xlsxToCsv.ts`:

```typescript
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

/**
 * Parse an .xlsx File into a CSV File ready for assetsApi.uploadCSV(file).
 * Headers are matched case-insensitively against RECOGNIZED_COLUMNS;
 * unknown columns are dropped with a warning. Multi-sheet workbooks use the
 * first sheet and emit a warning naming the ignored sheets.
 */
export async function parseXlsxToCsv(file: File): Promise<XlsxParseResult> {
  const warnings: string[] = [];
  const errors: string[] = [];

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

  return { csvFile, warnings, errors };
}
```

- [ ] **Step 2: Run the test to verify it passes**

```bash
just frontend test src/utils/bulkImport/xlsxToCsv.test.ts
```
Expected: all 8 tests PASS.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/utils/bulkImport/
git commit -m "feat(bulk-import): xlsx-to-csv client-side adapter (TRA-845)"
```

---

## Task 5: Wire xlsx + warnings into `BulkUploadModal`

Update the modal to accept `.xlsx` files, route them through the adapter, surface warnings/errors before upload, fix the stale CSV format documentation, and offer the XLSX template download alongside the CSV one.

**Files:**
- Modify: `frontend/src/components/assets/BulkUploadModal.tsx`

- [ ] **Step 1: Import the adapter and add warnings state**

At the top of `BulkUploadModal.tsx`, add:

```typescript
import { parseXlsxToCsv, RECOGNIZED_COLUMNS } from '@/utils/bulkImport/xlsxToCsv';
```

In the component body, alongside the existing `useState` calls (around line 14-18), add:

```typescript
const [uploadFile, setUploadFile] = useState<File | null>(null); // CSV or converted-from-xlsx
const [warnings, setWarnings] = useState<string[]>([]);
```

`file` continues to hold the user-selected file for display (name/size shown to user); `uploadFile` holds the CSV-ready `File` actually sent to the API.

- [ ] **Step 2: Replace `handleFileSelect` to handle both formats**

Replace the existing `handleFileSelect` (lines 21-41) with:

```typescript
const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
  const selectedFile = e.target.files?.[0];
  setError(null);
  setSuccess(null);
  setWarnings([]);
  setUploadFile(null);

  if (!selectedFile) return;

  if (selectedFile.size > 5 * 1024 * 1024) {
    setError('File size must be less than 5MB');
    setFile(null);
    return;
  }

  const lower = selectedFile.name.toLowerCase();
  if (lower.endsWith('.csv')) {
    setFile(selectedFile);
    setUploadFile(selectedFile);
    return;
  }

  if (lower.endsWith('.xlsx')) {
    setFile(selectedFile);
    const result = await parseXlsxToCsv(selectedFile);
    setWarnings(result.warnings);
    if (result.errors.length > 0) {
      setError(result.errors.join(' '));
      setUploadFile(null);
      return;
    }
    setUploadFile(result.csvFile);
    return;
  }

  setError('Please select a CSV or XLSX file');
  setFile(null);
};
```

- [ ] **Step 3: Update `handleUpload` to use `uploadFile`**

Replace the body of `handleUpload` (lines 43-90) to send `uploadFile` rather than `file`:

```typescript
const handleUpload = async () => {
  if (!uploadFile) {
    setError('Please select a file');
    return;
  }

  setLoading(true);
  setError(null);
  setSuccess(null);

  try {
    setActiveJobId(null);

    const response = await assetsApi.uploadCSV(uploadFile);

    if (!response.data || !response.data.job_id) {
      throw new Error('Invalid response from server. Bulk upload API may not be available.');
    }

    setActiveJobId(response.data.job_id);
    toast.success('Upload started! Tracking progress...');

    setFile(null);
    setUploadFile(null);
    setWarnings([]);
    setError(null);
    setSuccess(null);

    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }

    setTimeout(() => {
      onClose();
      onSuccess?.();
    }, 1000);
  } catch (err: any) {
    if (err.code === 'ERR_NETWORK' || err.message?.includes('Network Error')) {
      setError('Cannot connect to server. Please check your connection and try again.');
    } else if (err.response?.status === 404) {
      setError('Bulk upload API endpoint not found. The backend may not be running.');
    } else if (err.response?.status >= 500) {
      setError('Server error. Please try again later.');
    } else {
      setError(err.message || 'Upload failed. Please try again.');
    }
  } finally {
    setLoading(false);
  }
};
```

- [ ] **Step 4: Update `handleClose` to also clear new state**

Replace `handleClose` (lines 92-102) with:

```typescript
const handleClose = () => {
  if (!loading) {
    setFile(null);
    setUploadFile(null);
    setWarnings([]);
    setError(null);
    setSuccess(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
    onClose();
  }
};
```

- [ ] **Step 5: Fix stale schema doc and add XLSX template link**

Replace the requirements block (lines 135-156) with:

```tsx
<div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4">
  <div className="flex items-start justify-between mb-2 gap-3">
    <h3 className="text-sm font-semibold text-blue-900 dark:text-blue-300">
      File Format Requirements
    </h3>
    <div className="flex gap-2">
      <a
        href="/bulk_assets_sample.csv"
        download="bulk_assets_sample.csv"
        className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-blue-700 dark:text-blue-300 bg-blue-100 dark:bg-blue-900/40 hover:bg-blue-200 dark:hover:bg-blue-900/60 border border-blue-300 dark:border-blue-700 rounded-lg transition-colors"
      >
        <Download className="h-3.5 w-3.5" />
        Sample CSV
      </a>
      <a
        href="/bulk_assets_sample.xlsx"
        download="bulk_assets_sample.xlsx"
        className="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-blue-700 dark:text-blue-300 bg-blue-100 dark:bg-blue-900/40 hover:bg-blue-200 dark:hover:bg-blue-900/60 border border-blue-300 dark:border-blue-700 rounded-lg transition-colors"
      >
        <Download className="h-3.5 w-3.5" />
        Sample XLSX
      </a>
    </div>
  </div>
  <ul className="text-sm text-blue-800 dark:text-blue-400 space-y-1 list-disc list-inside">
    <li>Accepts <strong>.csv</strong> or <strong>.xlsx</strong> (first sheet only)</li>
    <li>Required columns: <code>external_key</code>, <code>name</code></li>
    <li>Optional columns: <code>{RECOGNIZED_COLUMNS.filter(c => c !== 'external_key' && c !== 'name').join(', ')}</code></li>
    <li><code>is_active</code>: <code>true</code> or <code>false</code></li>
    <li><code>tags</code>: comma-separated RFID tag values (quote the field if it contains commas)</li>
    <li>Maximum file size: 5MB</li>
  </ul>
</div>
```

- [ ] **Step 6: Render a warnings band below errors**

Immediately after the `{error && (...)}` block (around line 158-163), add:

```tsx
{warnings.length > 0 && (
  <div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4 flex items-start gap-3">
    <AlertCircle className="h-5 w-5 text-yellow-600 dark:text-yellow-400 flex-shrink-0 mt-0.5" />
    <div className="text-sm text-yellow-800 dark:text-yellow-300 space-y-1">
      {warnings.map((w, i) => (
        <p key={i}>{w}</p>
      ))}
    </div>
  </div>
)}
```

- [ ] **Step 7: Update file input `accept` and visible copy**

Change the input `accept` attribute (line 210) from `accept=".csv"` to `accept=".csv,.xlsx"`.

Update the helper text inside the dropzone (line 197): change `Click to select CSV file` to `Click to select CSV or XLSX file`.

- [ ] **Step 8: Update Upload button to use `uploadFile`**

In the Upload button (line 227-234), change `disabled={!file || loading}` to `disabled={!uploadFile || loading}`. This means when a user selects an `.xlsx` that produces errors, the Upload button stays disabled until they pick a valid file.

- [ ] **Step 9: Run typecheck and unit tests**

```bash
just frontend typecheck && just frontend test src/components/assets/
```
Expected: typecheck passes; any existing modal tests still pass.

- [ ] **Step 10: Commit**

```bash
git add frontend/src/components/assets/BulkUploadModal.tsx
git commit -m "feat(bulk-import): accept xlsx + refresh schema doc in modal (TRA-845)"
```

---

## Task 6: Add Import button to Assets header

Place an `Import` button next to `ShareButton` that opens the existing `BulkUploadModal`. The modal is already mounted in `AssetsScreen.tsx`; only the trigger is missing.

**Files:**
- Modify: `frontend/src/components/AssetsScreen.tsx`

- [ ] **Step 1: Import the Upload icon**

Update the `lucide-react` import (line 2) to include `Upload`:

```typescript
import { Plus, Package, Upload } from 'lucide-react';
```

- [ ] **Step 2: Add the Import button in the header row**

In the header div (currently lines 127-135), insert the Import button between `AssetSearchSort` and `ShareButton`:

```tsx
<div className="flex items-center gap-3">
  <div className="flex-1">
    <AssetSearchSort />
  </div>
  <button
    type="button"
    onClick={() => setIsBulkUploadOpen(true)}
    className="inline-flex items-center gap-1.5 px-3 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500 transition-colors"
    aria-label="Import assets from CSV or XLSX"
  >
    <Upload className="h-4 w-4" />
    <span className="hidden sm:inline">Import</span>
  </button>
  <ShareButton
    onFormatSelect={openExport}
    disabled={filteredAssets.length === 0}
  />
</div>
```

The `hidden sm:inline` on the label keeps the button compact on mobile (icon-only) and full-width on small-and-up screens, matching the visual weight of `ShareButton`.

- [ ] **Step 3: Run typecheck**

```bash
just frontend typecheck
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/AssetsScreen.tsx
git commit -m "feat(assets): re-expose bulk Import button in header (TRA-845)"
```

---

## Task 7: Full validation pass

**Files:** none

- [ ] **Step 1: Run combined frontend validate**

```bash
just frontend validate
```
Expected: lint, typecheck, and unit tests all pass.

- [ ] **Step 2: If any failure, fix and re-commit**

Address each failure root-cause; do not silence linters or skip tests. If the failure points to drift between the adapter / modal / schema doc, fix in place. Re-run `just frontend validate` until clean.

- [ ] **Step 3: Quick local browser smoke (optional but recommended)**

```bash
just frontend dev
```

In browser:
1. Navigate to Assets screen — confirm Import button is visible in the header next to Share.
2. Click Import — modal opens.
3. Click "Sample CSV" — file downloads, headers begin `external_key,name,...`.
4. Click "Sample XLSX" — file downloads, opens in your spreadsheet program with the same rows.
5. Upload the unmodified sample CSV — toast appears, job kicks off (or backend rejects if local backend not running — that's fine for the smoke).
6. Upload the unmodified sample XLSX — same flow; modal should not show warnings.
7. Build a 2-sheet xlsx and upload — multi-sheet warning appears.

Kill the dev server when done.

---

## Task 8: Push branch and open PR

**Files:** none

- [ ] **Step 1: Push the branch**

```bash
git push -u origin feat/tra-845-spa-bulk-import-reexpose
```

- [ ] **Step 2: Open PR**

```bash
gh pr create --title "feat(assets): re-expose bulk import with xlsx support (TRA-845)" --body "$(cat <<'EOF'
## Summary
- Re-expose the orphaned bulk asset import in the SPA via a new "Import" button in the Assets header (next to Share). Does not reinstate the TRA-259 Single/Bulk chooser — FAB still opens single-asset modal directly.
- Accept `.xlsx` files in addition to `.csv`. Client-side conversion via SheetJS feeds the existing `/api/v1/assets/bulk` backend unchanged.
- Refresh stale sample template (`public/bulk_assets_sample.csv` was using `identifier`/`type` from a deprecated schema). Add `bulk_assets_sample.xlsx` mirror.

## Test plan
- [x] `just frontend validate` (lint + typecheck + unit tests)
- [ ] Preview deploy: open Assets screen → Import → upload sample CSV → job completes, assets appear
- [ ] Preview deploy: same flow with sample XLSX
- [ ] Preview deploy: multi-sheet xlsx surfaces the warning band
- [ ] Preview deploy: xlsx with only unrecognized columns surfaces the error band and blocks upload

## Out of scope
- Public API promotion of `/api/v1/assets/bulk` — tracked in TRA-746 with webhooks in v1.1
- Server-side xlsx parsing
- Reinstating the chooser interstitial (deliberate TRA-259 reversal)
- Docs in `trakrf-docs` — separate session
EOF
)"
```

- [ ] **Step 3: Verify PR is open and preview deploy is triggered**

```bash
gh pr view --json url,number,headRefName,checks
```
Expected: PR URL returned; `sync-preview` check kicks off (see `.github/workflows/sync-preview.yml`).

---

## Task 9: Preview verification (after preview deploy completes)

Once `https://app.preview.trakrf.id` is updated:

- [ ] **Step 1: Verify Import button + modal**

Log into preview, navigate to Assets, confirm Import button visible. Open the modal; both sample download links work.

- [ ] **Step 2: Verify CSV upload end-to-end**

Upload the unmodified `bulk_assets_sample.csv`. Confirm:
- Toast: "Upload started! Tracking progress..."
- `GlobalUploadAlert` shows progress.
- After completion, refresh assets list — sample assets present.

- [ ] **Step 3: Verify XLSX upload end-to-end**

Same flow with `bulk_assets_sample.xlsx`. Should behave identically (no warnings, since the sample is single-sheet with only recognized columns).

- [ ] **Step 4: Verify location-column rejection**

Construct a CSV locally with a `location` column added and upload. Backend should reject with a clear error per `backend/internal/models/asset/asset.go:148-149`.

- [ ] **Step 5: Report verification results in PR**

Check off the test plan items in the PR description. Do not mark TRA-845 Done in Linear yet — keep In Progress pending docs work in `trakrf-docs` (per project memory `feedback_ticket_not_done_until_docs`).

---

## Self-Review Notes

- **Spec coverage check:** Re-expose entry point ✓ (Task 6); xlsx parsing ✓ (Tasks 3-4); location-column drop ✓ (sample CSV drops it, adapter drops it as unknown, backend rejects it — verified Task 9 step 4); compatibility pass ✓ (Task 9); sample template ✓ (Tasks 1-2); error UX ✓ (Task 5 step 6).
- **Placeholder scan:** clean.
- **Type consistency:** `RECOGNIZED_COLUMNS` exported from `xlsxToCsv.ts` and imported by `BulkUploadModal.tsx`; `parseXlsxToCsv` returns `{ csvFile, warnings, errors }` consistently across test and impl; `uploadFile: File | null` used consistently in modal.
