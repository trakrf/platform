# Tag Conflict UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a duplicate tag attach actionable — the backend 409 names the conflicting entity, both the asset and location forms detect conflicts on tag-field blur and show an inline error, and the misleading "Reassign" modal is removed.

**Architecture:** The `(org_id, type, value)` partial unique index already prevents duplicate live tag attachments and the handlers already return 409 — this is a UX-only change. Backend: on the unique-violation, `AddTagToAsset`/`AddTagToLocation` run one follow-up SELECT to name the entity already holding the tag and fold it into the error string; the HTTP handlers are untouched because the enriched message still contains the `"already exist"` substring their 409 branch matches. Frontend: a shared `checkTagConflict` helper calls `lookupApi.byTag`; both forms call it on tag-field blur and on barcode scan, store the result on a new `conflict?` field of each `TagInput`, render it through `TagInputRow`'s existing (currently-unused) `error` prop, and disable Save while any row conflicts. The "Reassign" modal is deleted. `LocationFormModal` gains the missing 409 branch so a save-time conflict surfaces like it does for assets. The excluded `AssetForm`/`AssetFormModal` vitest suites are repaired and re-enabled.

**Tech Stack:** Go (pgx v5, pgxmock v3, testify), React + TypeScript, Vitest, TimescaleDB (integration tests), `just` task runner.

---

## File Structure

**Backend**
- `backend/internal/storage/tags.go` — MODIFY: add `isTagDuplicateErr`, `tagConflict`, `lookupTagConflict`, `derefStr`, `resolveTagError`; route `AddTagToAsset`/`AddTagToLocation` errors through `resolveTagError`.
- `backend/internal/storage/tags_test.go` — MODIFY: add a clarifying comment to the two `_Duplicate` pgxmock tests (they keep passing — they now exercise the lookup-failed fallback).
- `backend/internal/storage/tags_conflict_integration_test.go` — CREATE: real-DB cross-asset / cross-location / soft-deleted tests.
- `backend/internal/handlers/assets/tag_conflict_integration_test.go` — CREATE: handler-level 409-detail test.

**Frontend**
- `frontend/src/types/assets/index.ts` — MODIFY: add `conflict?: string` to `TagInput`.
- `frontend/src/lib/tags/conflictCheck.ts` — CREATE: shared `checkTagConflict` helper.
- `frontend/src/lib/tags/conflictCheck.test.ts` — CREATE: helper unit tests.
- `frontend/vitest.config.ts` — MODIFY: remove two entries from the exclude list.
- `frontend/src/components/assets/AssetFormModal.test.tsx` — MODIFY: repair store mocks.
- `frontend/src/components/assets/AssetForm.test.tsx` — MODIFY: repair stale assertions + add conflict tests.
- `frontend/src/components/assets/AssetForm.tsx` — MODIFY: remove Reassign modal; add conflict check on blur/scan; disable Save on conflict.
- `frontend/src/components/locations/LocationForm.tsx` — MODIFY: same as AssetForm.
- `frontend/src/components/locations/LocationForm.test.tsx` — MODIFY: add conflict tests.
- `frontend/src/components/locations/LocationFormModal.tsx` — MODIFY: add 409 / `>= 400` branches and `detail` extraction.
- `frontend/src/components/locations/LocationFormModal.test.tsx` — CREATE: 409-handling test.

**Task order & dependencies:** Tasks 1–2 (backend) are independent of 3–7 (frontend). Task 3 must precede Tasks 5–6 (their test files must run first). Task 4 must precede Tasks 5–6. Task 7 is independent of 3–6.

---

## Task 1: Backend — enrich the tag-conflict error

**Files:**
- Modify: `backend/internal/storage/tags.go`
- Modify: `backend/internal/storage/tags_test.go`
- Create: `backend/internal/storage/tags_conflict_integration_test.go`

