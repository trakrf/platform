# TRA-468 — Normalize `valid_from` / `valid_to` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Guarantee one response shape for "no end date" across the public API — `valid_from` always RFC3339, `valid_to` omitted when no expiry — by backfilling legacy zero-time / far-future sentinel rows, closing update-path zero-time leaks, and locking in the behavior with integration tests.

**Architecture:** One data-cleanup migration over all four tables (`organizations`, `assets`, `locations`, `identifiers`) with range-based thresholds so unknown sentinels also get swept. Two `storage/*.go` `mapReqToFields` helpers are tightened with `IsZero()` guards (TRA-447 fixed the create path; this finishes the update path). New integration tests lock in the response shape. No schema column changes; no CHECK constraints; no doc edits in this branch (docs follow in a separate trakrf-docs PR per the "ship docs behind backend reality" rule).

**Tech Stack:** Go, PostgreSQL (TimescaleDB), `pgxpool`, `golang-migrate` migrations embedded via `embed.FS`, existing integration-test harness in `backend/internal/testutil` (`SetupTestDB`, `CreateTestAccount`, `CleanupAssets`, `CleanupTestAccounts`, `NewAssetFactory`). Integration tests run only under the `integration` build tag.

**Branch:** `miks2u/tra-468-normalize-valid-dates` (already created, spec committed as `e2a003f`)

**Spec:** `docs/superpowers/specs/2026-04-23-tra-468-normalize-valid-dates-design.md`

---

## File Map

**Create:**
- `backend/migrations/000030_normalize_valid_dates.up.sql`
- `backend/migrations/000030_normalize_valid_dates.down.sql`
- `backend/internal/storage/valid_dates_migration_integration_test.go` — migration regression test (integration build tag; lives alongside other storage integration tests so it gets `testutil` access)

**Modify (append tests only — no function rewrites):**
- `backend/internal/handlers/assets/assets_integration_test.go` — add update-clobber test + response-shape tests
- `backend/internal/handlers/locations/public_write_integration_test.go` — add update-clobber test + response-shape tests

**Modify (small code fix):**
- `backend/internal/storage/assets.go` — `mapReqToFields` block at ~line 408 (add `IsZero()` guard, switch dereference to `.ToTime()` to align with the locations pattern)
- `backend/internal/storage/locations.go` — `mapReqToFields` block at ~line 844 (add `IsZero()` guard)

**Untouched (verified out-of-scope):**
- `backend/internal/handlers/orgs/*.go` — no `valid_from` / `valid_to` request-side handling; migration handles legacy DB rows directly
- `backend/internal/storage/identifiers.go` — no API surface; migration handles data; storage already scans `*time.Time` → nil correctly for NULL values

---

## Task 1: Data-cleanup migration

**Files:**
- Create: `backend/migrations/000030_normalize_valid_dates.up.sql`
- Create: `backend/migrations/000030_normalize_valid_dates.down.sql`

---

- [ ] **Step 1.1: Verify migration numbering is still 000030**

Run:

```bash
ls /home/mike/platform/backend/migrations/ | grep -E '^0000[0-9]+' | sort | tail -3
```

Expected: highest is `000029_api_keys_created_by_nullable.*`. If someone merged a new migration on main first, pick the next free number and substitute it everywhere the plan says `000030` (filename, test name, log output). No other changes.

---

- [ ] **Step 1.2: Write the `.up.sql`**

Create `backend/migrations/000030_normalize_valid_dates.up.sql` with exactly:

```sql
SET search_path=trakrf,public;

-- TRA-468: normalize legacy valid_from/valid_to values to the single wire convention.
-- valid_from: NOT NULL, must represent a real effective moment (>= 1900, < 2100).
-- valid_to:   NULL = "no expiry"; RFC3339 timestamp otherwise; never a sentinel.
--
-- Range-based thresholds sweep the two observed sentinels (0001-01-01, 2099-12-31)
-- plus any future unknown sentinels (e.g., 1970-01-01 epoch, 9999-12-31). No legitimate
-- business data lives outside [1900-01-01, 2099-01-01).

-- organizations
UPDATE organizations SET valid_from = created_at
 WHERE valid_from < TIMESTAMPTZ '1900-01-01';
UPDATE organizations SET valid_to = NULL
 WHERE valid_to IS NOT NULL
   AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01');

-- assets
UPDATE assets SET valid_from = created_at
 WHERE valid_from < TIMESTAMPTZ '1900-01-01';
UPDATE assets SET valid_to = NULL
 WHERE valid_to IS NOT NULL
   AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01');

-- locations
UPDATE locations SET valid_from = created_at
 WHERE valid_from < TIMESTAMPTZ '1900-01-01';
UPDATE locations SET valid_to = NULL
 WHERE valid_to IS NOT NULL
   AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01');

-- identifiers
UPDATE identifiers SET valid_from = created_at
 WHERE valid_from < TIMESTAMPTZ '1900-01-01';
UPDATE identifiers SET valid_to = NULL
 WHERE valid_to IS NOT NULL
   AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01');
```

