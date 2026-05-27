# TRA-845 Re-expose Asset Bulk Import in SPA — Design

**Date:** 2026-05-27
**Ticket:** TRA-845 (High)
**Status:** Approved, pending implementation plan
**Related:** TRA-259 (hid the chooser), TRA-120 (backend bulk endpoint), TRA-121 (frontend bulk UI), TRA-222 (`identifiers` column), TRA-746 (public-API promotion — out of scope here)

## Problem

A live customer onboarding call on 2026-05-27 surfaced demand for spreadsheet upload to load assets. The capability exists end-to-end — `/api/v1/assets/bulk` backend, async job processing, frontend modal with job-status polling — but the SPA entry point was deleted by TRA-259 along with the Single/Bulk chooser interstitial (the FAB now opens the single-asset modal directly). The bulk modal in `frontend/src/components/AssetsScreen.tsx:193-197` is mounted but its open-state setter is never called: orphaned.

The machinery has also been unreachable for ~4 months across significant contract churn: Identifier → "Asset ID" label rename (TRA-259), asset `external_key` auto-mint, the `identifiers` column (TRA-222), PublicAssetView reshape, and the location-column removal (TRA-734/TRA-799). The existing sample template `frontend/public/bulk_assets_sample.csv` uses stale columns (`identifier`, `type`) that no longer match the backend schema (`external_key`, no `asset_type`). Drift is likely elsewhere too.

Customers will hand over Excel files. Shipping CSV-only is a half-feature and a credibility hit when the next call asks "can I just upload my xlsx" and the answer is "convert it first."

## Scope

Re-expose bulk import as a discoverable SPA action without reinstating the Single/Bulk chooser interstitial that TRA-259 deliberately removed. Accept `.xlsx` in addition to `.csv`, client-side. Fix the stale sample template. Verify end-to-end against the current backend contract before declaring done.

## Decisions

- **Entry point:** an "Import" button in the Assets screen header next to `ShareButton` (export). Symmetric placement with the existing export affordance; no overflow menu, no secondary FAB. Keeps the FAB single-purpose for the dominant flow (single-asset add) per TRA-259's friction win.
- **xlsx flow:** parse client-side with the already-installed `xlsx` (SheetJS) lib, convert to a CSV `Blob` in-memory, submit through the existing `useBulkUpload` hook. Backend contract unchanged. No backend changes for xlsx.
- **`.xls` (legacy binary):** not supported. Ticket marks it optional; declined.
- **Sample template:** ship both `public/bulk_assets_sample.csv` (rewritten with current schema) and `public/bulk_assets_sample.xlsx` (mirror).
- **Schema authoritative:** `external_key, name, valid_from, valid_to, is_active, description, tags` (matches `backend/internal/util/csv/helpers.go:153-234`). No `location`, no `asset_type`, no `identifier`.

## Architecture

### Files touched

| File | Change |
|---|---|
| `frontend/src/components/AssetsScreen.tsx` | Add Import button in header; wire `setIsBulkUploadOpen(true)`. |
| `frontend/src/components/assets/BulkUploadModal.tsx` | Accept `.xlsx` in file input; route through new adapter; render warnings band. Update template-download link to offer CSV + XLSX. |
| `frontend/src/utils/bulkImport/xlsxToCsv.ts` (new) | Pure adapter: `File` → `{ csvBlob: Blob, warnings: string[], errors: string[] }`. |
| `frontend/src/utils/bulkImport/xlsxToCsv.test.ts` (new) | Unit tests for the adapter. |
| `frontend/public/bulk_assets_sample.csv` | Rewrite with current schema. |
| `frontend/public/bulk_assets_sample.xlsx` | New. Mirrors the CSV. |
| Backend | No changes expected. Verify only. |

### Adapter contract (`xlsxToCsv.ts`)

```
parseXlsxToCsv(file: File): Promise<{
  csvBlob: Blob | null,   // null on hard error
  warnings: string[],     // surfaced before upload, not blocking
  errors: string[]        // surfaced before upload, blocks upload
}>
```

**Recognized columns (case-insensitive, trimmed):** `external_key, name, valid_from, valid_to, is_active, description, tags`. Unknown columns are dropped with a warning listing them.

**Behavior:**
- Read workbook, take the first worksheet. If `workbook.SheetNames.length > 1`, push a warning naming the sheets that will be ignored.
- Treat row 1 as headers. Empty sheet (no header row OR no data rows) → error.
- If no recognized columns are present → error.
- Convert rows to CSV with `XLSX.utils.sheet_to_csv` after pruning columns, then wrap in a `Blob` with `text/csv`.
- The CSV path bypasses the adapter entirely — files matching `.csv`/`text/csv` upload directly as today.

### Modal warnings UX

Above the existing upload button, render any `warnings[]` from the adapter as a yellow band ("Heads up: …") and any `errors[]` as a red band that disables the upload button. CSV uploads have no adapter-produced warnings; the band is hidden in that case. Reuse existing error display patterns from the modal — do not introduce new toast plumbing.

### Header Import button

Visual treatment matches `ShareButton` so the two read as a pair. The button opens the modal via the existing `setIsBulkUploadOpen` setter; no other plumbing changes — the modal, hook, polling, and result rendering are unchanged.

## Compatibility pass

End-to-end verification against preview after the frontend lands, before marking the ticket done:

1. CSV upload with current schema columns → 202, job ID returned.
2. xlsx upload (same logical rows) → adapter converts → backend 202.
3. Job status polls progress → COMPLETED with row counts.
4. Assets appear in the list view.
5. Upload with a `location` column header → backend rejects clearly (existing behavior per `backend/internal/models/asset/asset.go:148-149`); adapter strips it client-side but the test confirms backend rejection if it ever reaches the wire.
6. Sample template downloads (both formats) and a round-trip upload of the unmodified template succeeds.

No proactive backend refactor. If drift is found, scope-up via a follow-up ticket unless trivial.

## Testing

- **Adapter unit tests:** happy-path single sheet; multi-sheet (uses first, warns); unknown columns (drops, warns); empty sheet (errors); no recognized columns (errors); header case-insensitivity.
- **Modal smoke test:** Import button opens modal; CSV path unchanged; xlsx path renders warnings band; upload button disabled when adapter returns errors.
- **Manual against preview:** the six-step compatibility pass above.

## Out of scope

- Promoting `/api/v1/assets/bulk` to the public API (TRA-746, ships with webhooks in v1.1).
- Server-side `.xlsx` parsing on the backend endpoint itself.
- Reinstating the Single/Bulk chooser interstitial (deliberate TRA-259 reversal — do not undo).
- Location column anywhere (TRA-734 master-data / scan-data bifurcation).
- `.xls` (legacy binary).
- Documentation in `trakrf-docs` — separate session per [[feedback_docs_prs_separate_checkout]] / [[feedback_ticket_not_done_until_docs]]. Linear stays In Progress until docs land.
