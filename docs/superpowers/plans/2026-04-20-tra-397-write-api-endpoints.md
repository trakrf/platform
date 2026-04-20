# TRA-397 Write API endpoints — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire API-key authentication + `write` scope enforcement to the public write surface (assets, locations, identifier attachments, inventory save) with cross-org isolation at the storage layer and structured audit logging on every write.

**Architecture:** Same chi router, shared handler functions, new public-write group under `EitherAuth` + `WriteAudit` middlewares with per-route `RequireScope("<resource>:write")`. Handlers switch from session-only `GetUserClaims(r).CurrentOrgID` to principal-agnostic `middleware.GetRequestOrgID(r)`. Storage methods that previously trusted handler-supplied IDs now take an `orgID` argument and include `AND org_id = $n` in SQL — closing the IDOR gap opened by exposing write ops to API keys. Audit logging is non-persistent: one structured zerolog line per completed write request.

**Tech Stack:** Go 1.22 backend (chi, pgx, JWT-via-golang-jwt/v5, zerolog), Postgres with TimescaleDB.

**Reference issue:** [TRA-397](https://linear.app/trakrf/issue/TRA-397) — sub-issue of [TRA-210](https://linear.app/trakrf/issue/TRA-210). Blocked-by: TRA-396 (landed, PR #163). Blocks: TRA-398 (webhooks).

**Parallel-ticket coordination:** TRA-404 **has landed on main (commit a8d790e)**. Its changes to honor in this branch (already rebased): (a) `storage.UpdateLocation` translates pg duplicate-key / parent-FK errors into user-facing strings (`"location with identifier X already exists"`, `"invalid parent_location_id: ..."`); (b) `handlers.locations.Create` and `handlers.locations.Update` detect `"already exists"` and return 409 Conflict; (c) `@Failure 409` swagger annotation on `Create`. Tasks 3 and 5 below preserve these additions — call sites of `storage.UpdateLocation` gain an `orgID` arg but the error-translation body stays; handler rewrites keep the 409 branch.

**Descoped (explicit):**
- Bulk operations (CSV import/export, idempotency keys) — stays on session-only routes
- Persistent audit-log table — handled as a future ticket if customer demand warrants
- OAuth 2.0 client credentials flow — TRA-210 tracks as Phase 2
- Rate limiting — separate ticket

**Context for executors:**
- Always run commands from the project root. Use `just backend <cmd>` / `just frontend <cmd>` delegation rather than `cd`.
- Backend integration tests require `INTEGRATION_TESTS=1` env and a live Postgres (see `backend/internal/testutil/database.go`).
- Conventional commits: `feat(tra-397):`, `test(tra-397):`, `refactor(tra-397):`, `fix(tra-397):`, `docs(tra-397):`.
- Frequent incremental commits. Do NOT squash or amend during execution.

---

## Design decisions (non-obvious calls)

1. **Inventory save → `scans:write` scope.** Inventory save writes `asset_scans` rows. The read counterpart already uses `scans:read`. Adding `scans:write` keeps the noun-based scope model symmetric (every resource has read + write). Rejected alternatives: reuse `assets:write` (inventory ≠ asset CRUD) or introduce `inventory:write` (collides with `scans:read` for history queries on the same data).

2. **Cross-org isolation at storage, not handler.** Handlers could Get→compare→Update, but that has a TOCTOU window and doubles DB roundtrips. Adding `AND org_id = $n` to UPDATE/DELETE SQL is atomic and zero-cost. The handler signatures for `UpdateAsset`, `DeleteAsset`, `UpdateLocation`, `DeleteLocation` gain an `orgID` parameter; `RemoveIdentifier` uses a `WHERE EXISTS` subquery against the identifier's owning resource so we don't need a tri-tier API.

3. **"Not found" vs "forbidden" when cross-org.** Storage returning zero affected rows is mapped to HTTP 404 (not 403). Reveals less about what exists in other orgs. Matches TRA-396's public-read cross-org behavior (`TestGetAssetByIdentifier_CrossOrgReturns404`).

4. **Audit logging = structured zerolog, not DB table.** One `Info().Str("event","api.write").Int("org_id",...).Str("principal","api_key:<jti>"|"user:<id>").Str("method",...).Str("path",...).Int("status",...).Str("request_id",...)` line per completed write. No migration, no new service, trivially grep-able. Persistent audit table tracked as future work.

5. **Write routes move out of `Handler.RegisterRoutes`.** Following the pattern TRA-396 established for public reads: write routes are registered directly in `internal/cmd/serve/router.go` under the `EitherAuth` + `WriteAudit` group. `RegisterRoutes` keeps only session-only surface (bulk CSV, `by-id` reads).

6. **`@Security APIKey[scope]` annotation fixups.** Several write handlers already carry `@Security APIKey[assets:write]` etc. Two lag — `locations.Delete` says `@Security BearerAuth`, `inventory.Save` says `@Security BearerAuth`. Fix both as part of this ticket so the internal OpenAPI stays honest.

---

## File Structure

**Backend — new files:**
- `backend/internal/middleware/write_audit.go` — `WriteAudit` middleware
- `backend/internal/middleware/write_audit_test.go` — unit tests
- `backend/internal/handlers/assets/public_write_integration_test.go` — API-key write flow tests
- `backend/internal/handlers/locations/public_write_integration_test.go` — API-key write flow tests
- `backend/internal/handlers/inventory/public_write_integration_test.go` — API-key write flow tests

**Backend — modified files:**
- `backend/internal/models/apikey/apikey.go` — add `"scans:write"` to `ValidScopes`
- `backend/internal/storage/assets.go` — `UpdateAsset`, `DeleteAsset` take `orgID`; SQL adds `AND org_id = $n`
- `backend/internal/storage/locations.go` — `UpdateLocation`, `DeleteLocation` take `orgID`; same SQL treatment
- `backend/internal/storage/identifiers.go` — `RemoveIdentifier` takes `orgID`; SQL uses `WHERE EXISTS` against owning asset/location
- `backend/internal/handlers/assets/assets.go` — `Create`, `UpdateAsset`, `DeleteAsset`, `AddIdentifier`, `RemoveIdentifier` switch to `GetRequestOrgID`; pass orgID to storage
- `backend/internal/handlers/locations/locations.go` — `Create`, `Update`, `Delete`, `AddIdentifier`, `RemoveIdentifier` same treatment; fix `@Security BearerAuth` on `Delete`
- `backend/internal/handlers/inventory/save.go` — `Save` switches to `GetRequestOrgID`; fix `@Security BearerAuth`
- `backend/internal/cmd/serve/router.go` — new public-write group under `EitherAuth` + `WriteAudit`; writes pulled out of `Handler.RegisterRoutes` call sites
- `docs/api/openapi.public.yaml` — add write paths (POST/PUT/DELETE for assets, locations, identifier attachments; POST inventory/save)

---

## Task 1: Add `scans:write` scope

**Files:**
- Modify: `backend/internal/models/apikey/apikey.go:5-12`

- [ ] **Step 1.1: Add the scope constant**

Edit `backend/internal/models/apikey/apikey.go`:

```go
var ValidScopes = map[string]bool{
    "assets:read":     true,
    "assets:write":    true,
    "locations:read":  true,
    "locations:write": true,
    "scans:read":      true,
    "scans:write":     true,
}
```

- [ ] **Step 1.2: Run the apikey package tests**

Run: `just backend test ./internal/models/apikey/...`
Expected: existing tests PASS, including any validation tests that iterate over `ValidScopes`.

- [ ] **Step 1.3: Commit**

```bash
git add backend/internal/models/apikey/apikey.go
git commit -m "feat(tra-397): add scans:write scope for inventory writes"
```

---

## Task 2: Cross-org isolation — `UpdateAsset` / `DeleteAsset`

**Why:** Once API keys can hit `PUT /api/v1/assets/{id}` and `DELETE /api/v1/assets/{id}`, a key scoped to org A can target a surrogate ID belonging to org B unless the storage layer enforces org ownership in the SQL. Fix: take `orgID` in the storage method and add `AND org_id = $n` to the UPDATE.

**Files:**
- Modify: `backend/internal/storage/assets.go:83-125` (`UpdateAsset`)
- Modify: `backend/internal/storage/assets.go:253-260` (`DeleteAsset`)
- Modify: `backend/internal/handlers/assets/assets.go` (call sites)
- Create: `backend/internal/storage/assets_crossorg_test.go` — new integration test file

- [ ] **Step 2.1: Write the failing cross-org UPDATE test**

Create `backend/internal/storage/assets_crossorg_test.go`:

```go
//go:build integration
// +build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestUpdateAsset_CrossOrgReturnsNotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := testutil.CreateTestAccount(t, pool)

	created, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgA,
		Identifier: "asset-a",
		Name:       "Owned by A",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	newName := "should-not-be-applied"
	result, err := store.UpdateAsset(context.Background(), orgB, created.ID, assetmodel.UpdateAssetRequest{
		Name: &newName,
	})
	require.NoError(t, err)
	assert.Nil(t, result, "cross-org update must return nil (not found), not apply the change")

	fetched, err := store.GetAssetByID(context.Background(), &created.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "Owned by A", fetched.Name, "original asset must be untouched by cross-org update")
}

func TestDeleteAsset_CrossOrgReturnsFalse(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := testutil.CreateTestAccount(t, pool)

	created, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgA,
		Identifier: "asset-a-del",
		Name:       "Owned by A",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	deleted, err := store.DeleteAsset(context.Background(), orgB, created.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "cross-org delete must return false")

	fetched, err := store.GetAssetByID(context.Background(), &created.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "asset must still exist")
	assert.Nil(t, fetched.DeletedAt, "asset must not be soft-deleted by cross-org delete")
}
```

- [ ] **Step 2.2: Run the test — expect compile error**

Run: `INTEGRATION_TESTS=1 just backend test -run TestUpdateAsset_CrossOrgReturnsNotFound ./internal/storage/...`
Expected: FAIL — "too many arguments in call to store.UpdateAsset" (and DeleteAsset). Signatures don't yet take orgID.

- [ ] **Step 2.3: Update `UpdateAsset` signature and SQL**

Edit `backend/internal/storage/assets.go:83` to:

```go
func (s *Storage) UpdateAsset(ctx context.Context, orgID, id int, request asset.UpdateAssetRequest) (*asset.Asset, error) {
	updates := []string{}
	args := []any{id, orgID}
	argPos := 3
	fields, err := mapReqToFields(request)

	if err != nil {
		return nil, err
	}

	for key, value := range fields {
		if value != nil {
			updates = append(updates, fmt.Sprintf("%s = $%d", key, argPos))
			args = append(args, value)
			argPos++
		}
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	query := fmt.Sprintf(`
		update trakrf.assets
		set %s, updated_at = now()
		where id = $1 and org_id = $2 and deleted_at is null
		returning id, org_id, identifier, name, type, description, current_location_id, valid_from, valid_to,
		          metadata, is_active, created_at, updated_at, deleted_at
	`, strings.Join(updates, ", "))

	var a asset.Asset
	err = s.pool.QueryRow(ctx, query, args...).Scan(&a.ID, &a.OrgID,
		&a.Identifier, &a.Name, &a.Type, &a.Description,
		&a.CurrentLocationID, &a.ValidFrom, &a.ValidTo, &a.Metadata, &a.IsActive,
		&a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if strings.Contains(err.Error(), "already exists") {
			return nil, err
		}
		return nil, fmt.Errorf("could not update asset: %w", err)
	}

	return &a, nil
}
```

Note: the existing `if err == pgx.ErrNoRows` branch already returns `(nil, nil)` — which is what we want for cross-org hits. The addition of `and org_id = $2` turns cross-org updates into `ErrNoRows`.

- [ ] **Step 2.4: Update `DeleteAsset` signature and SQL**

Edit `backend/internal/storage/assets.go:253-260` to:

```go
func (s *Storage) DeleteAsset(ctx context.Context, orgID, id int) (bool, error) {
	query := `update trakrf.assets set deleted_at = now() where id = $1 and org_id = $2 and deleted_at is null`
	result, err := s.pool.Exec(ctx, query, id, orgID)
	if err != nil {
		return false, fmt.Errorf("could not delete asset: %w", err)
	}
	return result.RowsAffected() > 0, nil
}
```

Note the signature change: `id *int` → `id int` (pointer was gratuitous).

- [ ] **Step 2.5: Update the handler call sites in `assets.go`**

Edit `backend/internal/handlers/assets/assets.go` — `UpdateAsset` handler:

```go
func (handler *Handler) UpdateAsset(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetUpdateFailed, "missing organization context", ctx)
		return
	}

	idParam := chi.URLParam(req, "id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.AssetUpdateInvalidID, idParam), err.Error(), ctx)
		return
	}

	var request asset.UpdateAssetRequest

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.AssetUpdateInvalidReq, err.Error(), ctx)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.ValidationFailed, err.Error(), ctx)
		return
	}

	result, err := handler.storage.UpdateAsset(req.Context(), orgID, id, request)

	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.AssetUpdateFailed, err.Error(), ctx)
			return
		}
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetUpdateFailed, err.Error(), ctx)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]*asset.Asset{"data": result})
}
```

And `DeleteAsset` handler:

```go
func (handler *Handler) DeleteAsset(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetDeleteFailed, "missing organization context", ctx)
		return
	}

	idParam := chi.URLParam(req, "id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.AssetDeleteInvalidID, idParam), err.Error(), ctx)
		return
	}

	deleted, err := handler.storage.DeleteAsset(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetDeleteFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]bool{"deleted": deleted})
}
```

- [ ] **Step 2.6: Find and update any other in-tree call sites**

Run: `just backend grep -n "storage.UpdateAsset\|storage.DeleteAsset\|\.UpdateAsset(\|\.DeleteAsset("`
(Or: `rg -n "storage\.(Update|Delete)Asset|\.(Update|Delete)Asset\(" backend/`)

Inspect each hit. Typical locations: other handlers, test files, bulk jobs. Update every call to pass `orgID` as the new first int arg (after `ctx`). For `DeleteAsset`, also drop the `*int` pointer-wrapping at call sites.

- [ ] **Step 2.7: Build and run the test**

Run: `just backend build ./... && INTEGRATION_TESTS=1 just backend test -run "TestUpdateAsset_CrossOrgReturnsNotFound|TestDeleteAsset_CrossOrgReturnsFalse" ./internal/storage/...`
Expected: both tests PASS.

- [ ] **Step 2.8: Run the full storage + handler test packages**

Run: `INTEGRATION_TESTS=1 just backend test ./internal/storage/... ./internal/handlers/assets/...`
Expected: all existing tests still PASS (session-auth asset tests must continue to work since `GetRequestOrgID` falls through to session claims).

- [ ] **Step 2.9: Commit**

```bash
git add backend/internal/storage/assets.go \
        backend/internal/storage/assets_crossorg_test.go \
        backend/internal/handlers/assets/assets.go
git commit -m "feat(tra-397): enforce org isolation on asset update/delete storage"
```

---

## Task 3: Cross-org isolation — `UpdateLocation` / `DeleteLocation`

**Files:**
- Modify: `backend/internal/storage/locations.go:44-78` (`UpdateLocation`)
- Modify: `backend/internal/storage/locations.go:287-294` (`DeleteLocation`)
- Modify: `backend/internal/handlers/locations/locations.go` (call sites)
- Create: `backend/internal/storage/locations_crossorg_test.go`

- [ ] **Step 3.1: Write the failing cross-org test**

Create `backend/internal/storage/locations_crossorg_test.go`:

```go
//go:build integration
// +build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestUpdateLocation_CrossOrgReturnsNil(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := testutil.CreateTestAccount(t, pool)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgA,
		Identifier: "wh-a",
		Name:       "Owned by A",
		Path:       "wh-a",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	newName := "should-not-be-applied"
	result, err := store.UpdateLocation(context.Background(), orgB, created.ID, locmodel.UpdateLocationRequest{
		Name: &newName,
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestDeleteLocation_CrossOrgReturnsFalse(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := testutil.CreateTestAccount(t, pool)

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgA,
		Identifier: "wh-a-del",
		Name:       "Owned by A",
		Path:       "wh-a-del",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	deleted, err := store.DeleteLocation(context.Background(), orgB, created.ID)
	require.NoError(t, err)
	assert.False(t, deleted)
}
```

- [ ] **Step 3.2: Run test — expect compile failure**

Run: `INTEGRATION_TESTS=1 just backend test -run TestUpdateLocation_CrossOrgReturnsNil ./internal/storage/...`
Expected: FAIL — "too many arguments in call to store.UpdateLocation".

- [ ] **Step 3.3: Update `UpdateLocation` signature and SQL**

Edit `backend/internal/storage/locations.go:44`:

```go
func (s *Storage) UpdateLocation(ctx context.Context, orgID, id int, request location.UpdateLocationRequest) (*location.Location, error) {
	updates := []string{}
	args := []any{id, orgID}
	argPos := 3
	fields, err := mapLocationReqToFields(request)

	if err != nil {
		return nil, err
	}

	for key, value := range fields {
		if value != nil {
			updates = append(updates, fmt.Sprintf("%s = $%d", key, argPos))
			args = append(args, value)
			argPos++
		}
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	query := fmt.Sprintf(`
		UPDATE trakrf.locations
		SET %s, updated_at = NOW()
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL
		RETURNING id, org_id, name, identifier, parent_location_id, path, depth,
		          description, valid_from, valid_to, is_active, created_at, updated_at, deleted_at
	`, strings.Join(updates, ", "))

	var loc location.Location
	err = s.pool.QueryRow(ctx, query, args...).Scan(&loc.ID, &loc.OrgID, &loc.Name,
		&loc.Identifier, &loc.ParentLocationID, &loc.Path, &loc.Depth, &loc.Description,
		&loc.ValidFrom, &loc.ValidTo, &loc.IsActive, &loc.CreatedAt, &loc.UpdatedAt, &loc.DeletedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			identifier := "unknown"
			if request.Identifier != nil {
				identifier = *request.Identifier
			}
			return nil, fmt.Errorf("location with identifier %s already exists", identifier)
		}
		if strings.Contains(err.Error(), "parent_location_id_fkey") {
			return nil, fmt.Errorf("invalid parent_location_id: parent location does not exist")
		}
		return nil, fmt.Errorf("failed to update location: %w", err)
	}

	return &loc, nil
}
```

The error-translation block is TRA-404's contribution — keep it verbatim. The only edits relative to main are: (a) `orgID int` added to signature, (b) `args := []any{id, orgID}` with `argPos := 3`, (c) `AND org_id = $2` in the WHERE.

- [ ] **Step 3.4: Update `DeleteLocation` signature and SQL**

Edit `backend/internal/storage/locations.go:287`:

```go
func (s *Storage) DeleteLocation(ctx context.Context, orgID, id int) (bool, error) {
	query := `UPDATE trakrf.locations SET deleted_at = NOW() WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`
	result, err := s.pool.Exec(ctx, query, id, orgID)
	if err != nil {
		return false, fmt.Errorf("could not delete location: %w", err)
	}
	return result.RowsAffected() > 0, nil
}
```

- [ ] **Step 3.5: Update handler call sites in `locations.go`**

Rewrite `locations.Update` handler (lines 135–175) to pull orgID from `GetRequestOrgID`:

```go
func (handler *Handler) Update(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationUpdateFailed, "missing organization context", ctx)
		return
	}

	idParam := chi.URLParam(req, "id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationUpdateInvalidID, idParam), err.Error(), ctx)
		return
	}

	var request location.UpdateLocationRequest
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.LocationUpdateInvalidReq, err.Error(), ctx)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.ValidationFailed, err.Error(), ctx)
		return
	}

	result, err := handler.storage.UpdateLocation(req.Context(), orgID, id, request)

	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.LocationUpdateFailed, err.Error(), ctx)
			return
		}
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationUpdateFailed, err.Error(), ctx)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]*location.Location{"data": result})
}
```

The 409 branch is TRA-404's contribution — keep it. The only edits relative to main are: (a) `orgID, err := middleware.GetRequestOrgID(req)` preamble, (b) `orgID` threaded into the `UpdateLocation` call, (c) `if result == nil → 404` branch added (cross-org updates now return `nil, nil` instead of applying).

Rewrite `locations.Delete` handler (lines 188–208) — also fix the `@Security BearerAuth` docstring to `@Security APIKey[locations:write]`:

```go
// @Summary Delete location
// @Description Soft delete a location by ID
// @Tags locations,internal
// @Accept json
// @Produce json
// @Param id path int true "Location ID"
// @Success 202 {object} map[string]bool "deleted: true/false"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid location ID"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security APIKey[locations:write]
// @Router /api/v1/locations/{id} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationDeleteFailed, "missing organization context", ctx)
		return
	}

	idParam := chi.URLParam(req, "id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationDeleteInvalidID, idParam), err.Error(), ctx)
		return
	}

	deleted, err := handler.storage.DeleteLocation(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationDeleteFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]bool{"deleted": deleted})
}
```

- [ ] **Step 3.6: Find and update other call sites**

Run: `rg -n "storage\.(Update|Delete)Location|\.(Update|Delete)Location\(" backend/`
Update every call to pass `orgID` as the new first int arg after `ctx`.

- [ ] **Step 3.7: Build and run the new tests**

Run: `just backend build ./... && INTEGRATION_TESTS=1 just backend test -run "TestUpdateLocation_CrossOrgReturnsNil|TestDeleteLocation_CrossOrgReturnsFalse" ./internal/storage/...`
Expected: both tests PASS.

- [ ] **Step 3.8: Run surrounding test packages**

Run: `INTEGRATION_TESTS=1 just backend test ./internal/storage/... ./internal/handlers/locations/...`
Expected: all existing tests still PASS.

- [ ] **Step 3.9: Commit**

```bash
git add backend/internal/storage/locations.go \
        backend/internal/storage/locations_crossorg_test.go \
        backend/internal/handlers/locations/locations.go
git commit -m "feat(tra-397): enforce org isolation on location update/delete storage"
```

---

## Task 4: Cross-org isolation — `RemoveIdentifier`

**Why:** `RemoveIdentifier` takes only `identifierID` and soft-deletes it by primary key. An API key for org A can delete any identifier by ID. Fix: require `orgID` and scope the delete to identifiers whose owning asset or location belongs to that org.

**Files:**
- Modify: `backend/internal/storage/identifiers.go:130-139` (`RemoveIdentifier`)
- Modify: `backend/internal/handlers/assets/assets.go` (`RemoveIdentifier` call site)
- Modify: `backend/internal/handlers/locations/locations.go` (`RemoveIdentifier` call site)
- Create: `backend/internal/storage/identifiers_crossorg_test.go`

- [ ] **Step 4.1: Write the failing cross-org test**

Create `backend/internal/storage/identifiers_crossorg_test.go`:

```go
//go:build integration
// +build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestRemoveIdentifier_CrossOrgReturnsFalse(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := testutil.CreateTestAccount(t, pool)

	created, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgA,
		Identifier: "ident-host-a",
		Name:       "A",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToAsset(context.Background(), orgA, created.ID, shared.TagIdentifierRequest{
		Type:  "epc",
		Value: "EPC-CROSS-ORG",
	})
	require.NoError(t, err)

	deleted, err := store.RemoveIdentifier(context.Background(), orgB, ident.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "cross-org identifier removal must return false")
}
```

- [ ] **Step 4.2: Run test — expect compile failure**

Run: `INTEGRATION_TESTS=1 just backend test -run TestRemoveIdentifier_CrossOrgReturnsFalse ./internal/storage/...`
Expected: FAIL — "too many arguments in call to store.RemoveIdentifier".

- [ ] **Step 4.3: Update `RemoveIdentifier` signature and SQL**

Edit `backend/internal/storage/identifiers.go:130`:

```go
func (s *Storage) RemoveIdentifier(ctx context.Context, orgID, identifierID int) (bool, error) {
	query := `
		UPDATE trakrf.identifiers
		SET deleted_at = NOW()
		WHERE id = $1
		  AND deleted_at IS NULL
		  AND (
		    EXISTS (SELECT 1 FROM trakrf.assets    WHERE id = trakrf.identifiers.asset_id    AND org_id = $2)
		    OR
		    EXISTS (SELECT 1 FROM trakrf.locations WHERE id = trakrf.identifiers.location_id AND org_id = $2)
		  )
	`
	result, err := s.pool.Exec(ctx, query, identifierID, orgID)
	if err != nil {
		return false, fmt.Errorf("failed to remove identifier: %w", err)
	}
	return result.RowsAffected() > 0, nil
}
```

Note: `identifiers.asset_id` and `identifiers.location_id` are the FK columns set by `AddIdentifierToAsset` / `AddIdentifierToLocation`. Verify the column names match — if they differ (e.g. `owning_asset_id`), use the actual names. Quick grep: `rg -n "INSERT INTO trakrf.identifiers" backend/internal/storage/`.

- [ ] **Step 4.4: Update `assets.RemoveIdentifier` handler call site**

Edit `backend/internal/handlers/assets/assets.go:497-524`:

```go
func (handler *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetDeleteFailed, "missing organization context", requestID)
		return
	}

	idParam := chi.URLParam(r, "id")
	_, err = strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.AssetGetInvalidID, idParam), err.Error(), requestID)
		return
	}

	identifierIDParam := chi.URLParam(r, "identifierId")
	identifierID, err := strconv.Atoi(identifierIDParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"invalid identifier ID", err.Error(), requestID)
		return
	}

	deleted, err := handler.storage.RemoveIdentifier(r.Context(), orgID, identifierID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetDeleteFailed, err.Error(), requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]bool{"deleted": deleted})
}
```

- [ ] **Step 4.5: Update `locations.RemoveIdentifier` handler call site**

Apply the same pattern in `backend/internal/handlers/locations/locations.go:567-594` — add orgID extraction, pass to `storage.RemoveIdentifier(ctx, orgID, identifierID)`.

- [ ] **Step 4.6: Find and update other call sites**

Run: `rg -n "storage\.RemoveIdentifier|\.RemoveIdentifier\(" backend/`
Update every call.

- [ ] **Step 4.7: Build and run the new test**

Run: `just backend build ./... && INTEGRATION_TESTS=1 just backend test -run TestRemoveIdentifier_CrossOrgReturnsFalse ./internal/storage/...`
Expected: PASS.

- [ ] **Step 4.8: Run surrounding test packages**

Run: `INTEGRATION_TESTS=1 just backend test ./internal/storage/... ./internal/handlers/assets/... ./internal/handlers/locations/...`
Expected: all PASS.

- [ ] **Step 4.9: Commit**

```bash
git add backend/internal/storage/identifiers.go \
        backend/internal/storage/identifiers_crossorg_test.go \
        backend/internal/handlers/assets/assets.go \
        backend/internal/handlers/locations/locations.go
git commit -m "feat(tra-397): enforce org isolation on identifier removal"
```

---

## Task 5: Handler principal resolution — `Create` + `AddIdentifier` + `Save`

**Why:** Five handlers still read the caller via `middleware.GetUserClaims(r).CurrentOrgID`, which returns nil for API-key principals. Switch them to `middleware.GetRequestOrgID(r)` so they work under both auth paths with no other behavior change.

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go:82-90` (`Create`) and `427-436` (`AddIdentifier`)
- Modify: `backend/internal/handlers/locations/locations.go:75-84` (`Create`) and `497-506` (`AddIdentifier`)
- Modify: `backend/internal/handlers/inventory/save.go:58-68` and `45-56` (`@Security` docstring)

- [ ] **Step 5.1: `assets.Create` — switch to `GetRequestOrgID`**

In `backend/internal/handlers/assets/assets.go:82`, replace lines 85–91 (the claims extraction block) with:

```go
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetCreateFailed, "missing organization context", requestID)
		return
	}