`golang-migrate` wraps each file in an implicit transaction; do not add `BEGIN` / `COMMIT`.

---

- [ ] **Step 1.3: Write the `.down.sql`**

Create `backend/migrations/000030_normalize_valid_dates.down.sql` with exactly:

```sql
SET search_path=trakrf,public;

-- TRA-468 is one-way data cleanup: destroyed zero-time and far-future sentinels
-- cannot be reconstructed. Down migration is intentionally a no-op.
SELECT 1;
```

The `SELECT 1;` keeps `golang-migrate` happy across versions that don't accept empty files.

---

- [ ] **Step 1.4: Run the embed-sanity test**

```bash
cd /home/mike/platform/backend && go test ./migrations/... -run TestFSContainsMigrations
```

Expected: PASS. Confirms the new files are embedded.

---

- [ ] **Step 1.5: Write the migration regression test**

Create `backend/internal/storage/valid_dates_migration_integration_test.go`:

```go
//go:build integration
// +build integration

package storage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

// TestNormalizeValidDatesMigration verifies the 000030 cleanup logic on
// representative bad rows. The test DB already has all migrations applied
// after SetupTestDB; we re-run the same UPDATE statements after seeding
// bad data directly to confirm the SQL logic works on future encounters too.
func TestNormalizeValidDatesMigration(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, pool)

	// --- seed bad data directly (bypassing app layer) ---
	ts := time.Now().UnixNano()

	// Asset with both sentinels.
	var assetID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO trakrf.assets
			(name, identifier, type, description, org_id, valid_from, valid_to, is_active, metadata)
		VALUES
			($1, $2, 'asset', '', $3,
			 TIMESTAMPTZ '0001-01-01', TIMESTAMPTZ '2099-12-31',
			 true, '{}'::jsonb)
		RETURNING id
	`,
		fmt.Sprintf("tra468-asset-%d", ts),
		fmt.Sprintf("TRA468-ASSET-%d", ts),
		orgID,
	).Scan(&assetID)
	require.NoError(t, err)

	// Location with both sentinels.
	var locID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO trakrf.locations
			(org_id, identifier, name, is_active, valid_from, valid_to)
		VALUES
			($1, $2, $3, true,
			 TIMESTAMPTZ '0001-01-01', TIMESTAMPTZ '2099-12-31')
		RETURNING id
	`,
		orgID,
		fmt.Sprintf("TRA468-LOC-%d", ts),
		"tra468-loc",
	).Scan(&locID)
	require.NoError(t, err)

	// --- re-apply the migration's cleanup SQL ---
	cleanupStmts := []string{
		`UPDATE trakrf.organizations SET valid_from = created_at WHERE valid_from < TIMESTAMPTZ '1900-01-01'`,
		`UPDATE trakrf.organizations SET valid_to = NULL WHERE valid_to IS NOT NULL AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01')`,
		`UPDATE trakrf.assets SET valid_from = created_at WHERE valid_from < TIMESTAMPTZ '1900-01-01'`,
		`UPDATE trakrf.assets SET valid_to = NULL WHERE valid_to IS NOT NULL AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01')`,
		`UPDATE trakrf.locations SET valid_from = created_at WHERE valid_from < TIMESTAMPTZ '1900-01-01'`,
		`UPDATE trakrf.locations SET valid_to = NULL WHERE valid_to IS NOT NULL AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01')`,
		`UPDATE trakrf.identifiers SET valid_from = created_at WHERE valid_from < TIMESTAMPTZ '1900-01-01'`,
		`UPDATE trakrf.identifiers SET valid_to = NULL WHERE valid_to IS NOT NULL AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01')`,
	}
	for _, stmt := range cleanupStmts {
		_, err := pool.Exec(ctx, stmt)
		require.NoError(t, err, "stmt: %s", stmt)
	}

	// --- assert cleanup ---
	type row struct {
		validFrom time.Time
		validTo   *time.Time
		createdAt time.Time
	}

	check := func(t *testing.T, label, table string, id int64) {
		t.Helper()
		var r row
		err := pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT valid_from, valid_to, created_at FROM trakrf.%s WHERE id = $1`, table),
			id).Scan(&r.validFrom, &r.validTo, &r.createdAt)
		require.NoError(t, err)
		require.True(t, r.validFrom.Equal(r.createdAt),
			"%s: valid_from = %v, want created_at %v", label, r.validFrom, r.createdAt)
		require.Nil(t, r.validTo, "%s: valid_to should be nil, got %v", label, r.validTo)
	}

	check(t, "asset", "assets", assetID)
	check(t, "location", "locations", locID)
}
```

Notes for the engineer:
- The `storage` handlers already scan `valid_to` into `*time.Time`, so NULL round-trips as nil.
- The package is `storage_test` (external test package) because `testutil` imports `storage`; importing `testutil` from `storage` directly would be a cycle. Other integration tests in this directory already use this pattern — `grep -l 'package storage_test' backend/internal/storage/*.go` to confirm and match.

If the existing integration tests in that directory use `package storage` (internal), switch to that and drop the `_test` suffix — match whichever pattern is already in use.

---

- [ ] **Step 1.6: Run the new test**

```bash
cd /home/mike/platform/backend && go test -tags=integration ./internal/storage/... -run TestNormalizeValidDatesMigration -v
```

Expected: PASS.

If it fails: the cleanup SQL in Step 1.2 has a bug or the test's seed INSERT violates a constraint (likely a missing required column — read the error, add the missing field to the INSERT, re-run).

---

- [ ] **Step 1.7: Run the migration against the dev DB (optional sanity)**

Skip if the integration test DB already reapplied migrations cleanly. Otherwise:

```bash
cd /home/mike/platform/backend && go run ./cmd/serve migrate up
```

Expected: `000030 applied` (or equivalent success log). If `migrate up` recipe differs, see `backend/cmd/serve/` for the exact subcommand (landed in TRA-367).

---

- [ ] **Step 1.8: Commit**

```bash
git add backend/migrations/000030_normalize_valid_dates.up.sql \
        backend/migrations/000030_normalize_valid_dates.down.sql \
        backend/internal/storage/valid_dates_migration_integration_test.go
git commit -m "$(cat <<'EOF'
feat(tra-468): backfill zero-time and far-future valid_from/valid_to

Range-based cleanup across organizations, assets, locations, identifiers.
valid_from below 1900 is reset to created_at; valid_to outside
[1900, 2099) is nulled. Regression test seeds sentinels and verifies
cleanup.
EOF
)"
```

---

## Task 2: Close zero-time leak in asset update path

**Files:**
- Modify: `backend/internal/storage/assets.go` (~line 408)
- Modify: `backend/internal/handlers/assets/assets_integration_test.go` (append)

---

- [ ] **Step 2.1: Confirm current state of `mapReqToFields` in assets.go**

```bash
sed -n '402,420p' /home/mike/platform/backend/internal/storage/assets.go
```

Expected (current bug state):

```go
if req.ValidFrom != nil {
    fields["valid_from"] = *req.ValidFrom
}
if req.ValidTo != nil {
    fields["valid_to"] = *req.ValidTo
}
```

If the block has already been fixed (e.g., a merge conflict raced), adapt Step 2.3 to match whatever's there — the target shape is the one in Step 2.3.

---

- [ ] **Step 2.2: Write the failing update-path test**

Append to `backend/internal/handlers/assets/assets_integration_test.go` (end of file). The test follows the same pattern as `TestUpdateAsset` at line 231 — direct handler call with `withOrgContext` and manual `chi.RouteCtxKey` injection:

```go
// TRA-468: PATCH/PUT without valid_from/valid_to must not zero or clobber existing values.
func TestUpdateAsset_DoesNotClobberValidDates(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)

	// Create with explicit valid_from and valid_to.
	vf := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	vt := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	vfFlex := shared.FlexibleDate{Time: vf}
	vtFlex := shared.FlexibleDate{Time: vt}

	createReq := asset.CreateAssetRequest{
		Identifier: "TRA468-UPD-001",
		Name:       "update-test",
		Type:       "asset",
		ValidFrom:  &vfFlex,
		ValidTo:    &vtFlex,
	}
	created, err := store.CreateAsset(context.Background(), createReq)
	require.NoError(t, err)
	require.NotNil(t, created)

	// PUT only the name — no valid_from/valid_to in body.
	newName := "renamed"
	updateReq := asset.UpdateAssetRequest{Name: &newName}
	body, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/TRA468-UPD-001", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
		URLParams: chi.RouteParams{
			Keys:   []string{"identifier"},
			Values: []string{"TRA468-UPD-001"},
		},
	}))
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	handler.UpdateAsset(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp UpdateAssetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, newName, resp.Data.Name)
	assert.True(t, resp.Data.ValidFrom.Equal(vf),
		"valid_from clobbered: got %v, want %v", resp.Data.ValidFrom, vf)
	require.NotNil(t, resp.Data.ValidTo, "valid_to clobbered to nil")
	assert.True(t, resp.Data.ValidTo.Equal(vt),
		"valid_to clobbered: got %v, want %v", resp.Data.ValidTo, vt)
}
```

Add the `shared` import to the existing import block at the top of the file if it isn't already there:

```go
"github.com/trakrf/platform/backend/internal/models/shared"
```

---

- [ ] **Step 2.3: Run the test to see current behavior**

```bash
cd /home/mike/platform/backend && go test -tags=integration ./internal/handlers/assets/... -run TestUpdateAsset_DoesNotClobberValidDates -v
```

Two outcomes:
- **Passes:** the bug isn't currently reachable via JSON (FlexibleDate unmarshaller produces `nil` when the field is absent). The `IsZero()` guard in Step 2.4 is still warranted as defense-in-depth for any future caller that hands in a zero-valued `FlexibleDate` pointer. Keep going.
- **Fails** (`valid_from` or `valid_to` clobbered): there's a real leak. Step 2.4 fixes it.

Note the outcome in your shell history, then proceed.

---

- [ ] **Step 2.4: Apply the fix — add `IsZero()` guards, align with locations pattern**

Edit `backend/internal/storage/assets.go` at the `mapReqToFields` block (~line 408). Replace:

```go
if req.ValidFrom != nil {
    fields["valid_from"] = *req.ValidFrom
}
if req.ValidTo != nil {
    fields["valid_to"] = *req.ValidTo
}
```

with:

```go
if req.ValidFrom != nil && !req.ValidFrom.IsZero() {
    fields["valid_from"] = req.ValidFrom.ToTime()
}
if req.ValidTo != nil && !req.ValidTo.IsZero() {
    fields["valid_to"] = req.ValidTo.ToTime()
}
```

This matches the existing shape in `backend/internal/storage/locations.go:844` and guarantees the SQL driver sees a real `time.Time` (not a `FlexibleDate` struct whose `driver.Valuer` support may vary).

---

- [ ] **Step 2.5: Re-run the test plus surrounding asset integration suite**

```bash
cd /home/mike/platform/backend && go test -tags=integration ./internal/handlers/assets/... -v
```

Expected: all PASS, including the new `TestUpdateAsset_DoesNotClobberValidDates` and the existing `TestUpdateAsset` / `TestFullCRUDWorkflow`.

---

- [ ] **Step 2.6: Commit**

```bash
git add backend/internal/storage/assets.go \
        backend/internal/handlers/assets/assets_integration_test.go
git commit -m "fix(tra-468): asset update path rejects zero-time valid_from/valid_to"
```

---

## Task 3: Close zero-time leak in location update path

**Files:**
- Modify: `backend/internal/storage/locations.go` (~line 844)
- Modify: `backend/internal/handlers/locations/public_write_integration_test.go` (append)

---

- [ ] **Step 3.1: Confirm current state of location `mapReqToFields`**

```bash
sed -n '840,858p' /home/mike/platform/backend/internal/storage/locations.go
```

Expected:

```go
if req.ValidFrom != nil {
    t := req.ValidFrom.ToTime()
    fields["valid_from"] = t
}
if req.ValidTo != nil {
    t := req.ValidTo.ToTime()
    fields["valid_to"] = t
}
```

---

- [ ] **Step 3.2: Write the failing clobber test**

Append to `backend/internal/handlers/locations/public_write_integration_test.go` (end of file). This file uses the `locations_test` external package and the full public-write router with API-key auth — follow the existing `TestCreateLocation_APIKey_HappyPath` shape:

```go
// TRA-468: PUT without valid_from/valid_to must not clobber existing values.
func TestUpdateLocation_DoesNotClobberValidDates(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-loc-write-clobber-valid-dates")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	// Create with explicit valid_to.
	createBody := `{"identifier":"tra468-clobber","name":"clobber-test",` +
		`"valid_from":"2026-01-01","valid_to":"2027-01-01","is_active":true}`
	reqC := httptest.NewRequest(http.MethodPost, "/api/v1/locations",
		bytes.NewBufferString(createBody))
	reqC.Header.Set("Authorization", "Bearer "+token)
	reqC.Header.Set("Content-Type", "application/json")
	wC := httptest.NewRecorder()
	r.ServeHTTP(wC, reqC)
	require.Equal(t, http.StatusCreated, wC.Code, wC.Body.String())

	var createdResp map[string]any
	require.NoError(t, json.Unmarshal(wC.Body.Bytes(), &createdResp))
	createdData := createdResp["data"].(map[string]any)
	origValidFrom := createdData["valid_from"]
	origValidTo := createdData["valid_to"]
	require.NotNil(t, origValidFrom)
	require.NotNil(t, origValidTo, "seed create did not return valid_to")

	// PUT only the name — nothing else.
	updateBody := `{"name":"renamed-loc"}`
	reqU := httptest.NewRequest(http.MethodPut, "/api/v1/locations/tra468-clobber",
		bytes.NewBufferString(updateBody))
	reqU.Header.Set("Authorization", "Bearer "+token)
	reqU.Header.Set("Content-Type", "application/json")
	wU := httptest.NewRecorder()
	r.ServeHTTP(wU, reqU)
	require.Equal(t, http.StatusOK, wU.Code, wU.Body.String())

	var updatedResp map[string]any
	require.NoError(t, json.Unmarshal(wU.Body.Bytes(), &updatedResp))
	updatedData := updatedResp["data"].(map[string]any)

	assert.Equal(t, origValidFrom, updatedData["valid_from"],
		"valid_from clobbered on PUT")
	assert.Equal(t, origValidTo, updatedData["valid_to"],
		"valid_to clobbered on PUT")
}
```

---

- [ ] **Step 3.3: Run the test**

```bash
cd /home/mike/platform/backend && go test -tags=integration ./internal/handlers/locations/... -run TestUpdateLocation_DoesNotClobberValidDates -v
```

Two outcomes — same interpretation as Step 2.3.

---

- [ ] **Step 3.4: Apply the fix**

Edit `backend/internal/storage/locations.go` at the `mapReqToFields` block (~line 844). Replace the current:

```go
if req.ValidFrom != nil {
    t := req.ValidFrom.ToTime()
    fields["valid_from"] = t
}
if req.ValidTo != nil {
    t := req.ValidTo.ToTime()
    fields["valid_to"] = t
}
```

with:

```go
if req.ValidFrom != nil && !req.ValidFrom.IsZero() {
    fields["valid_from"] = req.ValidFrom.ToTime()
}
if req.ValidTo != nil && !req.ValidTo.IsZero() {
    fields["valid_to"] = req.ValidTo.ToTime()
}
```

---

- [ ] **Step 3.5: Re-run the location integration suite**

```bash
cd /home/mike/platform/backend && go test -tags=integration ./internal/handlers/locations/... -v
```

Expected: all PASS.

---

- [ ] **Step 3.6: Commit**

```bash
git add backend/internal/storage/locations.go \
        backend/internal/handlers/locations/public_write_integration_test.go
git commit -m "fix(tra-468): location update path rejects zero-time valid_from/valid_to"
```

---

## Task 4: Response-shape integration tests (lock in `valid_to` omission)

**Files:**
- Modify: `backend/internal/handlers/assets/assets_integration_test.go` (append)
- Modify: `backend/internal/handlers/locations/public_write_integration_test.go` (append)

---

- [ ] **Step 4.1: Append asset response-shape tests**

Append to `backend/internal/handlers/assets/assets_integration_test.go`:

```go
// TRA-468: POST with no valid_to must omit the `valid_to` key from the response JSON.
func TestCreateAsset_OmitsValidToWhenNull(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	reqBody := `{"identifier":"TRA468-OMIT","name":"no-expiry","type":"asset","valid_from":"2026-01-01"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, accountID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	data := envelope["data"].(map[string]any)

	_, hasValidTo := data["valid_to"]
	assert.False(t, hasValidTo, "response contained valid_to key when none was set: %#v", data["valid_to"])
	_, hasValidFrom := data["valid_from"]
	assert.True(t, hasValidFrom, "response missing valid_from (should always be present)")
}

// TRA-468: POST with explicit valid_to must return it as RFC3339 on both POST and GET.
func TestCreateAsset_IncludesValidToWhenSet(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)

	accountID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	reqBody := `{"identifier":"TRA468-KEEP","name":"with-expiry","type":"asset","valid_from":"2026-01-01","valid_to":"2027-06-15"}`
	reqC := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(reqBody))
	reqC.Header.Set("Content-Type", "application/json")
	reqC = withOrgContext(reqC, accountID)
	wC := httptest.NewRecorder()
	router.ServeHTTP(wC, reqC)

	require.Equal(t, http.StatusCreated, wC.Code, wC.Body.String())

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(wC.Body.Bytes(), &envelope))
	data := envelope["data"].(map[string]any)
	vt, ok := data["valid_to"].(string)
	require.True(t, ok, "valid_to missing or wrong type on POST: %#v", data["valid_to"])
	_, err := time.Parse(time.RFC3339, vt)
	require.NoError(t, err, "valid_to not RFC3339 on POST: %q", vt)

	// GET the asset back and verify the same shape round-trips.
	surrogateID := int(data["surrogate_id"].(float64))
	reqG := httptest.NewRequest(http.MethodGet, "/api/v1/assets/"+strconv.Itoa(surrogateID), nil)
	reqG = withOrgContext(reqG, accountID)
	wG := httptest.NewRecorder()
	router.ServeHTTP(wG, reqG)

	require.Equal(t, http.StatusOK, wG.Code, wG.Body.String())
	var getEnvelope map[string]any
	require.NoError(t, json.Unmarshal(wG.Body.Bytes(), &getEnvelope))
	getData := getEnvelope["data"].(map[string]any)
	assert.Equal(t, vt, getData["valid_to"], "GET valid_to differs from POST")
}
```

**Check first:** the GET path may be `/api/v1/assets/{identifier}` rather than `/api/v1/assets/{surrogate_id}` depending on router registration — inspect `setupTestRouter` and handler route registrations. Use whatever the existing `TestGetAssetByID` in the same file uses. If it reads by integer ID, keep `strconv.Itoa`; if by identifier, pass `"TRA468-KEEP"` directly.

---

- [ ] **Step 4.2: Append location response-shape tests**

Append to `backend/internal/handlers/locations/public_write_integration_test.go`:

```go
// TRA-468: POST with no valid_to must omit the `valid_to` key from the response JSON.
func TestCreateLocation_OmitsValidToWhenNull(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-loc-write-omit-valid-to")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	body := `{"identifier":"tra468-loc-omit","name":"no-expiry","valid_from":"2026-01-01","is_active":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	data := envelope["data"].(map[string]any)

	_, hasValidTo := data["valid_to"]
	assert.False(t, hasValidTo, "response contained valid_to key when none was set: %#v", data["valid_to"])
	_, hasValidFrom := data["valid_from"]
	assert.True(t, hasValidFrom, "response missing valid_from (should always be present)")
}