Integration tests need a live TimescaleDB. From the repo root, before running them:
```bash
docker compose -p platform up -d timescaledb
```
and export `PG_URL` pointing at it (a superuser URL ending in `/postgres`; check `docker-compose.yaml`'s `timescaledb` service for the exact credentials, commonly `postgres://postgres:postgres@localhost:5432/postgres`). `SetupTestDatabase` rewrites the db name to `trakrf_test` itself.

- [ ] **Step 1: Write the failing integration test**

Create `backend/internal/storage/tags_conflict_integration_test.go`:

```go
//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func rfidReq(value string) shared.TagRequest {
	t := "rfid"
	return shared.TagRequest{TagType: &t, Value: value}
}

// seedLocation inserts a location directly, mirroring testutil.CreateTestAsset.
func seedLocation(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey, name string) int {
	t.Helper()
	now := time.Now()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, valid_from, valid_to, is_active)
		VALUES ($1, $2, $3, $4, $5, TRUE)
		RETURNING id
	`, orgID, externalKey, name, now, now.Add(24*time.Hour)).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestAddTag_CrossAssetConflict(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	assetA := testutil.CreateTestAsset(t, pool, orgID, "AST-A")
	assetB := testutil.CreateTestAsset(t, pool, orgID, "AST-B")

	value := "E2000000CONFLICT01"
	_, err := store.AddTagToAsset(ctx, orgID, assetA.ID, rfidReq(value))
	require.NoError(t, err)

	_, err = store.AddTagToAsset(ctx, orgID, assetB.ID, rfidReq(value))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists", "handler keys its 409 on this substring")
	assert.Contains(t, err.Error(), "asset")
	assert.Contains(t, err.Error(), "AST-A", "names the conflicting asset's external_key")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `just backend test-integration -run TestAddTag_CrossAssetConflict ./internal/storage/`
Expected: FAIL — the current message is `tag rfid:E2000000CONFLICT01 already exists`, so the `"AST-A"` assertion fails.

- [ ] **Step 3: Implement the enrichment in `tags.go`**

In `backend/internal/storage/tags.go`, add `"errors"` to the import block (it currently imports `context`, `encoding/json`, `fmt`, `strings`, `pgx`, `pgconn`, and the model packages). Then add these declarations (place them just above `parseTagError`):

```go
// isTagDuplicateErr reports whether err is the (org_id, type, value)
// partial-unique-index violation on the tags table.
func isTagDuplicateErr(err error) bool {
	if pgErr, ok := err.(*pgconn.PgError); ok {
		return pgErr.ConstraintName == "tags_org_id_type_value_unique"
	}
	return strings.Contains(err.Error(), "duplicate key")
}

// tagConflict describes the entity a tag value is already attached to.
type tagConflict struct {
	EntityType  string // "asset" or "location"
	Name        string
	ExternalKey string
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// lookupTagConflict finds the live tag row colliding on (orgID, tagType,
// value) and returns the asset or location it is attached to. Returns
// (nil, nil) when no live collision is found — e.g. the conflicting row was
// soft-deleted between the failed INSERT and this lookup.
func (s *Storage) lookupTagConflict(ctx context.Context, orgID int, tagType, value string) (*tagConflict, error) {
	query := `
		SELECT t.asset_id, t.location_id,
		       a.name, a.external_key,
		       l.name, l.external_key
		  FROM trakrf.tags t
		  LEFT JOIN trakrf.assets    a ON a.id = t.asset_id
		  LEFT JOIN trakrf.locations l ON l.id = t.location_id
		 WHERE t.org_id = $1 AND t.type = $2 AND t.value = $3
		   AND t.deleted_at IS NULL
		 LIMIT 1
	`
	var assetID, locationID *int
	var assetName, assetKey, locName, locKey *string
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, tagType, value).Scan(
			&assetID, &locationID, &assetName, &assetKey, &locName, &locKey,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	switch {
	case assetID != nil:
		return &tagConflict{EntityType: "asset", Name: derefStr(assetName), ExternalKey: derefStr(assetKey)}, nil
	case locationID != nil:
		return &tagConflict{EntityType: "location", Name: derefStr(locName), ExternalKey: derefStr(locKey)}, nil
	default:
		return nil, nil
	}
}

// resolveTagError converts an INSERT error from AddTagToAsset/AddTagToLocation
// into a user-facing error. For the (org, type, value) unique-violation it
// enriches the message by naming the entity already holding the tag;
// everything else delegates to parseTagError. The enriched message keeps the
// "already exists" substring the HTTP handlers match to produce a 409.
func (s *Storage) resolveTagError(ctx context.Context, orgID int, err error, tagType, value string) error {
	if !isTagDuplicateErr(err) {
		return parseTagError(err, tagType, value)
	}
	conflict, lookupErr := s.lookupTagConflict(ctx, orgID, tagType, value)
	if lookupErr != nil || conflict == nil {
		return parseTagError(err, tagType, value) // generic fallback
	}
	return fmt.Errorf(
		"tag %s:%s already exists — it is attached to %s %q (%s); remove it there before attaching here",
		tagType, value, conflict.EntityType, conflict.Name, conflict.ExternalKey,
	)
}
```

Then change the error line in **both** `AddTagToAsset` and `AddTagToLocation`. Each currently ends:
```go
	if err != nil {
		return nil, parseTagError(err, tagType, req.Value)
	}
```
Change the `parseTagError(...)` call to:
```go
	if err != nil {
		return nil, s.resolveTagError(ctx, orgID, err, tagType, req.Value)
	}
```
(`parseTagError` itself is unchanged — it stays the fallback.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `just backend test-integration -run TestAddTag_CrossAssetConflict ./internal/storage/`
Expected: PASS.

- [ ] **Step 5: Add the cross-location and soft-deleted cases**

Append to `tags_conflict_integration_test.go`:

```go
func TestAddTag_CrossLocationConflict(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	locID := seedLocation(t, pool, orgID, "LOC-DOCK3", "Dock 3")
	assetB := testutil.CreateTestAsset(t, pool, orgID, "AST-B")

	value := "E2000000CONFLICT02"
	_, err := store.AddTagToLocation(ctx, orgID, locID, rfidReq(value))
	require.NoError(t, err)

	_, err = store.AddTagToAsset(ctx, orgID, assetB.ID, rfidReq(value))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.Contains(t, err.Error(), "location")
	assert.Contains(t, err.Error(), "Dock 3", "names the conflicting location")
	assert.Contains(t, err.Error(), "LOC-DOCK3")
}

func TestAddTag_SoftDeletedRowNotBlocking(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	assetA := testutil.CreateTestAsset(t, pool, orgID, "AST-A")
	assetB := testutil.CreateTestAsset(t, pool, orgID, "AST-B")

	value := "E2000000CONFLICT03"
	tag, err := store.AddTagToAsset(ctx, orgID, assetA.ID, rfidReq(value))
	require.NoError(t, err)

	removed, err := store.RemoveAssetTag(ctx, orgID, assetA.ID, tag.ID)
	require.NoError(t, err)
	require.True(t, removed)

	// The soft-deleted row must not block re-using the value elsewhere.
	_, err = store.AddTagToAsset(ctx, orgID, assetB.ID, rfidReq(value))
	require.NoError(t, err)
}
```

- [ ] **Step 6: Run all three integration tests**

Run: `just backend test-integration -run 'TestAddTag_' ./internal/storage/`
Expected: PASS — 3 tests.

- [ ] **Step 7: Confirm the unit suite still passes; annotate the `_Duplicate` tests**

Run: `just backend test`
Expected: PASS. `TestAddTagToAsset_Duplicate` / `TestAddTagToLocation_Duplicate` still pass: under pgxmock the follow-up `lookupTagConflict` transaction has no scripted `ExpectBegin`, so it errors and `resolveTagError` falls back to the generic `parseTagError` message, which still contains `"already exists"`.

In `tags_test.go`, add one comment line inside each of those two tests (just above the `result, err := storage.AddTag...` call):
```go
	// resolveTagError attempts a follow-up lookup here; with no further mock
	// expectations scripted it errors and falls back to the generic message.
```

- [ ] **Step 8: Commit**

```bash
git add backend/internal/storage/tags.go backend/internal/storage/tags_test.go \
        backend/internal/storage/tags_conflict_integration_test.go
git commit -m "feat(backend): name the conflicting entity in tag-attach 409 (TRA-806)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Backend — handler-level 409-detail integration test

The HTTP handlers do not change — this task only proves end-to-end that the enriched detail reaches the 409 body.

**Files:**
- Create: `backend/internal/handlers/assets/tag_conflict_integration_test.go`

- [ ] **Step 1: Write the failing-then-passing integration test**

This test exercises code already implemented in Task 1, so it should pass immediately once written — it is a regression guard. Create `backend/internal/handlers/assets/tag_conflict_integration_test.go`, modeled on `tag_type_required_integration_test.go` in the same package:

```go
//go:build integration
// +build integration

package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupTagConflictRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets/{asset_id}/tags", handler.AddTag)
	return r
}

func withTagConflictOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra806@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func TestAddAssetTag_DuplicateValue_Returns409NamingConflictingEntity(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	assetA := testutil.CreateTestAsset(t, pool, orgID, "AST-A")
	assetB := testutil.CreateTestAsset(t, pool, orgID, "AST-B")

	tagType := "rfid"
	value := "E2000000HANDLER01"
	_, err := store.AddTagToAsset(context.Background(), orgID, assetA.ID,
		shared.TagRequest{TagType: &tagType, Value: value})
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupTagConflictRouter(handler)

	body := strings.NewReader(fmt.Sprintf(`{"tag_type":"rfid","value":%q}`, value))
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/assets/%d/tags", assetB.ID), body)
	req.Header.Set("Content-Type", "application/json")
	req = withTagConflictOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp.Error.Type)
	assert.Contains(t, resp.Error.Detail, "AST-A",
		"409 detail must name the conflicting asset")
}
```

- [ ] **Step 2: Run the test**

Run: `just backend test-integration -run TestAddAssetTag_DuplicateValue ./internal/handlers/assets/`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/assets/tag_conflict_integration_test.go
git commit -m "test(backend): handler 409 detail names the conflicting entity (TRA-806)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Frontend — re-enable and repair the excluded vitest suites

`AssetForm.test.tsx` and `AssetFormModal.test.tsx` are excluded in `vitest.config.ts` under the `TRA-192: Tests with incomplete store mocks` block. This task makes them green against the **current** (post-TRA-799) form behavior, before Task 5 adds new tests to `AssetForm.test.tsx`.

**Files:**
- Modify: `frontend/vitest.config.ts`
- Modify: `frontend/src/components/assets/AssetFormModal.test.tsx`
- Modify: `frontend/src/components/assets/AssetForm.test.tsx`

Frontend tests run with `just frontend test` (honors the config exclude). To run a single file during this task: `pnpm --dir frontend exec vitest run src/components/assets/AssetForm.test.tsx`.

- [ ] **Step 1: Remove the two exclude entries**

In `frontend/vitest.config.ts`, delete these two lines from the `exclude` array:
```js
      '**/src/components/assets/AssetForm.test.tsx',
      '**/src/components/assets/AssetFormModal.test.tsx',