```

Leave `request.OrgID = orgID` and the rest of `Create` unchanged.

- [ ] **Step 5.2: `assets.AddIdentifier` — switch to `GetRequestOrgID`**

In `backend/internal/handlers/assets/assets.go:427`, replace lines 430–436 with:

```go
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetCreateFailed, "missing organization context", requestID)
		return
	}
```

Note: the existing handler declares `err` later with `:=` when calling `strconv.Atoi(idParam)`. Change that to `err =` (plain assignment) since we now declare `err` earlier, or rename the orgID err and keep the downstream `:=`. Simplest: change the downstream `err :=` to `err =`.

- [ ] **Step 5.3: `locations.Create` — switch to `GetRequestOrgID`**

In `backend/internal/handlers/locations/locations.go:75`, replace lines 78–84 with:

```go
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", requestID)
		return
	}
```

Note: this function is also the site TRA-404 will touch (lines 108–111 error mapping). Our edit is at 78–84 — zero overlap.

- [ ] **Step 5.4: `locations.AddIdentifier` — switch to `GetRequestOrgID`**

In `backend/internal/handlers/locations/locations.go:497`, replace lines 500–506 with:

```go
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", requestID)
		return
	}
```

Same `err` shadowing caveat as Step 5.2.

- [ ] **Step 5.5: `inventory.Save` — switch to `GetRequestOrgID` and fix `@Security`**

Edit `backend/internal/handlers/inventory/save.go:45-68`:

```go
// Save handles POST /api/v1/inventory/save
// @Summary Save inventory scans
// @Description Persist scanned RFID assets to the asset_scans hypertable
// @Tags inventory,public
// @Accept json
// @Produce json
// @Param request body SaveRequest true "Save request with location and asset IDs"
// @Success 201 {object} map[string]any "data: SaveInventoryResult"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid request"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "Location or assets not owned by org"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security APIKey[scans:write]
// @Router /api/v1/inventory/save [post]
func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.InventorySaveFailed, "missing organization context", requestID)
		return
	}

	// 2. Decode and validate request
	var request SaveRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.InvalidJSON, err.Error(), requestID)
		return
	}
	// ... (rest unchanged)