// TRA-468: POST with explicit valid_to must return it as RFC3339.
func TestCreateLocation_IncludesValidToWhenSet(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-loc-write-include-valid-to")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	body := `{"identifier":"tra468-loc-keep","name":"with-expiry","valid_from":"2026-01-01","valid_to":"2027-06-15","is_active":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	data := envelope["data"].(map[string]any)
	vt, ok := data["valid_to"].(string)
	require.True(t, ok, "valid_to missing or wrong type: %#v", data["valid_to"])
	_, err := time.Parse(time.RFC3339, vt)
	assert.NoError(t, err, "valid_to not RFC3339: %q", vt)
}
```

---

- [ ] **Step 4.3: Run the new tests**

```bash
cd /home/mike/platform/backend && go test -tags=integration ./internal/handlers/assets/... -run TRA468 -v
cd /home/mike/platform/backend && go test -tags=integration ./internal/handlers/locations/... -run TRA468 -v
```

Expected: all four tests PASS. These probably pass on main today (`omitempty` is already on `PublicAssetView.ValidTo` and `PublicLocationView.ValidTo`); the value is locking the behavior in as a regression guard.

---

- [ ] **Step 4.4: Commit**

```bash
git add backend/internal/handlers/assets/assets_integration_test.go \
        backend/internal/handlers/locations/public_write_integration_test.go