```
Leave the rest of the `TRA-192` block intact.

- [ ] **Step 2: Run both suites to capture failures**

Run: `pnpm --dir frontend exec vitest run src/components/assets/AssetForm.test.tsx src/components/assets/AssetFormModal.test.tsx`
Expected: FAIL — roughly 18 failures across the two files. Two known root causes:
1. **`AssetFormModal.test.tsx`** — `vi.mock('@/stores')` blanks every store, so `useBarcodeStore((s) => s.barcodes)` is `undefined` and `useScanToInput` crashes on `barcodes.length`.
2. **`AssetForm.test.tsx`** — stale assertions for UI removed in TRA-799 (`Identifier` label, `Type` field, `Scan RFID`/`Scan Barcode` buttons, `Scanning RFID` placeholder), plus one `useScanToInput` spy whose return object omits `setFocused`.

- [ ] **Step 3: Repair `AssetFormModal.test.tsx`**

Replace the module-level `vi.mock('@/stores')` approach with the real-store pattern that `LocationForm.test.tsx` uses successfully. Concretely:
- Remove `vi.mock('@/stores')`.
- Keep `vi.mock('@/lib/api/assets')`.
- In `beforeEach`, instead of auto-mocking stores, `vi.spyOn(useScanToInputModule, 'useScanToInput')` and return a complete object — it MUST include every field the hook's consumers read:
  ```ts
  vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({
    startRfidScan: vi.fn(),
    startBarcodeScan: vi.fn(),
    stopScan: vi.fn(),
    isScanning: false,
    scanType: null,
    setFocused: vi.fn(),
  });
  ```
  (Import `* as useScanToInputModule from '@/hooks/useScanToInput'`.)
- For the `useAssetStore` selector behavior the modal needs, set real store state via `useAssetStore.setState(...)`/`getState()` rather than a blanket module mock — follow `LocationForm.test.tsx`'s use of `useDeviceStore.setState(...)` / `useLocationStore.getState()` as the template.

- [ ] **Step 4: Repair `AssetForm.test.tsx`**

Open the current `AssetForm.tsx` and read its actual rendered labels/placeholders, then update each stale assertion in `AssetForm.test.tsx` to match. Known stale strings to find and correct (TRA-799 removed or renamed these): `Identifier` (asset-id field label), `Type` (removed), `Scan RFID`, `Scan Barcode`, `Scanning for RFID tag...`, `Scanning for barcode...`, `/Scanning RFID/i` placeholder. Tests asserting genuinely-removed UI (e.g. the standalone `Type` field) should be deleted, not rewritten. In every `vi.spyOn(useScanToInputModule, 'useScanToInput').mockReturnValue({...})` add `setFocused: vi.fn()` — the current `AssetForm` calls `setFocused` in a `useEffect`.

- [ ] **Step 5: Run both suites to verify green**

Run: `pnpm --dir frontend exec vitest run src/components/assets/AssetForm.test.tsx src/components/assets/AssetFormModal.test.tsx`
Expected: PASS — both files.

- [ ] **Step 6: Run the full frontend suite (regression check)**

Run: `just frontend test`
Expected: PASS — no other suite regressed by un-excluding these files.

- [ ] **Step 7: Commit**

```bash
git add frontend/vitest.config.ts \
        frontend/src/components/assets/AssetForm.test.tsx \
        frontend/src/components/assets/AssetFormModal.test.tsx