```

- [ ] **Step 5.6: Build and run existing handler tests**

Run: `just backend build ./... && INTEGRATION_TESTS=1 just backend test ./internal/handlers/assets/... ./internal/handlers/locations/... ./internal/handlers/inventory/...`
Expected: all existing session-auth tests PASS (session path flows through `GetRequestOrgID` unchanged).

- [ ] **Step 5.7: Commit**

```bash
git add backend/internal/handlers/assets/assets.go \
        backend/internal/handlers/locations/locations.go \
        backend/internal/handlers/inventory/save.go
git commit -m "refactor(tra-397): write handlers use GetRequestOrgID for principal-agnostic org resolution"
```

---

## Task 6: `WriteAudit` middleware

**Files:**
- Create: `backend/internal/middleware/write_audit.go`
- Create: `backend/internal/middleware/write_audit_test.go`

- [ ] **Step 6.1: Write the failing test**

Create `backend/internal/middleware/write_audit_test.go`:

```go
package middleware_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/middleware"
)

func TestWriteAudit_LogsAPIKeyPrincipal(t *testing.T) {
	var buf bytes.Buffer
	prev := logger.Get()
	defer logger.SetForTest(prev)
	logger.SetForTest(zerolog.New(&buf))

	handler := middleware.WriteAudit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":1}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", strings.NewReader(`{}`))
	req = req.WithContext(middleware.WithAPIKeyPrincipalForTest(req.Context(), &middleware.APIKeyPrincipal{
		OrgID:  42,
		Scopes: []string{"assets:write"},
		JTI:    "jti-abc",
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line), "audit middleware must emit a single JSON log line")
	assert.Equal(t, "api.write", line["event"])
	assert.EqualValues(t, 42, line["org_id"])
	assert.Equal(t, "api_key:jti-abc", line["principal"])
	assert.Equal(t, http.MethodPost, line["method"])
	assert.Equal(t, "/api/v1/assets", line["path"])
	assert.EqualValues(t, 201, line["status"])
}