git commit -m "test(tra-468): lock in valid_to omission on public create+get responses"
```

---

## Task 5: Full validation and PR

---

- [ ] **Step 5.1: Run the full backend test suite, both tagged and untagged**

```bash
cd /home/mike/platform
just backend test                                                    # unit
cd backend && go test -tags=integration ./... ; cd ..                # integration
```

Expected: all PASS. Investigate and fix any regressions before proceeding. If `just backend test` already runs the integration suite, the second invocation is redundant but harmless.

---

- [ ] **Step 5.2: Run the root validate recipe**

```bash
just validate
```

Expected: backend + frontend lint/test both green.

---

- [ ] **Step 5.3: Push branch**

```bash
git push -u origin miks2u/tra-468-normalize-valid-dates
```

---

- [ ] **Step 5.4: Open PR**

```bash
gh pr create --title "fix(tra-468): normalize valid_from/valid_to across API" --body "$(cat <<'EOF'
## Summary
- Backfill migration (`000030_normalize_valid_dates`) sweeps zero-time `valid_from` and sentinel `valid_to` values (`0001-01-01`, `2099-12-31`, and any out-of-range peers) across `organizations`, `assets`, `locations`, `identifiers`.
- Asset and location update paths (`storage/*.go::mapReqToFields`) now reject zero-valued `FlexibleDate` and always convert to `time.Time` via `.ToTime()` before handing to the SQL driver.
- New integration tests lock in the public-response convention: `valid_from` always RFC3339, `valid_to` omitted when no expiry.
- Docs will follow in a separate `trakrf-docs` PR per the "ship docs behind backend reality" rule.

