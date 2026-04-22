# TRA-455: Wrap RLS-Protected Queries in WithOrgTx — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every `s.pool.Query*()` / `s.pool.Exec()` / `s.pool.Begin()` callsite in `backend/internal/storage/*.go` that touches one of the 6 RLS-protected tables (`trakrf.assets`, `bulk_import_jobs`, `identifiers`, `locations`, `scan_devices`, `scan_points`) runs inside `WithOrgTx`. Silent empty reads and 500s on writes (AKS: TRA-438 repro) are resolved.

**Architecture:** Thread `orgID` through signatures (option A — compile-time enforcement). ~48 callsites across 6 files migrate to `s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error { ... })`. Six storage methods that currently lack an `orgID` param gain one — every handler caller already has `orgID` from `middleware.GetRequestOrgID`, and every internal storage caller has it in scope too. Ctx-based plumbing was explicitly rejected — RLS is a multi-tenant security boundary and belongs in the type system, not ambient state.

**Tech Stack:** Go 1.24, pgx/v5, PostgreSQL RLS, existing `backend/internal/storage/transactions.go` `WithOrgTx` helper.

---

## Scope Summary

- **48 offender callsites** (pre-audit)
- **6 methods need signature change** (gain `orgID int` param, callers updated)
- **1 raw tx** in `assets.go` `BatchCreateAssets` (not via `s.pool.Query`) also migrates to `WithOrgTx`
- **1 raw tx** in `inventory.go` `SaveInventoryScans` inserts only into `trakrf.asset_scans` (not RLS-protected) — its tx stays as-is, but the two preceding validation SELECTs (on `trakrf.locations` and `trakrf.assets`) get wrapped
- `reports.go` `ListAssetHistory` / `CountAssetHistory` hit only `trakrf.asset_scans` — **not in the 6-table list, left untouched**
- No `pool.*` callsites exist outside `backend/internal/storage/` (confirmed via grep). `testutil/*.go` pool usage is test-only and exempt.

## File Map

| File | Status | Responsibility |
| --- | --- | --- |
| `backend/internal/storage/assets.go` | modify | Wrap 15 pool callsites + `BatchCreateAssets` raw tx in `WithOrgTx`. Signature changes: `GetAssetByID`, `getAssetWithLocationByID`. |
| `backend/internal/storage/identifiers.go` | modify | Wrap 9 remaining pool callsites. Signature changes: `GetIdentifiersByAssetID`, `GetIdentifiersByLocationID`, `GetIdentifierByID`, `getIdentifiersForAssets`, `getIdentifiersForLocations`. |
| `backend/internal/storage/locations.go` | modify | Wrap 14 pool callsites. Signature changes: `GetLocationByID`, `getLocationWithParentByID`, `scanHierarchyRows` (verify during task — may already take orgID). |
| `backend/internal/storage/bulk_import_jobs.go` | modify | Wrap 4 pool callsites. All methods already take `orgID` — no signature changes. |
| `backend/internal/storage/reports.go` | modify | Wrap 2 pool callsites (`ListCurrentLocations`, `CountCurrentLocations`). Already take `orgID`. |
| `backend/internal/storage/inventory.go` | modify | Wrap 2 validation SELECTs. Leave `asset_scans` raw tx alone. |
| `backend/internal/handlers/**/*.go` | modify | Update callsites of the 6 methods that gain `orgID`. Each handler already has `orgID` in scope from `middleware.GetRequestOrgID`. |
| `backend/internal/storage/rls_sentinel_test.go` | create | Integration test: open a pool with role default `app.current_org_id = 0`, verify each of the 6 tables under WithOrgTx (reads + writes succeed) and without WithOrgTx (reads empty, writes error `42501`). |
| `backend/internal/storage/*_test.go` | modify | Update test callsites for the 6 signature-changed methods. |
| `.github/workflows/<ci workflow>` or `backend/justfile` | modify | Add crude grep-based guard step that fails CI if `s\.pool\.(Query\|Exec\|Begin)` reappears in `internal/storage/*.go` (non-test). Follow-up ticket filed for a proper `forbidigo`/analyzer-based rule. |