func TestWriteAudit_LogsSessionPrincipal(t *testing.T) {
	var buf bytes.Buffer
	prev := logger.Get()
	defer logger.SetForTest(prev)
	logger.SetForTest(zerolog.New(&buf))

	handler := middleware.WriteAudit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/7", strings.NewReader(`{}`))
	userID := 99
	req = req.WithContext(middleware.WithUserClaimsForTest(req.Context(), &middleware.UserClaims{
		UserID:        userID,
		CurrentOrgID:  intPtr(17),
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "user:99", line["principal"])
	assert.EqualValues(t, 17, line["org_id"])
}

func TestWriteAudit_LogsUnauthenticatedWithZeroOrg(t *testing.T) {
	var buf bytes.Buffer
	prev := logger.Get()
	defer logger.SetForTest(prev)
	logger.SetForTest(zerolog.New(&buf))

	handler := middleware.WriteAudit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/3", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "anonymous", line["principal"])
	assert.EqualValues(t, 0, line["org_id"])
	assert.EqualValues(t, 401, line["status"])
}

func intPtr(v int) *int { return &v }
```

The test references three helpers that don't yet exist:
- `logger.SetForTest(zerolog.Logger)` — lets the test swap the global logger. Add it to `backend/internal/logger/logger.go` as a new exported function.
- `middleware.WithAPIKeyPrincipalForTest(ctx, p) context.Context` — exported test hook for setting the principal; inside middleware use the existing unexported context key.
- `middleware.WithUserClaimsForTest(ctx, c) context.Context` — same, for session claims.

- [ ] **Step 6.2: Add logger test hook**

Edit `backend/internal/logger/logger.go` — add:

```go
// SetForTest replaces the global logger. Intended for tests only.
func SetForTest(l zerolog.Logger) {
	globalLogger = l
}
```

(Use the actual existing package-level variable name — the current file sets it through a sync.Once or similar. If the current pattern doesn't expose the variable, extract it to a package-level var so the test hook can assign to it. Check `backend/internal/logger/logger.go:Get()` for the shape.)

- [ ] **Step 6.3: Add middleware test hooks**

Edit `backend/internal/middleware/apikey.go` — add at bottom:

```go
// WithAPIKeyPrincipalForTest attaches an APIKey principal to the context.
// Exported for tests only.
func WithAPIKeyPrincipalForTest(ctx context.Context, p *APIKeyPrincipal) context.Context {
	return context.WithValue(ctx, APIKeyPrincipalKey, p)
}
```

Edit `backend/internal/middleware/middleware.go` (or wherever `GetUserClaims` lives) — add an equivalent `WithUserClaimsForTest`:

```go
func WithUserClaimsForTest(ctx context.Context, c *UserClaims) context.Context {
	return context.WithValue(ctx, userClaimsKey, c)
}
```

(Match the actual unexported context-key variable name used by `GetUserClaims`.)

- [ ] **Step 6.4: Create the middleware**

Create `backend/internal/middleware/write_audit.go`:

```go
package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/trakrf/platform/backend/internal/logger"
)

// statusRecorder intercepts the response status without buffering the body.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// WriteAudit logs one structured line per write request: principal, org, method,
// path, status, request_id. Intended to be mounted only on the public write
// route group — does not itself enforce any auth or scope.
func WriteAudit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		principal := "anonymous"
		orgID := 0

		if p := GetAPIKeyPrincipal(r); p != nil {
			principal = "api_key:" + p.JTI
			orgID = p.OrgID
		} else if c := GetUserClaims(r); c != nil {
			principal = "user:" + strconv.Itoa(c.UserID)
			if c.CurrentOrgID != nil {
				orgID = *c.CurrentOrgID
			}
		}

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		logger.Get().Info().
			Str("event", "api.write").
			Str("principal", principal).
			Int("org_id", orgID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", status).
			Str("request_id", GetRequestID(r.Context())).
			Msg(fmt.Sprintf("%s %s", r.Method, r.URL.Path))
	})
}
```

- [ ] **Step 6.5: Run the middleware tests**

Run: `just backend test ./internal/middleware/... -run WriteAudit`
Expected: three tests PASS.

- [ ] **Step 6.6: Commit**

```bash
git add backend/internal/middleware/write_audit.go \
        backend/internal/middleware/write_audit_test.go \
        backend/internal/middleware/apikey.go \
        backend/internal/middleware/middleware.go \
        backend/internal/logger/logger.go