git commit -m "test(frontend): re-enable AssetForm/AssetFormModal vitest suites (TRA-806)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Frontend — shared `checkTagConflict` helper

**Files:**
- Create: `frontend/src/lib/tags/conflictCheck.ts`
- Create: `frontend/src/lib/tags/conflictCheck.test.ts`
- Modify: `frontend/src/types/assets/index.ts`

- [ ] **Step 1: Write the failing helper test**

Create `frontend/src/lib/tags/conflictCheck.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { checkTagConflict } from './conflictCheck';
import { lookupApi } from '@/lib/api/lookup';

vi.mock('@/lib/api/lookup');

describe('checkTagConflict', () => {
  beforeEach(() => vi.clearAllMocks());

  it('returns null for an empty value', async () => {
    expect(await checkTagConflict('   ')).toBeNull();
  });

  it('returns null when the tag is not attached anywhere (404)', async () => {
    vi.mocked(lookupApi.byTag).mockRejectedValue({ response: { status: 404 } });
    expect(await checkTagConflict('E2-FREE')).toBeNull();
  });

  it('returns a message naming a conflicting location', async () => {
    vi.mocked(lookupApi.byTag).mockResolvedValue({
      data: { data: { entity_type: 'location', entity_id: 9, location: { id: 9, name: 'Dock 3' } } },
    } as never);
    const msg = await checkTagConflict('E2-TAKEN');
    expect(msg).toContain('location');
    expect(msg).toContain('Dock 3');
  });

  it('returns null when the hit is the entity being edited', async () => {
    vi.mocked(lookupApi.byTag).mockResolvedValue({
      data: { data: { entity_type: 'asset', entity_id: 42, asset: { id: 42, name: 'Forklift' } } },
    } as never);
    expect(await checkTagConflict('E2-OWN', { entityType: 'asset', entityId: 42 })).toBeNull();
  });

  it('returns null on an unexpected error (best-effort)', async () => {
    vi.mocked(lookupApi.byTag).mockRejectedValue({ response: { status: 500 } });
    expect(await checkTagConflict('E2-ERR')).toBeNull();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm --dir frontend exec vitest run src/lib/tags/conflictCheck.test.ts`