No frontend changes. No migrations. Exactly zero schema changes — the bootstrap `ALTER ROLE` sentinel in the infra repo stays (converts "unrecognized parameter" into soft RLS empty/reject; still useful defense-in-depth).

---

## Design Notes

### The migration pattern

**Before:**
```go
func (s *Storage) CreateAsset(ctx context.Context, request asset.Asset) (*asset.Asset, error) {
    var a asset.Asset
    err := s.pool.QueryRow(ctx, query, args...).Scan(&a.ID, ...)
    if err != nil { return nil, fmt.Errorf(...) }
    return &a, nil
}
```

**After:**
```go
func (s *Storage) CreateAsset(ctx context.Context, request asset.Asset) (*asset.Asset, error) {
    var a asset.Asset
    err := s.WithOrgTx(ctx, request.OrgID, func(tx pgx.Tx) error {
        return tx.QueryRow(ctx, query, args...).Scan(&a.ID, ...)
    })
    if err != nil { return nil, fmt.Errorf(...) }
    return &a, nil
}
```

Everything that came from `pool.Query*` becomes `tx.Query*` inside the closure. Everything else (error mapping, struct scanning, result assembly) stays outside the closure unchanged.

### Signature changes

Six methods currently accept a surrogate ID without an `orgID`, relying on a load-bearing comment ("caller MUST have already authorized"). That handshake is exactly the kind of implicit invariant that let TRA-455 exist. Each method gains `orgID int` as the first post-`ctx` param, AND adds `WHERE org_id = $N` to its WHERE clause so the method is self-authorizing:

| Before | After |
| --- | --- |
| `GetAssetByID(ctx, id *int)` | `GetAssetByID(ctx, orgID int, id *int)` |
| `getAssetWithLocationByID(ctx, id int)` | `getAssetWithLocationByID(ctx, orgID, id int)` |
| `GetIdentifiersByAssetID(ctx, assetID int)` | `GetIdentifiersByAssetID(ctx, orgID, assetID int)` |
| `GetIdentifiersByLocationID(ctx, locationID int)` | `GetIdentifiersByLocationID(ctx, orgID, locationID int)` |
| `GetIdentifierByID(ctx, identifierID int)` | `GetIdentifierByID(ctx, orgID, identifierID int)` |
| `getIdentifiersForAssets(ctx, assetIDs []int)` | `getIdentifiersForAssets(ctx, orgID int, assetIDs []int)` |
| `getIdentifiersForLocations(ctx, locationIDs []int)` | `getIdentifiersForLocations(ctx, orgID int, locationIDs []int)` |

Also verify during Task 5 whether `locations.go` helpers need the same treatment.

Delete the "caller MUST have already authorized" comments — the new signature self-documents.

Removable redundancies uncovered by this refactor (delete them when you touch the handler):

- `handlers/assets/assets.go:226` `if view == nil || view.OrgID != orgID {` — the handler-level cross-check against `view.OrgID`. With `GetAssetByID` now org-scoped, a cross-org lookup returns `nil` and the existing `nil` check is all you need.
- `storage/assets.go:832` `GetAssetWithLocationByIDForTest` test shim — confirm it's still needed once `getAssetWithLocationByID` takes `orgID` (test callers may be fine calling the lowercase version directly inside the package).

### Testing strategy

The load-bearing test is `rls_sentinel_test.go`:

1. Open a dedicated pgxpool backed by a role that has `app.current_org_id = '0'` as its *only* session default (the sentinel). Mirror production behavior.
2. Using **raw pool** (no `WithOrgTx`): assert every one of the 6 RLS tables returns zero rows on `SELECT` and fails with SQLSTATE `42501` on `INSERT` of any row whose `org_id != 0`. This is the canary — if RLS policies are ever dropped, this test fails loudly.
3. Using **`WithOrgTx`** with a real `orgID`: assert read and write both work. This proves the helper sets the GUC correctly and that RLS accepts policies under a legitimate org context.
4. For each of the 6 storage files, pick one representative CRUD operation and call it through the regular `Storage` struct under the sentinel pool. Expect success. This is the regression gate — any storage method that still uses raw `pool.*` will fail here once the pool runs under sentinel mode.

The existing test suite continues running under its current test role (whatever permissions it has today) — no churn in existing tests beyond the signature updates.

### Commit discipline

One commit per file migration, in this order (identifiers first because it already has the WithOrgTx pattern, assets last because it's the most intricate):

1. `test(tra-455): add RLS sentinel-mode integration test (RED)` — new test file exists but is expected to fail for any file not yet migrated
2. `refactor(tra-455): wrap identifiers.go queries in WithOrgTx`
3. `refactor(tra-455): wrap bulk_import_jobs.go queries in WithOrgTx`
4. `refactor(tra-455): wrap reports.go RLS queries in WithOrgTx`
5. `refactor(tra-455): wrap inventory.go validation queries in WithOrgTx`
6. `refactor(tra-455): wrap locations.go queries in WithOrgTx`
7. `refactor(tra-455): wrap assets.go queries and batch tx in WithOrgTx`
8. `chore(tra-455): add CI guard banning pool.Query/Exec/Begin in storage`
9. (if needed) `refactor(tra-455): drop now-redundant handler-level org checks`

After commit 7, the sentinel test passes end-to-end.

---

## Task 1: Branch + worktree setup

- [ ] **Step 1: Create worktree on feature branch**

Run from project root:

```bash
git -C /home/mike/platform fetch origin
git -C /home/mike/platform worktree add .worktrees/tra-455 -b miks2u/tra-455-wrap-rls-protected-queries-in-withorgtx-fixes-500s-on-asset origin/main
cd /home/mike/platform/.worktrees/tra-455
```

Expected: worktree created, branch checked out tracking `origin/main`.

- [ ] **Step 2: Verify clean tree**

Run: `git status --short`

Expected: empty output.

- [ ] **Step 3: Verify backend builds before any changes**

Run: `just backend build`

Expected: success, no errors.

---

## Task 2: RLS sentinel integration test (RED)

**Files:**
- Create: `backend/internal/storage/rls_sentinel_test.go`
- Reference: `backend/internal/storage/storage_test.go` (existing integration test harness for pool setup patterns)
- Reference: `backend/internal/testutil/database.go` (how existing fixtures construct pools)

- [ ] **Step 1: Read existing test harness**

Read `backend/internal/testutil/database.go` and `backend/internal/storage/storage_test.go` to understand the current connection/role setup. Identify: (a) the DSN/role used by tests, (b) whether that role already has `app.current_org_id` set, (c) how a test creates a fresh pool if needed.

- [ ] **Step 2: Write the test skeleton**

Create `backend/internal/storage/rls_sentinel_test.go`. Build-tag it `//go:build integration` if the repo uses that convention (check `storage_test.go`). Set up:

- A helper `newSentinelPool(t *testing.T) *pgxpool.Pool` that opens a pool with session-level `SET app.current_org_id = '0'` via `AfterConnect`. If the test DB's role already has a sentinel default, skip the `AfterConnect`; otherwise use `AfterConnect` to `SET` (NOT `SET LOCAL`) so the GUC persists across queries on each acquired conn.
- A helper `assertReadEmpty(t, pool, table)` that runs `SELECT count(*) FROM trakrf.<table>` and asserts zero.
- A helper `assertInsertRejected(t, pool, table, insertSQL, args)` that runs INSERT with `org_id` ≠ 0, expects `*pgconn.PgError` with `Code == "42501"`.

- [ ] **Step 3: Write the raw-pool assertions**

For each of the 6 RLS tables, call the two helpers. Example:

```go
func TestRLS_SentinelMode_RawPoolReadsEmpty(t *testing.T) {
    pool := newSentinelPool(t)
    defer pool.Close()
    for _, tbl := range []string{"assets", "bulk_import_jobs", "identifiers", "locations", "scan_devices", "scan_points"} {
        t.Run(tbl, func(t *testing.T) { assertReadEmpty(t, pool, tbl) })
    }
}

func TestRLS_SentinelMode_RawPoolWritesRejected(t *testing.T) {
    pool := newSentinelPool(t)
    defer pool.Close()

    cases := []struct{ tbl, sql string; args []any }{
        {"assets", `INSERT INTO trakrf.assets (name, identifier, type, org_id) VALUES ('x','x','asset',1)`, nil},
        // ... one minimal INSERT per table; use a valid non-zero org_id
    }
    for _, c := range cases {
        t.Run(c.tbl, func(t *testing.T) { assertInsertRejected(t, pool, c.tbl, c.sql, c.args) })
    }
}
```

- [ ] **Step 4: Write the WithOrgTx-works assertion**

Add a test that constructs a `*Storage` backed by the sentinel pool, seeds a row via a known-working path (whatever the existing tests use to create an org / asset fixture — may need a helper that uses `WithOrgTx` directly to bootstrap fixtures under sentinel mode), then calls representative storage methods and expects success:

- `s.CreateAsset(...)`
- `s.GetAssetByID(ctx, orgID, &id)` (will be post-signature-change — OK, this task is the RED test so it references the post-refactor API)
- `s.LookupByTagValues(...)`
- `s.ListAllLocations(...)`
- `s.CreateBulkImportJob(...)`

This test should compile-fail or runtime-fail today (methods haven't been updated). That's expected and drives the migration.

- [ ] **Step 5: Run the new test and document the failure**

Run: `just backend test ./internal/storage/... -run RLS_SentinelMode`

Expected: FAIL (compile error from signature mismatch, or runtime RLS violations, or both). Record the failure output in the task checklist — it becomes the baseline the subsequent migration tasks eliminate.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/storage/rls_sentinel_test.go
git commit -m "$(cat <<'EOF'
test(tra-455): add RLS sentinel-mode integration test (RED)

Drives the WithOrgTx migration: verifies raw pool under sentinel role default
returns empty reads and rejects writes with 42501; verifies WithOrgTx-wrapped
storage methods work under the same session context.

Currently failing — migration tasks 3–8 resolve the failures.
EOF
)"
```

---

## Task 3: Migrate identifiers.go

**Files:**
- Modify: `backend/internal/storage/identifiers.go`
- Modify: `backend/internal/storage/identifiers_test.go` (update callsites for signature-changed methods)
- Modify: `backend/internal/storage/identifiers_crossorg_test.go`
- Modify: callers of the 5 signature-changed methods (grep below)

- [ ] **Step 1: Identify all callers of signature-changed methods**

Run:

```bash
grep -rn "GetIdentifiersByAssetID\|GetIdentifiersByLocationID\|GetIdentifierByID\|getIdentifiersForAssets\|getIdentifiersForLocations" backend/ --include="*.go"
```

Expected output lists callsites in storage internals, handlers, and tests. Record the list — every callsite needs an `orgID` arg inserted.

- [ ] **Step 2: Update `identifiers.go` method signatures**

Change the 5 method signatures (see Design Notes table). Each gains `orgID int` immediately after `ctx`. In the query, add `AND org_id = $N` to the WHERE clause.

- [ ] **Step 3: Wrap each `s.pool.*` call in `WithOrgTx`**

9 callsites: lines 24, 54, 137, 159, 175, 230, 265, 333, 443 (pre-refactor line numbers — they'll shift).

Pattern for each: replace `s.pool.QueryRow(ctx, ...)` / `s.pool.Query(ctx, ...)` / `s.pool.Exec(ctx, ...)` with:

```go
err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
    // tx.QueryRow / tx.Query / tx.Exec with the same args
    return scan/assign logic
})
```

For `Query` that returns `pgx.Rows`: iterate rows *inside* the closure (rows cannot outlive the tx).

- [ ] **Step 4: Update internal callers inside `identifiers.go`**

Lines 452, 459 (post-refactor): `s.GetAssetByID(ctx, assetID)` → `s.GetAssetByID(ctx, orgID, assetID)`. `orgID` is already in scope in `LookupByTagValue` / `LookupByTagValues`.

- [ ] **Step 5: Update handler callers**

For every hit from Step 1 in `backend/internal/handlers/**`, thread `orgID` through. Every handler has `orgID, _ := middleware.GetRequestOrgID(req)` already; just pass it to the storage call.

- [ ] **Step 6: Update test callers**

For every hit from Step 1 in `_test.go` files, add `orgID` arg. Tests already construct org fixtures — use the fixture's `OrgID`.

- [ ] **Step 7: Build and run tests**

```bash
just backend test ./internal/storage/... -run Identifiers
just backend test ./internal/handlers/...
```

Expected: pass. Sentinel test (from Task 2) now passes for identifiers-specific subtests.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/storage/identifiers.go backend/internal/storage/identifiers_test.go backend/internal/storage/identifiers_crossorg_test.go backend/internal/handlers/
git commit -m "refactor(tra-455): wrap identifiers.go queries in WithOrgTx

Thread orgID through 5 methods that previously relied on caller-side
authorization. All 9 remaining pool.Query/Exec callsites now run inside
WithOrgTx; RLS policies fire correctly for reads and writes."
```

---

## Task 4: Migrate bulk_import_jobs.go

**Files:**
- Modify: `backend/internal/storage/bulk_import_jobs.go`
- Modify: any `bulk_import_jobs` tests (likely none standalone — covered by handler integration tests)

- [ ] **Step 1: Wrap 4 pool callsites in WithOrgTx**

Lines 23, 52, 89, 113. All methods already take `orgID` (or `jobID` where orgID is implicit via FK — verify by reading each signature). For methods that take `jobID` only: gain `orgID int` param like Task 3.

Check: `UpdateBulkImportJobProgress(ctx, jobID, ...)` and `UpdateBulkImportJobStatus(ctx, jobID, ...)` at lines 89, 113 — do they take orgID today? If not, add it. Callers (handlers/bulkimport/*) already have orgID.

- [ ] **Step 2: Update callers if signatures changed**

Run grep; update any handler/caller that lost a matching signature.

- [ ] **Step 3: Build and test**

```bash
just backend build
just backend test ./internal/handlers/bulkimport/...
```

Expected: pass.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/storage/bulk_import_jobs.go backend/internal/handlers/bulkimport/
git commit -m "refactor(tra-455): wrap bulk_import_jobs.go queries in WithOrgTx"
```

---

## Task 5: Migrate reports.go

**Files:**
- Modify: `backend/internal/storage/reports.go` (2 callsites: lines 49, 118)
- Note: lines 220, 265 touch only `trakrf.asset_scans` (not RLS-protected) — leave as-is

- [ ] **Step 1: Wrap ListCurrentLocations (line 49) in WithOrgTx**

Already takes `orgID`. Wrap the `s.pool.Query` call; iterate rows inside the closure.

- [ ] **Step 2: Wrap CountCurrentLocations (line 118) in WithOrgTx**

Already takes `orgID`. Straightforward `QueryRow.Scan` inside the closure.

- [ ] **Step 3: Build and test**

```bash
just backend test ./internal/handlers/reports/...
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/storage/reports.go
git commit -m "refactor(tra-455): wrap reports.go RLS queries in WithOrgTx"
```

---

## Task 6: Migrate inventory.go

**Files:**
- Modify: `backend/internal/storage/inventory.go`
- Note: leave the `asset_scans` INSERT loop (lines 98–118) with raw tx — that table is not RLS-protected

- [ ] **Step 1: Wrap the location validation query in WithOrgTx**

Line 64 currently: `s.pool.QueryRow(ctx, locationQuery, req.LocationID, orgID).Scan(&locationName)`. Wrap in `s.WithOrgTx(ctx, orgID, ...)`.

- [ ] **Step 2: Wrap the asset count query in WithOrgTx**

Line 82. Same pattern.

- [ ] **Step 3: Decide: combine the validation into one tx?**

Option A (minimal): two separate `WithOrgTx` calls for the two SELECTs, leave the existing raw tx for the INSERT.
Option B (single tx): combine the validation SELECTs + asset_scans INSERT into one `WithOrgTx` call — simpler state, better atomicity if validation results are about to be acted on.

**Pick Option B.** Fewer round-trips, and the validation's meaning is "do these still exist at INSERT time" — doing it in a separate tx creates a TOCTOU window where the asset could be deleted between validation and insert. Collapse into one `WithOrgTx`.

- [ ] **Step 4: Test**

```bash
just backend test ./internal/storage/... -run Inventory
just backend test ./internal/handlers/inventory/...
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/storage/inventory.go
git commit -m "refactor(tra-455): wrap inventory.go validation queries in WithOrgTx

Collapses validation SELECTs and asset_scans INSERT into a single
WithOrgTx call — eliminates a TOCTOU window between validation and
insert, and runs the validation SELECTs under the correct org GUC."
```

---

## Task 7: Migrate locations.go

**Files:**
- Modify: `backend/internal/storage/locations.go` (14 callsites)
- Modify: `backend/internal/storage/locations_test.go`, `locations_integration_test.go`, `locations_crossorg_test.go`
- Modify: any handler callers of signature-changed methods

- [ ] **Step 1: Identify signature changes needed**

Read each of the 14 offending methods. Any that lack `orgID` today gains it. Likely candidates: `GetLocationByID`, `getLocationWithParentByID`, `scanHierarchyRows`, `GetLocationWithRelations`. Grep for callers, same as Task 3 Step 1.

- [ ] **Step 2: Apply signature changes, wrap queries**

Same pattern as Task 3. For `CreateLocationWithIdentifiers` (line 456) which calls `trakrf.create_location_with_identifiers()` DB function — the function internally inserts into RLS tables, so the wrapper is required.

- [ ] **Step 3: Update all callers (handlers + tests + internal storage)**

- [ ] **Step 4: Build and test**

```bash
just backend test ./internal/storage/... -run Location
just backend test ./internal/handlers/locations/...
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/storage/locations.go backend/internal/storage/locations_test.go backend/internal/storage/locations_integration_test.go backend/internal/storage/locations_crossorg_test.go backend/internal/handlers/
git commit -m "refactor(tra-455): wrap locations.go queries in WithOrgTx"
```

---

## Task 8: Migrate assets.go

**Files:**
- Modify: `backend/internal/storage/assets.go` (15 callsites + raw tx in `BatchCreateAssets`)
- Modify: `backend/internal/storage/assets_test.go`, `assets_integration_test.go`, `assets_crossorg_test.go`, `assets_type_check_test.go`
- Modify: handler callers of `GetAssetByID`, `getAssetWithLocationByID`

- [ ] **Step 1: Signature changes**

`GetAssetByID(ctx, id *int)` → `GetAssetByID(ctx, orgID int, id *int)`.
`getAssetWithLocationByID(ctx, id int)` → `getAssetWithLocationByID(ctx, orgID, id int)`.

Update the WHERE clauses to include `org_id = $N`.

- [ ] **Step 2: Wrap 15 pool callsites**

Same pattern as Tasks 3/7.

- [ ] **Step 3: Migrate BatchCreateAssets raw tx to WithOrgTx**

Current code (lines ~284–332): `s.pool.Begin(ctx)`, loop `tx.Exec`, `tx.Commit`. Replace with:

```go
err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
    for i, a := range assets {
        if _, err := tx.Exec(ctx, query, a.Name, ..., a.OrgID); err != nil {
            // existing per-row error mapping
            return fmt.Errorf("row %d: ...", i, ...)
        }
    }
    return nil
})
```

The `orgID` is derivable: `assets[0].OrgID` (function already assumes uniform org — comment on line 270). Add a defensive check that every `a.OrgID == orgID` and fail loudly if not.

- [ ] **Step 4: Handler updates**

Grep callers of `GetAssetByID`, `getAssetWithLocationByID`:

```bash
grep -rn "GetAssetByID\|getAssetWithLocationByID\|GetAssetWithLocationByIDForTest" backend/ --include="*.go"
```

Thread `orgID` into each callsite. Delete the redundant `view.OrgID != orgID` checks (e.g., `handlers/assets/assets.go:226`) — the org-scoped query replaces them.

- [ ] **Step 5: Reconsider `GetAssetWithLocationByIDForTest` (line 832)**

If nothing outside the storage package still needs it, delete it. If integration tests use it, update its signature to `(ctx, orgID, id)` and keep.

- [ ] **Step 6: Build and test**

```bash
just backend test ./internal/storage/... -run Asset
just backend test ./internal/handlers/assets/...
just backend test ./internal/handlers/reports/...   # uses GetAssetByID
just backend test ./internal/storage/... -run RLS_SentinelMode   # should now pass end-to-end
```

Expected: all pass, including Task 2's sentinel test.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/storage/assets.go backend/internal/storage/assets_test.go backend/internal/storage/assets_integration_test.go backend/internal/storage/assets_crossorg_test.go backend/internal/storage/assets_type_check_test.go backend/internal/handlers/
git commit -m "refactor(tra-455): wrap assets.go queries and batch tx in WithOrgTx

Includes signature changes for GetAssetByID and getAssetWithLocationByID —
both now take orgID and self-authorize via WHERE org_id = \$N. Removes the
handler-level view.OrgID cross-check that this replaces. BatchCreateAssets
moves from raw pool.Begin to WithOrgTx so INSERTs see the org GUC."
```

---

## Task 9: CI guard banning pool.* in storage

**Files:**
- Modify: `backend/justfile` (add recipe) and/or `.github/workflows/<backend CI workflow>.yml`

- [ ] **Step 1: Add Justfile recipe**

Append to `backend/justfile`:

```makefile
# Fail if pool.Query/Exec/Begin reappears in storage (non-test). TRA-455.
check-rls-guard:
    @ ! grep -rn 's\.pool\.\(Query\|Exec\|Begin\)' internal/storage/ --include='*.go' | grep -v '_test.go' || (echo "ERROR: s.pool.Query/Exec/Begin found in storage non-test code. Wrap in WithOrgTx. See TRA-455." && exit 1)
```

- [ ] **Step 2: Hook into existing `check` or `validate` aggregate**

Find the parent recipe that CI runs (likely `just validate` or `just backend test`). Add `check-rls-guard` as a prerequisite or sequential step.

- [ ] **Step 3: Run the guard locally**

```bash
just backend check-rls-guard
```

Expected: pass (previous tasks eliminated all offenders).

- [ ] **Step 4: Verify CI runs the guard**

Check whichever workflow gates PRs. If it calls `just backend validate` / `just validate`, no workflow edit needed. If it runs individual commands, add a step for `check-rls-guard`.

- [ ] **Step 5: File follow-up ticket for proper analyzer**

Create Linear ticket (title: "TRA-455 follow-up: replace grep-based RLS guard with proper Go analyzer") — references this PR, body: "Swap the grep check for a `forbidigo` rule or custom `go/analysis` pass that can handle edge cases grep misses (e.g., aliased pool variables, pool accessors beyond `s.pool`)."

- [ ] **Step 6: Commit**

```bash
git add backend/justfile .github/workflows/  # whichever applies
git commit -m "chore(tra-455): add CI guard banning pool.Query/Exec/Begin in storage

Coarse grep-based check — fails CI if any non-test storage file
reintroduces a raw pool call against the RLS-protected tables. Follow-up
ticket tracks replacing with a proper go/analysis pass."
```

---

## Task 10: Full verification before PR

- [ ] **Step 1: Full backend test run**

```bash
just backend test
```

Expected: green.

- [ ] **Step 2: Integration tests (if gated separately)**

```bash
just backend test-integration   # or whichever invocation hits the integration tag
```

Expected: green, including `rls_sentinel_test.go`.

- [ ] **Step 3: Lint and type check**

```bash
just backend lint
just validate
```

Expected: green.

- [ ] **Step 4: Push branch and open PR**

```bash
git push -u origin miks2u/tra-455-wrap-rls-protected-queries-in-withorgtx-fixes-500s-on-asset
gh pr create --title "fix(tra-455): wrap RLS-protected queries in WithOrgTx" --body "$(cat <<'EOF'
## Summary
- Threads `orgID` through every storage method touching the 6 RLS-protected tables; wraps all `pool.Query*`/`pool.Exec`/raw `pool.Begin` callsites in `WithOrgTx` so `app.current_org_id` is set via `SET LOCAL` on every RLS query.
- Six methods gain an `orgID` param (previously relied on caller-side authorization comments). Eliminates the silent-empty-reads failure mode seen on AKS and the `42501` on `POST /api/v1/assets`.
- Adds a sentinel-mode integration test (`rls_sentinel_test.go`) as a permanent regression gate.
- Adds a crude grep-based CI guard banning `s.pool.Query/Exec/Begin` in non-test storage code. Follow-up ticket tracks swapping it for a proper analyzer.

## Test plan
- [ ] `just backend test` green locally
- [ ] `rls_sentinel_test.go` passes end-to-end
- [ ] `just backend check-rls-guard` passes
- [ ] Preview deploy: `POST /api/v1/assets` returns 201, `POST /api/v1/lookup/tags` returns 200
- [ ] AKS deploy (after merge): re-run TRA-438 phase 3 repro — both endpoints return 2xx, PG logs show zero `42704` / `42501` for `trakrf-app`

Closes TRA-455.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 5: Verify preview deploy**

Once preview builds (≈5–10 min), hit the two smoke endpoints in the preview app:
- Save a new asset from the UI — expect success, no 500.
- Any flow that calls `POST /api/v1/lookup/tags` (inventory scan lookup) — expect success.

- [ ] **Step 6: Merge strategy**

Per project convention (feedback memory: no squash merges), use merge commit when the user approves. Do not merge until user signs off on preview verification.

---

## Out of scope for this PR

- **Proper analyzer replacing the grep guard** — follow-up Linear ticket (Task 9 Step 5).
- **RLS on `trakrf.asset_scans`** — if tenant isolation at the DB layer is desired for the scans table, that's a schema change warranting its own ticket. Flag during review.
- **Removing the infra-repo sentinel `ALTER ROLE` default** — keep it in place as defense-in-depth. Once storage is fully compliant, the sentinel never fires for app traffic; it only protects against future regressions. Removing it is a separate decision.
- **Test-harness role switch to enforce sentinel mode across the entire suite** — current plan adds a dedicated `rls_sentinel_test.go` that uses a sentinel pool. Converting the whole test harness to run under sentinel mode would catch regressions more thoroughly but risks destabilizing unrelated tests. Consider as a follow-up only if the grep-guard proves insufficient.