git commit -m "feat(tra-397): add WriteAudit middleware for structured write request logging"
```

---

## Task 7: Router rewiring — public write group

**Files:**
- Modify: `backend/internal/cmd/serve/router.go` (new public-write group; remove writes from `Handler.RegisterRoutes` call sites inside the session-only group)
- Modify: `backend/internal/handlers/assets/assets.go:530-538` (drop writes from `RegisterRoutes`)
- Modify: `backend/internal/handlers/locations/locations.go:596-605` (drop writes from `RegisterRoutes`)
- Modify: `backend/internal/handlers/inventory/save.go:115-117` (drop route from `RegisterRoutes`; add comment)

- [ ] **Step 7.1: Remove public-write routes from `assets.Handler.RegisterRoutes`**

Edit `backend/internal/handlers/assets/assets.go:530-538`:

```go
// RegisterRoutes keeps only session-only surface (bulk CSV). Public write and
// identifier routes are registered directly in internal/cmd/serve/router.go
// under the EitherAuth + WriteAudit + RequireScope group. Public reads are
// also registered there (per TRA-396).
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/assets/bulk", handler.UploadCSV)
	r.Get("/api/v1/assets/bulk/{jobId}", handler.GetJobStatus)
}
```

- [ ] **Step 7.2: Remove public-write routes from `locations.Handler.RegisterRoutes`**

Edit `backend/internal/handlers/locations/locations.go:596-605`:

```go
// RegisterRoutes keeps only session-only surface (hierarchy by-id). Public write
// routes are registered in internal/cmd/serve/router.go under EitherAuth +
// WriteAudit + RequireScope. Public reads likewise (per TRA-396).
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/locations/{id}/ancestors", handler.GetAncestors)
	r.Get("/api/v1/locations/{id}/descendants", handler.GetDescendants)
	r.Get("/api/v1/locations/{id}/children", handler.GetChildren)
}
```

- [ ] **Step 7.3: Remove the inventory-save route from `inventory.Handler.RegisterRoutes`**

Edit `backend/internal/handlers/inventory/save.go:114-117`:

```go
// RegisterRoutes is intentionally empty — POST /api/v1/inventory/save is
// registered in internal/cmd/serve/router.go under the public write group
// (EitherAuth + WriteAudit + RequireScope("scans:write")).
func (h *Handler) RegisterRoutes(r chi.Router) {}
```

- [ ] **Step 7.4: Add the public-write group in `router.go`**

Edit `backend/internal/cmd/serve/router.go` — insert after the existing TRA-396 public-read group (after line 110, before the internal-only surrogate group):

```go
	// TRA-397 public write surface — accepts API-key OR session auth via EitherAuth.
	// Every route is audited via WriteAudit and gated by a per-resource write scope.
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.WriteAudit)
		r.Use(middleware.SentryContext)

		// Assets
		r.With(middleware.RequireScope("assets:write")).Post("/api/v1/assets", assetsHandler.Create)
		r.With(middleware.RequireScope("assets:write")).Put("/api/v1/assets/{id}", assetsHandler.UpdateAsset)
		r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{id}", assetsHandler.DeleteAsset)
		r.With(middleware.RequireScope("assets:write")).Post("/api/v1/assets/{id}/identifiers", assetsHandler.AddIdentifier)
		r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{id}/identifiers/{identifierId}", assetsHandler.RemoveIdentifier)

		// Locations
		r.With(middleware.RequireScope("locations:write")).Post("/api/v1/locations", locationsHandler.Create)
		r.With(middleware.RequireScope("locations:write")).Put("/api/v1/locations/{id}", locationsHandler.Update)
		r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{id}", locationsHandler.Delete)
		r.With(middleware.RequireScope("locations:write")).Post("/api/v1/locations/{id}/identifiers", locationsHandler.AddIdentifier)
		r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{id}/identifiers/{identifierId}", locationsHandler.RemoveIdentifier)

		// Inventory (scan writes)
		r.With(middleware.RequireScope("scans:write")).Post("/api/v1/inventory/save", inventoryHandler.Save)
	})
```

- [ ] **Step 7.5: Build and run the full backend test suite**

Run: `just backend build ./... && INTEGRATION_TESTS=1 just backend test ./...`
Expected: all tests PASS. Existing session-auth tests still work because `EitherAuth` routes session tokens through the same handlers.

- [ ] **Step 7.6: Commit**

```bash
git add backend/internal/cmd/serve/router.go \
        backend/internal/handlers/assets/assets.go \
        backend/internal/handlers/locations/locations.go \
        backend/internal/handlers/inventory/save.go
git commit -m "feat(tra-397): router surgery — public write routes under EitherAuth + WriteAudit + scope gates"
```

---

## Task 8: Integration tests — assets write via API key

**Files:**
- Create: `backend/internal/handlers/assets/public_write_integration_test.go`

- [ ] **Step 8.1: Write the test file**

Create `backend/internal/handlers/assets/public_write_integration_test.go`:

```go
//go:build integration
// +build integration

package assets_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/handlers/assets"
	"github.com/trakrf/platform/backend/internal/middleware"
	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func buildAssetsPublicWriteRouter(store *storage.Storage) *chi.Mux {
	handler := assets.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.WriteAudit)
		r.With(middleware.RequireScope("assets:write")).Post("/api/v1/assets", handler.Create)
		r.With(middleware.RequireScope("assets:write")).Put("/api/v1/assets/{id}", handler.UpdateAsset)
		r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{id}", handler.DeleteAsset)
		r.With(middleware.RequireScope("assets:write")).Post("/api/v1/assets/{id}/identifiers", handler.AddIdentifier)
		r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
	})
	return r
}