Expected: FAIL — `conflictCheck.ts` does not exist.

- [ ] **Step 3: Implement the helper**

Create `frontend/src/lib/tags/conflictCheck.ts`:

```ts
import { lookupApi } from '@/lib/api/lookup';

export interface ConflictSelf {
  entityType: 'asset' | 'location';
  entityId: number;
}

/**
 * Checks whether an RFID tag value is already attached to a different entity.
 * Returns a human-readable conflict message, or null when the value is free,
 * not found, or already belongs to `self` (the entity currently being edited).
 * Best-effort: any unexpected error resolves to null — the save-time 409 is
 * the correctness backstop.
 */
export async function checkTagConflict(
  value: string,
  self?: ConflictSelf,
): Promise<string | null> {
  const trimmed = value.trim();
  if (!trimmed) return null;
  try {
    const response = await lookupApi.byTag('rfid', trimmed);
    const result = response.data.data;
    if (self && result.entity_type === self.entityType && result.entity_id === self.entityId) {
      return null;
    }
    const name =
      result.asset?.name ?? result.location?.name ?? `${result.entity_type} #${result.entity_id}`;
    return `Tag already attached to ${result.entity_type} "${name}" — remove it there before attaching here.`;
  } catch (err: unknown) {
    // 404 = not attached anywhere; any other error = best-effort skip.
    return null;
  }
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `pnpm --dir frontend exec vitest run src/lib/tags/conflictCheck.test.ts`
Expected: PASS — 5 tests.