Closes TRA-468.

## Test plan
- [ ] `just validate` green locally
- [ ] `TestNormalizeValidDatesMigration` PASS against a fresh integration DB
- [ ] `TestUpdateAsset_DoesNotClobberValidDates` / `TestUpdateLocation_DoesNotClobberValidDates` PASS
- [ ] `TestCreateAsset_OmitsValidToWhenNull` / `TestCreateLocation_OmitsValidToWhenNull` PASS
- [ ] `TestCreateAsset_IncludesValidToWhenSet` / `TestCreateLocation_IncludesValidToWhenSet` PASS
- [ ] Manual black-box on preview deploy: `GET /api/v1/locations` and `GET /api/v1/assets` return single convention — `valid_from` always RFC3339, `valid_to` key absent when no expiry, no `0001-01-01` or `2099-12-31` anywhere

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Report the PR URL.

---

- [ ] **Step 5.5: File follow-up Linear ticket for docs**

Create a child ticket of TRA-468 titled "docs(trakrf-docs): document valid_from/valid_to convention". Description pulls from the spec's Section 4. Blocked-by: this PR merging + preview deploy succeeding. Assign to self.

---

## Self-Review Checklist (run after all tasks complete)

1. **Spec coverage:**
   - [ ] Backfill migration (all 4 tables, range-based) → Task 1 ✓
   - [ ] Code audit for zero-time leaks (assets + locations update path) → Tasks 2 + 3 ✓
   - [ ] Response-shape integration tests (omit + include) → Task 4 ✓
   - [ ] Migration regression test → Step 1.5 ✓
   - [ ] Docs queued as separate PR → Step 5.5 ✓
   - [ ] No column changes, no CHECK constraints → confirmed not added ✓

2. **Placeholder scan:** No "TBD" / "TODO" / vague "add validation" steps. Every code step has exact code.

3. **Type / name consistency:**
   - `req.ValidFrom.IsZero()` + `req.ValidFrom.ToTime()` used identically in both storage fixes.
   - `testutil.SetupTestDB`, `testutil.CreateTestAccount`, `testutil.CleanupAssets`, `testutil.CleanupTestAccounts` — all confirmed to exist in `backend/internal/testutil/database.go`.
   - `seedLocOrgAndKey` + `buildLocationsPublicWriteRouter` — existing helpers in `public_write_integration_test.go`, reused without modification.
   - Integration build tag `//go:build integration` applied on all new test files.

Fix any issue inline; no full re-review needed.