func TestCreateAsset_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	body := `{"identifier":"api-create-1","name":"Via API","type":"asset"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotEmpty(t, w.Header().Get("Location"))

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "api-create-1", data["identifier"])
}

func TestCreateAsset_WrongScope_Returns403(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, readOnlyToken := seedOrgAndKey(t, pool, store, "", []string{"assets:read"})
	r := buildAssetsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets",
		bytes.NewBufferString(`{"identifier":"x","name":"y","type":"asset"}`))
	req.Header.Set("Authorization", "Bearer "+readOnlyToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestUpdateAsset_CrossOrg_Returns404(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	// orgA owns the asset; orgB's API key attempts to update.
	orgA, _ := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	_, tokenB := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgA, Identifier: "orgA-asset", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	body := `{"name":"hijacked"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/"+strconv.Itoa(asset.ID), bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tokenB)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Confirm asset untouched
	fetched, err := store.GetAssetByID(context.Background(), &asset.ID)
	require.NoError(t, err)
	assert.Equal(t, "A", fetched.Name)
}

func TestDeleteAsset_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "to-delete", Name: "Bye", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/assets/"+strconv.Itoa(asset.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	var resp map[string]bool
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp["deleted"])
}

func TestAddIdentifier_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-write")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "wh", Name: "WH", Path: "wh",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	_ = loc

	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "ident-host", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)

	body := `{"type":"epc","value":"EPC-ABC-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets/"+strconv.Itoa(asset.ID)+"/identifiers",
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}
```

Add `"strconv"` to the imports if not already present.

- [ ] **Step 8.2: Run the tests**

Run: `INTEGRATION_TESTS=1 just backend test -run "TestCreateAsset_APIKey|TestCreateAsset_WrongScope|TestUpdateAsset_CrossOrg|TestDeleteAsset_APIKey|TestAddIdentifier_APIKey" ./internal/handlers/assets/...`
Expected: all PASS.

- [ ] **Step 8.3: Commit**

```bash
git add backend/internal/handlers/assets/public_write_integration_test.go
git commit -m "test(tra-397): integration tests for assets public write endpoints via API key"
```

---

## Task 9: Integration tests — locations write via API key

**Files:**
- Create: `backend/internal/handlers/locations/public_write_integration_test.go`

- [ ] **Step 9.1: Write the test file**

Create `backend/internal/handlers/locations/public_write_integration_test.go`. Mirror Task 8's structure, replacing asset-specific setup with location setup. Reuse `seedLocOrgAndKey` (defined in the existing `public_integration_test.go`). The router builder:

```go
func buildLocationsPublicWriteRouter(store *storage.Storage) *chi.Mux {
	handler := locations.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.WriteAudit)
		r.With(middleware.RequireScope("locations:write")).Post("/api/v1/locations", handler.Create)
		r.With(middleware.RequireScope("locations:write")).Put("/api/v1/locations/{id}", handler.Update)
		r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{id}", handler.Delete)
		r.With(middleware.RequireScope("locations:write")).Post("/api/v1/locations/{id}/identifiers", handler.AddIdentifier)
		r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
	})
	return r
}
```

Tests to include (one each, following the asset file's patterns):
- `TestCreateLocation_APIKey_HappyPath` — POST returns 201 with `data.identifier == "wh-1"`.
- `TestCreateLocation_WrongScope_Returns403` — token with `locations:read` → 403.
- `TestUpdateLocation_CrossOrg_Returns404` — orgB token cannot update orgA's location.
- `TestDeleteLocation_APIKey_HappyPath` — DELETE returns 202 with `"deleted":true`.
- `TestAddIdentifier_APIKey_HappyPath` — POST to `{id}/identifiers` returns 201.

- [ ] **Step 9.2: Run the tests**

Run: `INTEGRATION_TESTS=1 just backend test -run "TestCreateLocation_APIKey|TestCreateLocation_WrongScope|TestUpdateLocation_CrossOrg|TestDeleteLocation_APIKey|TestAddIdentifier_APIKey" ./internal/handlers/locations/...`
Expected: all PASS.

- [ ] **Step 9.3: Commit**

```bash
git add backend/internal/handlers/locations/public_write_integration_test.go
git commit -m "test(tra-397): integration tests for locations public write endpoints via API key"
```

---

## Task 10: Integration tests — inventory save via API key

**Files:**
- Create: `backend/internal/handlers/inventory/public_write_integration_test.go`

- [ ] **Step 10.1: Write the test file**

Create `backend/internal/handlers/inventory/public_write_integration_test.go`:

```go
//go:build integration
// +build integration

package inventory_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/handlers/inventory"
	"github.com/trakrf/platform/backend/internal/middleware"
	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

var inventoryUserCounter int64

func seedInventoryOrgAndKey(t *testing.T, pool *pgxpool.Pool, store *storage.Storage, scopes []string) (int, string) {
	t.Helper()
	orgID := testutil.CreateTestAccount(t, pool)
	n := atomic.AddInt64(&inventoryUserCounter, 1)
	var userID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash)
         VALUES ('t', $1, 'stub') RETURNING id`,
		fmt.Sprintf("inv-user-%d@t.com", n),
	).Scan(&userID))

	key, err := store.CreateAPIKey(context.Background(), orgID, "k", scopes, userID, nil)
	require.NoError(t, err)

	tok, err := jwt.GenerateAPIKey(key.JTI, orgID, scopes, nil)
	require.NoError(t, err)
	return orgID, tok
}

func buildInventoryPublicWriteRouter(store *storage.Storage) *chi.Mux {
	handler := inventory.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.WriteAudit)
		r.With(middleware.RequireScope("scans:write")).Post("/api/v1/inventory/save", handler.Save)
	})
	return r
}

func TestInventorySave_APIKey_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "inv-wh", Name: "WH", Path: "inv-wh",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "inv-asset", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildInventoryPublicWriteRouter(store)

	body := fmt.Sprintf(`{"location_id":%d,"asset_ids":[%d]}`, loc.ID, asset.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotNil(t, resp["data"])
}