- [ ] **Step 5: Add `conflict?` to the `TagInput` type**

In `frontend/src/types/assets/index.ts`, change the `TagInput` interface to:

```ts
export interface TagInput {
  id?: number; // Present if existing tag, undefined if new
  type: 'rfid';
  value: string;
  conflict?: string; // Cross-entity conflict message; set/cleared by the forms
}
```

- [ ] **Step 6: Typecheck and commit**

Run: `just frontend typecheck`
Expected: PASS.

```bash
git add frontend/src/lib/tags/conflictCheck.ts frontend/src/lib/tags/conflictCheck.test.ts \
        frontend/src/types/assets/index.ts
git commit -m "feat(frontend): add shared checkTagConflict helper (TRA-806)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Frontend — AssetForm conflict UX

Remove the "Reassign" modal, check for conflicts on blur and on barcode scan, render the inline error, and disable Save while conflicted.

**Files:**
- Modify: `frontend/src/components/assets/AssetForm.tsx`
- Modify: `frontend/src/components/assets/AssetForm.test.tsx`

- [ ] **Step 1: Write the failing tests**

Add to `AssetForm.test.tsx` (mock the helper so the test is deterministic). At the top, with the other mocks:
```ts
import { checkTagConflict } from '@/lib/tags/conflictCheck';
vi.mock('@/lib/tags/conflictCheck');
```
Add a `describe('AssetForm - Tag conflict', () => { ... })` block with three tests:
1. *renders an inline conflict error and disables Save* — `vi.mocked(checkTagConflict).mockResolvedValue('Tag already attached to location "Dock 3" — remove it there before attaching here.')`; render `<AssetForm mode="create" onSubmit={vi.fn()} onCancel={vi.fn()} />`; add a tag row, type a value into it, blur the input; `await screen.findByText(/already attached to location "Dock 3"/)`; assert the submit button (`name: /create/i`) is `toBeDisabled()`.
2. *clears the conflict when the tag is free* — `mockResolvedValue(null)`; after a blur, assert no conflict text is shown and Save is enabled.
3. *the "Reassign" modal is gone* — assert `screen.queryByText('Tag Already Assigned')` is `null` even after entering a value.

Use `LocationForm.test.tsx`'s render/query patterns as the template for adding a tag row and locating the input.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm --dir frontend exec vitest run src/components/assets/AssetForm.test.tsx -t "Tag conflict"`
Expected: FAIL — no conflict text appears; the Reassign modal still renders.

- [ ] **Step 3: Edit `AssetForm.tsx` — remove the Reassign modal**

- Delete the import: `import { ConfirmModal } from '@/components/shared/modals/ConfirmModal';` (line ~9).
- Delete the `confirmModal` `useState` declaration (lines ~40-44).
- Delete the `handleConfirmReassign` function (lines ~165-180).
- Delete the `ConfirmModal` JSX block — the `{confirmModal && ( <ConfirmModal ... /> )}` element (lines ~518-528).

- [ ] **Step 4: Edit `AssetForm.tsx` — add the conflict-check functions**

Add these three functions to the component body (near the other handlers):

```tsx
const applyConflict = (index: number, message: string | undefined) => {
  setTagInputs((prev) =>
    prev.map((t, i) => (i === index ? { ...t, conflict: message } : t)),
  );
};

const runConflictCheck = async (index: number, value: string) => {
  const trimmed = value.trim();
  if (!trimmed) {
    applyConflict(index, undefined);
    return;
  }
  // A duplicate of another row in this same form is the local-dedup case,
  // not a cross-entity conflict — leave it to the existing dedup handling.
  if (tagInputs.some((t, i) => i !== index && t.value === trimmed)) {
    applyConflict(index, undefined);
    return;
  }
  const self =
    mode === 'edit' && asset?.id != null
      ? { entityType: 'asset' as const, entityId: asset.id }
      : undefined;
  const message = await checkTagConflict(trimmed, self);
  applyConflict(index, message ?? undefined);
};

const handleTagBlur = (index: number) => {
  setFocusedTagIndex(null);
  const input = tagInputs[index];
  if (input) void runConflictCheck(index, input.value);
};
```

Add the import: `import { checkTagConflict } from '@/lib/tags/conflictCheck';`

- [ ] **Step 5: Edit `AssetForm.tsx` — rewrite `handleBarcodeScan`**

Replace the body of `handleBarcodeScan` (lines ~99-163) so it sets the scanned value and then runs the conflict check instead of opening the modal:

```tsx
const handleBarcodeScan = async (epc: string) => {
  setIsScanning(false);
  if (!epc || epc.trim() === '') {
    toast.error('No tag data received from scanner');
    return;
  }

  // Trigger scan into the focused row.
  if (focusedTagIndex !== null && tagInputs[focusedTagIndex]) {
    if (tagInputs.some((t, i) => i !== focusedTagIndex && t.value === epc)) {
      toast.error('This tag is already in the list');
      return;
    }
    const index = focusedTagIndex;
    setTagInputs((prev) =>
      prev.map((t, i) => (i === index ? { ...t, value: epc, conflict: undefined } : t)),
    );
    void runConflictCheck(index, epc);
    return;
  }

  // Button-initiated scan: append a new row.
  if (tagInputs.some((t) => t.value === epc)) {
    toast.error('This tag is already in the list');
    return;
  }
  const newIndex = tagInputs.length;
  setTagInputs([...tagInputs, { type: 'rfid', value: epc }]);
  void runConflictCheck(newIndex, epc);
};
```

- [ ] **Step 6: Edit `AssetForm.tsx` — wire the tag row and Save button**

In the `<TagInputRow>` element (lines ~467-497):
- Change `onBlur={() => setFocusedTagIndex(null)}` to `onBlur={() => handleTagBlur(index)}`.
- Add the prop `error={tagInput.conflict}`.
- In `onValueChange`, clear the stale conflict when the value is edited — the updated row object must include `conflict: undefined`:
  ```tsx
  onValueChange={(value) => {
    setTagInputs((prev) =>
      prev.map((t, i) => (i === index ? { ...t, value, conflict: undefined } : t)),
    );
  }}
  ```

Change the submit button's `disabled` (line ~511) from `disabled={loading}` to:
```tsx
disabled={loading || tagInputs.some((t) => t.conflict)}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `pnpm --dir frontend exec vitest run src/components/assets/AssetForm.test.tsx`
Expected: PASS — the new `Tag conflict` block and the existing (Task 3) tests.

- [ ] **Step 8: Typecheck and commit**

Run: `just frontend typecheck`
Expected: PASS.

```bash
git add frontend/src/components/assets/AssetForm.tsx frontend/src/components/assets/AssetForm.test.tsx
git commit -m "feat(frontend): inline tag-conflict UX on AssetForm, remove Reassign modal (TRA-806)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Frontend — LocationForm conflict UX

Apply the Task 5 changes to `LocationForm`. It is a structural mirror of `AssetForm`; the only differences are the entity type (`'location'`) and the edited-entity id source (`location?.id`).

**Files:**
- Modify: `frontend/src/components/locations/LocationForm.tsx`
- Modify: `frontend/src/components/locations/LocationForm.test.tsx`

- [ ] **Step 1: Write the failing tests**

Add a `describe('LocationForm - Tag conflict', ...)` block to `LocationForm.test.tsx` mirroring the three Task 5 tests, with `vi.mock('@/lib/tags/conflictCheck')`. The conflict message in the mock should name an asset (e.g. `'Tag already attached to asset "Forklift 7" — remove it there before attaching here.'`) to prove the cross-direction. Assert the submit button (`name: /create/i`) becomes disabled.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm --dir frontend exec vitest run src/components/locations/LocationForm.test.tsx -t "Tag conflict"`
Expected: FAIL.

- [ ] **Step 3: Edit `LocationForm.tsx`**

Apply the exact same edits as Task 5 Steps 3-6, adjusted for LocationForm:
- Remove the `ConfirmModal` import (line ~9), the `confirmModal` state (lines ~85-89), `handleConfirmReassign` (lines ~208-223), and the `ConfirmModal` JSX block (lines ~597-607).
- Add `applyConflict`, `runConflictCheck`, `handleTagBlur` — **identical** to Task 5 Step 4 except the `self` value:
  ```tsx
  const self =
    mode === 'edit' && location?.id != null
      ? { entityType: 'location' as const, entityId: location.id }
      : undefined;
  ```
- Add `import { checkTagConflict } from '@/lib/tags/conflictCheck';`
- Rewrite `handleBarcodeScan` (lines ~142-206) using the Task 5 Step 5 body verbatim (it has no asset/location-specific code).
- In the `<TagInputRow>` element (lines ~546-576): change `onBlur` to `onBlur={() => handleTagBlur(index)}`, add `error={tagInput.conflict}`, and clear `conflict` in `onValueChange` (Task 5 Step 6).
- Change the submit button `disabled` (line ~590) to `disabled={loading || tagInputs.some((t) => t.conflict)}`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm --dir frontend exec vitest run src/components/locations/LocationForm.test.tsx`
Expected: PASS.