func TestInventorySave_WrongScope_Returns403(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedInventoryOrgAndKey(t, pool, store, []string{"scans:read"})

	r := buildInventoryPublicWriteRouter(store)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save",
		bytes.NewBufferString(`{"location_id":1,"asset_ids":[1]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestInventorySave_CrossOrg_Returns403(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-inv")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA, _ := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})
	_, tokenB := seedInventoryOrgAndKey(t, pool, store, []string{"scans:write"})

	loc, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgA, Identifier: "xo-wh", Name: "WH", Path: "xo-wh",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	asset, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgA, Identifier: "xo-asset", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildInventoryPublicWriteRouter(store)
	body := fmt.Sprintf(`{"location_id":%d,"asset_ids":[%d]}`, loc.ID, asset.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/save", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	_ = strconv.Itoa // silence unused import if removed
}
```

Note: inventory-save already returns 403 (not 404) on cross-org because the storage layer raises `"not found or access denied"` (see `save.go:92`). That matches the existing contract; tests assert it.

- [ ] **Step 10.2: Run the tests**

Run: `INTEGRATION_TESTS=1 just backend test -run "TestInventorySave_APIKey|TestInventorySave_WrongScope|TestInventorySave_CrossOrg" ./internal/handlers/inventory/...`
Expected: all PASS.

- [ ] **Step 10.3: Commit**

```bash
git add backend/internal/handlers/inventory/public_write_integration_test.go
git commit -m "test(tra-397): integration tests for inventory save via API key"
```

---

## Task 11: Public OpenAPI spec — flip write endpoints from `internal` to `public` tag

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` — 5 `@Tags` lines
- Modify: `backend/internal/handlers/locations/locations.go` — 5 `@Tags` lines
- Regenerate: `docs/api/openapi.public.{json,yaml}` and `docs/api/openapi.internal.{json,yaml}` via `just backend api-spec`

**Goal:** Expose the 10 write endpoints in the public OpenAPI spec. The repo's `apispec` partitioner (TRA-394) splits handlers into `openapi.public` and `openapi.internal` based on the `public` / `internal` discriminator tag in the swagger `@Tags` annotation. Task 11 is a tag-flip + regen task, NOT a hand-authored YAML task — the original plan's YAML snippets are obsolete because the spec is machine-generated.

**Why the plan changed:** During Task 11 preamble, the controller discovered the public spec lives at `docs/api/openapi.public.{json,yaml}` and is emitted by `backend/internal/tools/apispec/partition.go` from swag-generated v2 specs. Each operation must have exactly one of `public` or `internal`. Inventory Save was already flipped in Task 5. The remaining 10 writes (5 asset, 5 location) need the same treatment.

- [ ] **Step 11.1: Inspect current @Tags state**

Run: `grep -n '^// @Tags' backend/internal/handlers/assets/assets.go backend/internal/handlers/locations/locations.go backend/internal/handlers/inventory/save.go`

Expected output (at time of writing):
- `assets.go:69` Create — `assets,internal` (flip)
- `assets.go:133` UpdateAsset — `assets,internal` (flip)
- `assets.go:205` GetAssetByID — `assets,internal` (**keep** — internal surrogate-ID path)
- `assets.go:251` DeleteAsset — `assets,internal` (flip)
- `assets.go:300` ListAssets — `assets,public` (keep)
- `assets.go:384` GetAssetByIdentifier — `assets,public` (keep)
- `assets.go:430` AddIdentifier — `assets,internal` (flip)
- `assets.go:499` RemoveIdentifier — `assets,internal` (flip)
- `locations.go:64` Create — `locations,internal` (flip)
- `locations.go:125` Update — `locations,internal` (flip)
- `locations.go:195` Delete — `locations,internal` (flip)
- `locations.go:241` ListLocations — `locations,public` (keep)
- `locations.go:315` GetLocationByIdentifier — `locations,public` (keep)
- `locations.go:359` GetLocationByID — `locations,internal` (**keep** — internal surrogate)
- `locations.go:400,435,470` hierarchy reads (ancestors/descendants/children) — `locations,internal` (**keep** — session-only per Task 7)
- `locations.go:506` AddIdentifier — `locations,internal` (flip)
- `locations.go:575` RemoveIdentifier — `locations,internal` (flip)
- `inventory/save.go:47` Save — `inventory,public` (keep — Task 5 already flipped)

If line numbers have drifted, map by handler function name instead of line number.

- [ ] **Step 11.2: Flip 10 `@Tags` annotations from `internal` to `public`**

For each of the 10 write handlers listed in Step 11.1, change the `@Tags` value from `<resource>,internal` to `<resource>,public`. The discriminator is stripped from the output spec, so the resource tag alone becomes the group name in Redoc.

**Assets (5 flips):**
- `assets.Create` — `@Tags assets,internal` → `@Tags assets,public`
- `assets.UpdateAsset` — `@Tags assets,internal` → `@Tags assets,public`
- `assets.DeleteAsset` — `@Tags assets,internal` → `@Tags assets,public`
- `assets.AddIdentifier` — `@Tags assets,internal` → `@Tags assets,public`
- `assets.RemoveIdentifier` — `@Tags assets,internal` → `@Tags assets,public`

**Locations (5 flips):**
- `locations.Create` — `@Tags locations,internal` → `@Tags locations,public`
- `locations.Update` — `@Tags locations,internal` → `@Tags locations,public`
- `locations.Delete` — `@Tags locations,internal` → `@Tags locations,public`
- `locations.AddIdentifier` — `@Tags locations,internal` → `@Tags locations,public`
- `locations.RemoveIdentifier` — `@Tags locations,internal` → `@Tags locations,public`

These edits are literal — just replace the word `internal` with `public` on those 10 `@Tags` lines. Preserve surrounding whitespace.

**DO NOT FLIP** — these stay internal per Task 7 scope:
- `assets.GetAssetByID` (surrogate-ID path for frontend)
- `locations.GetLocationByID` (surrogate-ID path)
- `locations.GetAncestors`, `GetDescendants`, `GetChildren` (hierarchy reads — still session-only)
- `assets.UploadCSV`, `GetJobStatus` (bulk CSV — session-only per Task 7)

- [ ] **Step 11.3: Verify the existing `@Security` annotations are correct**

For the 10 flipped handlers, confirm the `@Security` line reads `APIKey[<resource>:<verb>]` — NOT `BearerAuth`. The full expected list:

- `assets.Create` → `@Security APIKey[assets:write]` ✓ (set in original code)
- `assets.UpdateAsset` → `@Security APIKey[assets:write]` ✓
- `assets.DeleteAsset` → `@Security APIKey[assets:write]` ✓
- `assets.AddIdentifier` → `@Security APIKey[assets:write]` ✓
- `assets.RemoveIdentifier` → `@Security APIKey[assets:write]` ✓
- `locations.Create` → `@Security APIKey[locations:write]` ✓
- `locations.Update` → `@Security APIKey[locations:write]` ✓
- `locations.Delete` → `@Security APIKey[locations:write]` (Task 3 already fixed from `BearerAuth`)
- `locations.AddIdentifier` → `@Security APIKey[locations:write]` ✓
- `locations.RemoveIdentifier` → `@Security APIKey[locations:write]` ✓

If any still read `@Security BearerAuth`, update them — grep: `grep -n '@Security' backend/internal/handlers/assets/assets.go backend/internal/handlers/locations/locations.go`.

- [ ] **Step 11.4: Regenerate OpenAPI specs**

Run: `just backend api-spec`

Expected: both `docs/api/openapi.public.{json,yaml}` and `docs/api/openapi.internal.{json,yaml}` are rewritten. Verify the 10 write paths now appear in the public spec:

```bash
grep -c "/api/v1/assets:" docs/api/openapi.public.yaml   # expect 1 (POST + GET on same path)
grep -c "/api/v1/locations:" docs/api/openapi.public.yaml # expect 1
grep -c "/api/v1/inventory/save" docs/api/openapi.public.yaml # expect 1
grep -c "/api/v1/assets/{id}" docs/api/openapi.public.yaml  # expect at least 1
grep -c "/api/v1/assets/{id}/identifiers" docs/api/openapi.public.yaml # expect at least 1
```

- [ ] **Step 11.5: Validate the spec with redocly**

Run: `pnpm dlx @redocly/cli lint docs/api/openapi.public.yaml`
Expected: 0 errors.

If the Redocly "identical-paths" rule complains about `/api/v1/assets/{id}` vs `/api/v1/assets/{identifier}`, check `redocly.yaml` — TRA-396 already suppressed that rule. No action needed.

- [ ] **Step 11.6: Commit**

Commit message (plan's original message still applies — this is spec publication):

```bash
git add backend/internal/handlers/assets/assets.go \
        backend/internal/handlers/locations/locations.go \
        docs/api/openapi.public.json docs/api/openapi.public.yaml \
        docs/api/openapi.internal.json docs/api/openapi.internal.yaml
git commit -m "docs(tra-397): publish write endpoints to public OpenAPI spec"
```

---

<!-- OBSOLETE: the below YAML snippets represent the pre-rewrite plan where Task 11 hand-authored paths. They're preserved for archeological reference but MUST NOT be used. The spec is machine-generated; Steps 11.1-11.6 above are authoritative. -->

<details>
<summary>Obsolete: hand-authored YAML path snippets (do not use)</summary>

```yaml
  /api/v1/assets:
    post:
      tags: [assets]
      summary: Create an asset
      security:
        - APIKey: [assets:write]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/AssetCreateRequest'
      responses:
        '201':
          description: Created
          headers:
            Location:
              schema: { type: string }
          content:
            application/json:
              schema:
                type: object
                properties:
                  data: { $ref: '#/components/schemas/Asset' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthorized' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '409': { $ref: '#/components/responses/Conflict' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '500': { $ref: '#/components/responses/InternalError' }

  /api/v1/assets/{id}:
    put:
      tags: [assets]
      summary: Update an asset
      security:
        - APIKey: [assets:write]
      parameters:
        - { name: id, in: path, required: true, schema: { type: integer } }
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/AssetUpdateRequest'
      responses:
        '202':
          description: Accepted
          content:
            application/json:
              schema:
                type: object
                properties:
                  data: { $ref: '#/components/schemas/Asset' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthorized' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '409': { $ref: '#/components/responses/Conflict' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '500': { $ref: '#/components/responses/InternalError' }
    delete:
      tags: [assets]
      summary: Delete an asset
      security:
        - APIKey: [assets:write]
      parameters:
        - { name: id, in: path, required: true, schema: { type: integer } }
      responses:
        '202':
          description: Accepted
          content:
            application/json:
              schema:
                type: object
                properties:
                  deleted: { type: boolean }
        '401': { $ref: '#/components/responses/Unauthorized' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '500': { $ref: '#/components/responses/InternalError' }

  /api/v1/assets/{id}/identifiers:
    post:
      tags: [assets]
      summary: Attach an identifier to an asset
      security:
        - APIKey: [assets:write]
      parameters:
        - { name: id, in: path, required: true, schema: { type: integer } }
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/TagIdentifierRequest'
      responses:
        '201':
          description: Created
          content:
            application/json:
              schema:
                type: object
                properties:
                  data: { $ref: '#/components/schemas/TagIdentifier' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthorized' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '500': { $ref: '#/components/responses/InternalError' }

  /api/v1/assets/{id}/identifiers/{identifierId}:
    delete:
      tags: [assets]
      summary: Remove an identifier from an asset
      security:
        - APIKey: [assets:write]
      parameters:
        - { name: id,           in: path, required: true, schema: { type: integer } }
        - { name: identifierId, in: path, required: true, schema: { type: integer } }
      responses:
        '202':
          description: Accepted
          content:
            application/json:
              schema:
                type: object
                properties:
                  deleted: { type: boolean }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthorized' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '500': { $ref: '#/components/responses/InternalError' }
```

If the `Asset`, `AssetCreateRequest`, `AssetUpdateRequest`, `TagIdentifier`, `TagIdentifierRequest`, or shared `responses/*` entries don't yet exist in `components`, add minimal ones. Keep shapes aligned to the internal spec — copy from `openapi.internal.json` where the same entity is already defined.

- [ ] **Step 11.3: Add location write paths**

Append similar entries for:
- `POST /api/v1/locations` — `security: [{ APIKey: [locations:write] }]`
- `PUT /api/v1/locations/{id}` — `locations:write`
- `DELETE /api/v1/locations/{id}` — `locations:write`
- `POST /api/v1/locations/{id}/identifiers` — `locations:write`
- `DELETE /api/v1/locations/{id}/identifiers/{identifierId}` — `locations:write`

Use the `Location`, `LocationCreateRequest`, `LocationUpdateRequest` component schemas (add minimal ones if missing, mirroring assets).

- [ ] **Step 11.4: Add inventory save path**

```yaml
  /api/v1/inventory/save:
    post:
      tags: [inventory]
      summary: Save inventory scans
      description: Persist scanned RFID assets at a given location.
      security:
        - APIKey: [scans:write]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [location_id, asset_ids]
              properties:
                location_id: { type: integer }
                asset_ids: { type: array, items: { type: integer }, minItems: 1 }
      responses:
        '201':
          description: Created
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: object
                    additionalProperties: true
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthorized' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '500': { $ref: '#/components/responses/InternalError' }
```

Close the `<details>` block above before the next task heading.
</details>

---

## Task 12: Final validation

- [ ] **Step 12.1: Lint**

Run: `just lint`
Expected: clean. Fix any linter warnings touching TRA-397 code.

- [ ] **Step 12.2: Full backend test suite**

Run: `INTEGRATION_TESTS=1 just backend test ./...`
Expected: 0 failures.

- [ ] **Step 12.3: Frontend regression check**

Run: `just frontend lint && just frontend test`
Expected: 0 failures. The frontend uses session auth, which still flows through the same handlers via `EitherAuth`. No handler-contract changes, but `UpdateAsset` now returns 404 (not 500) when the asset doesn't belong to the caller's org — frontend won't hit that path but it's worth knowing.

- [ ] **Step 12.4: Spec generation check**

Run: `just backend api-spec`
Expected: regenerates `openapi.internal.{json,yaml}` cleanly. Inspect the diff — the `@Security` annotation fixups for locations.Delete and inventory.Save should now land in the internal spec.

- [ ] **Step 12.5: Manual smoke (optional)**

Start `just up` and exercise one write via curl with an API key:
```bash
curl -X POST https://app.preview.trakrf.id/api/v1/assets \
     -H "Authorization: Bearer $API_KEY" \
     -H "Content-Type: application/json" \
     -d '{"identifier":"smoke-1","name":"Smoke","type":"asset"}'
```
Expected: 201 Created with `Location` header + `data` body.

Skip this step if no preview-deploy API key is handy — the integration tests cover the paths.

- [ ] **Step 12.6: Commit any formatting/gen artifacts**

```bash
git add -p
# Review the diff — include only regenerated spec or formatting fixes.
git commit -m "chore(tra-397): regenerate internal OpenAPI after Security annotation fixups"
```

- [ ] **Step 12.7: Push and open PR**

```bash
git push -u origin feature/tra-397-write-api-endpoints
gh pr create --title "feat(tra-397): write API endpoints — CRUD via API key auth" --body "$(cat <<'EOF'
## Summary
- Wires API-key auth + per-resource `write` scope onto the public write surface (assets, locations, identifier attachments, inventory save) under `EitherAuth` + `WriteAudit`.
- Closes the IDOR gap that would have opened by exposing update/delete endpoints to API keys: storage-layer UPDATE/DELETE for assets, locations, and identifiers now filter by `org_id`.
- Adds `scans:write` scope for inventory save; fixes stale `@Security BearerAuth` on `locations.Delete` and `inventory.Save`.
- Every public write request emits one structured `event=api.write` log line (principal, org, method, path, status, request_id). Persistent audit table deferred.

## Test plan
- [ ] `INTEGRATION_TESTS=1 just backend test ./...` — full suite green
- [ ] Cross-org UPDATE/DELETE/identifier-removal integration tests assert 404 / false / `deleted_at IS NULL` on the victim row
- [ ] `just frontend test` — no regression (frontend stays on session auth path)
- [ ] `pnpm dlx @redocly/cli lint docs/api/openapi.public.yaml` — 0 errors

## Related
- Parent epic: TRA-210
- Blocked-by: TRA-396 (landed)
- Blocks: TRA-398 (webhooks)
- Parallel: TRA-404 touches `locations.Create` error mapping; no line overlap with this PR.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 12.8: Update Linear**

Mark TRA-397 "In Review" and paste the PR URL in the ticket.

---

## Self-Review Checklist

Ran against the spec/ticket after writing the plan:

- **Spec coverage:**
  - Assets POST/PUT/DELETE ✓ (Tasks 2, 5, 7, 8)
  - Locations POST/PUT/DELETE ✓ (Tasks 3, 5, 7, 9)
  - Identifiers POST/DELETE (both asset and location sides) ✓ (Tasks 4, 5, 7, 8, 9)
  - Inventory save ✓ (Tasks 5, 7, 10)
  - `write` scope check in middleware ✓ (Task 1 adds `scans:write`; others already exist; Task 7 wires `RequireScope("*:write")` per-route)
  - Audit logging ✓ (Task 6 middleware, wired in Task 7)
  - Existing frontend session-auth continues to work ✓ (Task 12.3, plus handlers use `GetRequestOrgID` which falls through session claims)
  - Bulk descoped ✓ (Task 7 keeps bulk on session-only)

- **Placeholder scan:** searched for TBD/TODO/"similar to"/"add appropriate" — none in the final plan.

- **Type consistency:**
  - `UpdateAsset(ctx, orgID, id, req)` — referenced consistently in Tasks 2, 5, 8
  - `DeleteAsset(ctx, orgID, id)` — consistent in Tasks 2, 5, 8 (signature changed from `*int` → `int`; step 2.4 flags it; step 2.6 sweeps other callers)
  - `UpdateLocation(ctx, orgID, id, req)` — consistent in Tasks 3, 5, 9
  - `DeleteLocation(ctx, orgID, id)` — consistent in Tasks 3, 5, 9
  - `RemoveIdentifier(ctx, orgID, identifierID)` — consistent in Tasks 4, 5, 8, 9
  - `WriteAudit` middleware — signature `func(next http.Handler) http.Handler` matches how it's mounted in Task 7 (`r.Use(middleware.WriteAudit)`, not called as `WriteAudit(store)`)
  - `GetRequestOrgID(r) (int, error)` — used consistently across all handler rewrites

- **Known external dependencies noted:**
  - `logger.SetForTest` (Task 6.2) — may require extracting the global logger into a package-level var
  - Test helpers `WithAPIKeyPrincipalForTest` / `WithUserClaimsForTest` (Task 6.3) — require matching the actual unexported context-key var names
  - `identifiers.asset_id` / `identifiers.location_id` column names (Task 4.3) — executor must verify via `rg "INSERT INTO trakrf.identifiers"` if the assumed names are wrong

- **Risks / flag items surfaced:**
  - `err` shadowing in handlers when introducing earlier `orgID, err := ...` followed by `err := strconv.Atoi(...)` — called out in steps 5.2 and 5.4
  - TRA-404 merge order — called out in the header; specific line ranges documented
  - `DeleteAsset` signature change `id *int` → `id int` — called out in step 2.4; step 2.6 instructs sweep