- [ ] **Step 5: Typecheck and commit**

Run: `just frontend typecheck`
Expected: PASS.

```bash
git add frontend/src/components/locations/LocationForm.tsx \
        frontend/src/components/locations/LocationForm.test.tsx
git commit -m "feat(frontend): inline tag-conflict UX on LocationForm, remove Reassign modal (TRA-806)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Frontend — LocationFormModal 409 parity

`LocationFormModal.tsx`'s create/update `catch` block (lines ~115-127) has no `409` branch and no generic `>= 400` branch, and never reads `err.response.data.error.detail`. A save-time conflict falls through to the final `else` and shows the useless generic Axios string. `AssetFormModal.tsx` (lines ~139-156) handles it correctly — bring the location modal to parity.

**Files:**
- Create: `frontend/src/components/locations/LocationFormModal.test.tsx`
- Modify: `frontend/src/components/locations/LocationFormModal.tsx`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/locations/LocationFormModal.test.tsx`, modeled on the (now-repaired, Task 3) `AssetFormModal.test.tsx` — same render wrapper, same store approach, `vi.mock('@/lib/api/locations')`. Write one test:
- *surfaces a save-time 409 detail* — mock `locationsApi.create` to reject with `{ response: { status: 409, data: { error: { detail: 'tag rfid:E2-X already exists — it is attached to asset "Forklift 7" (AST-7); remove it there before attaching here' } } } }`; open the modal in create mode; submit a minimally-valid location; `await screen.findByText(/attached to asset "Forklift 7"/)` to assert the real detail is shown.

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm --dir frontend exec vitest run src/components/locations/LocationFormModal.test.tsx`
Expected: FAIL — the modal shows `"Request failed with status code 409"`, not the detail.

- [ ] **Step 3: Edit `LocationFormModal.tsx`**

Replace the `catch` block at lines ~115-127 with this — adding the `apiError` extraction, the `409` branch, and the generic `>= 400` branch, mirroring `AssetFormModal.tsx`:

```tsx
} catch (err: any) {
  const apiError = err.response?.data?.error?.detail;

  if (err.code === 'ERR_NETWORK' || err.message?.includes('Network Error')) {
    setError('Cannot connect to server. Please check your connection and try again.');
  } else if (err.response?.status === 404) {
    setError('Location API endpoint not found. The backend may not be running.');
  } else if (err.response?.status === 409) {
    setError(apiError || 'A tag on this location is already attached elsewhere.');
  } else if (err.response?.status >= 500) {
    setError(apiError || 'Server error. Please try again later.');
  } else if (err.response?.status >= 400) {
    setError(apiError || err.message || 'Invalid request. Please check your input.');
  } else {
    setError(err.message || 'An error occurred. Please try again.');
  }
} finally {
  setLoading(false);
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `pnpm --dir frontend exec vitest run src/components/locations/LocationFormModal.test.tsx`
Expected: PASS.

- [ ] **Step 5: Run the full validation suite**

Run: `just validate`
Expected: PASS — lint, typecheck, tests, build across both workspaces. (Backend integration tests are not part of `just validate`; they were run in Tasks 1-2.)

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/locations/LocationFormModal.tsx \
        frontend/src/components/locations/LocationFormModal.test.tsx
git commit -m "fix(frontend): surface save-time 409 detail on LocationFormModal (TRA-806)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Done criteria

- A duplicate tag attach returns 409 whose `detail` names the conflicting asset/location (Tasks 1-2).
- Typing or scanning an already-attached tag on either form shows an inline red-bordered error on that row and disables Save (Tasks 5-6).
- The "Reassign" modal no longer exists in either form (Tasks 5-6).
- A save-time 409 surfaces its real detail on both the asset and location modals (Task 7 + pre-existing asset behavior).
- `AssetForm.test.tsx` / `AssetFormModal.test.tsx` run in the default suite again (Task 3).
- `just validate` passes; `just backend test-integration ./internal/storage/ ./internal/handlers/assets/` passes.

## Not in this plan
- The E2E Playwright scenario (spec's test plan) — run against preview after merge, per the e2e batching norm.
- TRA-803 (asset edit: tag-only change doesn't persist) — separate ticket.
