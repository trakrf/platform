# TRA-447 API Create-Path Defaults + Polish — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the four black-box findings (D4/D5/D6/D7) on public asset/location create, plus the adjacent polish (three-value type enum, strict request bodies, structured validation errors, parent natural-key on update) so the next iteration of black-box testing finds nothing in this surface.

**Architecture:** Pointer-flip `IsActive`/`ValidFrom` on create-request structs so absence is distinguishable from zero; apply defaults in handler after validate. Add `ParentIdentifier` to location create + update requests; resolve via `GetLocationByIdentifier` before storage. Expand asset `type` CHECK constraint to `{asset, person, inventory}` and make the field optional. Add `DecodeJSONStrict` (sets `DisallowUnknownFields`) and wire into the four affected endpoints. Extend `apierrors.FieldError` with structured `Params` and populate from validator tag parameters. Swaggerignore `parent_location_id` on public surface so API-savvy customers see only natural keys.

**Tech Stack:** Go backend (chi router, pgx), go-playground/validator/v10, TimescaleDB, `swag` for OpenAPI generation, integration tests under `//go:build integration`.

**Design doc:** `docs/superpowers/specs/2026-04-22-tra-447-api-create-path-defaults-design.md`

**Branch:** `miks2u/tra-447-api-create-path-defaults` (worktree at `.worktrees/tra-447`)

---

## Phase 1 — Shared infrastructure (unblocks the rest)

### Task 1: Extend `FieldError` with structured `Params` and populate from validator tags

**Files:**
- Modify: `backend/internal/models/errors/errors.go:19-24`
- Modify: `backend/internal/util/httputil/validation.go`
- Modify: `backend/internal/util/httputil/validation_test.go`

- [ ] **Step 1: Add assertion to existing unit test covering populated `Params` for `oneof`, `min`, `max`**

Append to `backend/internal/util/httputil/validation_test.go`:

```go
func TestRespondValidationError_PopulatesParams(t *testing.T) {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)

	type s struct {
		Kind string `json:"kind" validate:"required,oneof=red green blue"`
		Name string `json:"name" validate:"required,min=2,max=5"`
		Age  int    `json:"age"  validate:"gte=18,lte=99"`
	}
	err := v.Struct(s{Kind: "purple", Name: "xxxxxxxx", Age: 5})
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondValidationError(w, r, err, "req-1")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	byField := map[string]apierrors.FieldError{}
	for _, f := range resp.Error.Fields {
		byField[f.Field] = f
	}

	assert.Equal(t, []any{"red", "green", "blue"}, byField["kind"].Params["allowed_values"])
	assert.EqualValues(t, 5, byField["name"].Params["max_length"])
	assert.EqualValues(t, 18, byField["age"].Params["min"])
	assert.EqualValues(t, 99, byField["age"].Params["max"])
}
```

Imports to add at the top of the file if not already present: `"github.com/stretchr/testify/require"`, `"github.com/stretchr/testify/assert"`.

- [ ] **Step 2: Run test — verify it fails (field `Params` not yet defined)**

```bash
cd backend && go test ./internal/util/httputil/... -run TestRespondValidationError_PopulatesParams -count=1
```

Expected: `undefined: apierrors.FieldError.Params` or similar compile error.

- [ ] **Step 3: Add `Params` to `FieldError`**

Replace the struct at `backend/internal/models/errors/errors.go:19-24`:

```go
// FieldError describes a single field-level validation failure.
//
// Params carries structured, programmatically-introspectable context for
// the failure. Populated keys depend on Code:
//   - invalid_value (from oneof tag): allowed_values []string
//   - too_short / too_long (min/max on string/slice): min_length / max_length int
//   - too_small / too_large (min/max/gte/lte on numeric): min / max int
//
// Params is omitted entirely when no structured data is available.
type FieldError struct {
	Field   string         `json:"field"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Params  map[string]any `json:"params,omitempty"`
}
```

- [ ] **Step 4: Populate `Params` and embed values in `Message` in `validation.go`**

Replace the `messageForField` function at `backend/internal/util/httputil/validation.go:75-91` and the `RespondValidationError` function at `:95-112` with:

```go
// messageForField produces a short human-safe message. Embeds the
// validator parameter (e.g. allowed enum values, max length) so the
// string is informative on its own; Params carries the structured form.
func messageForField(fe validator.FieldError) string {
	switch codeForTag(fe) {
	case "required":
		return fmt.Sprintf("%s is required", fe.Field())
	case "too_short":
		return fmt.Sprintf("%s must be at least %s characters", fe.Field(), fe.Param())
	case "too_long":
		return fmt.Sprintf("%s must be at most %s characters", fe.Field(), fe.Param())
	case "too_small":
		return fmt.Sprintf("%s must be >= %s", fe.Field(), fe.Param())
	case "too_large":
		return fmt.Sprintf("%s must be <= %s", fe.Field(), fe.Param())
	case "invalid_value":
		if fe.Tag() == "oneof" && fe.Param() != "" {
			return fmt.Sprintf("%s must be one of: %s", fe.Field(),
				strings.Join(strings.Fields(fe.Param()), ", "))
		}
		return fmt.Sprintf("%s is not a valid value", fe.Field())
	}
	return fmt.Sprintf("%s failed validation", fe.Field())
}

// paramsForField returns structured context for a failure, or nil when
// nothing useful can be derived. See FieldError.Params for the key schema.
func paramsForField(fe validator.FieldError) map[string]any {
	switch codeForTag(fe) {
	case "invalid_value":
		if fe.Tag() == "oneof" && fe.Param() != "" {
			vals := strings.Fields(fe.Param())
			out := make([]any, len(vals))
			for i, v := range vals {
				out[i] = v
			}
			return map[string]any{"allowed_values": out}
		}
	case "too_short":
		if n, err := strconv.Atoi(fe.Param()); err == nil {
			return map[string]any{"min_length": n}
		}
	case "too_long":
		if n, err := strconv.Atoi(fe.Param()); err == nil {
			return map[string]any{"max_length": n}
		}
	case "too_small":
		if n, err := strconv.Atoi(fe.Param()); err == nil {
			return map[string]any{"min": n}
		}
	case "too_large":
		if n, err := strconv.Atoi(fe.Param()); err == nil {
			return map[string]any{"max": n}
		}
	}
	return nil
}

// RespondValidationError translates validator.ValidationErrors into the
// documented validation envelope and writes it.
func RespondValidationError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	var ves validator.ValidationErrors
	if !errors.As(err, &ves) {
		WriteJSONError(w, r, http.StatusBadRequest, apierrors.ErrBadRequest,
			"Bad Request", "Request validation failed", requestID)
		return
	}
	fields := make([]apierrors.FieldError, 0, len(ves))
	for _, fe := range ves {
		fields = append(fields, apierrors.FieldError{
			Field:   fe.Field(),
			Code:    codeForTag(fe),
			Message: messageForField(fe),
			Params:  paramsForField(fe),
		})
	}
	WriteJSONErrorWithFields(w, r, http.StatusBadRequest, apierrors.ErrValidation,
		"Validation failed", "Request did not pass validation", requestID, fields)
}
```

Add `"strconv"` to the imports at the top of `validation.go` if not already present.

- [ ] **Step 5: Run test — verify it passes**

```bash
cd backend && go test ./internal/util/httputil/... -count=1
```

Expected: all tests pass, including the new `TestRespondValidationError_PopulatesParams`.

- [ ] **Step 6: Commit**

```bash
cd /home/mike/platform/.worktrees/tra-447
git add backend/internal/models/errors/errors.go backend/internal/util/httputil/validation.go backend/internal/util/httputil/validation_test.go
git commit -m "feat(tra-447): structured FieldError.Params for programmatic error introspection

Populate allowed_values for oneof, min_length/max_length for string min/max,
min/max for numeric min/max/gte/lte. Message string also embeds the param so
human-readable errors are informative on their own."
```

---

### Task 2: Add `DecodeJSONStrict` helper (rejects unknown fields)

**Files:**
- Modify: `backend/internal/util/httputil/decode.go`
- Modify: `backend/internal/util/httputil/decode_test.go`

- [ ] **Step 1: Add failing unit test**

Append to `backend/internal/util/httputil/decode_test.go`:

```go
func TestDecodeJSONStrict_RejectsUnknownField(t *testing.T) {
	type target struct {
		Name string `json:"name"`
	}
	var got target
	r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"name":"x","extra":1}`))
	err := httputil.DecodeJSONStrict(r, &got)

	if err == nil {
		t.Fatalf("expected strict decode to reject unknown field, got nil")
	}
	var decErr *httputil.JSONDecodeError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected *httputil.JSONDecodeError, got %T", err)
	}
}

func TestDecodeJSONStrict_AcceptsKnownFieldsOnly(t *testing.T) {
	type target struct {
		Name string `json:"name"`
	}
	var got target
	r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"name":"x"}`))
	if err := httputil.DecodeJSONStrict(r, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "x" {
		t.Fatalf("Name = %q, want %q", got.Name, "x")
	}
}
```

Add imports if missing: `"errors"`, `"bytes"`.

- [ ] **Step 2: Run test — expect failure (function not defined)**

```bash
cd backend && go test ./internal/util/httputil/... -run TestDecodeJSONStrict -count=1
```

Expected: compile error, `undefined: httputil.DecodeJSONStrict`.

- [ ] **Step 3: Add `DecodeJSONStrict` to `decode.go`**

Append to `backend/internal/util/httputil/decode.go`:

```go
// DecodeJSONStrict is DecodeJSON with DisallowUnknownFields. Use on
// public API endpoints where unrecognised body fields should produce a
// 400 rather than being silently ignored.
func DecodeJSONStrict(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &JSONDecodeError{Cause: err}
	}
	return nil
}
```

- [ ] **Step 4: Run test — expect pass**

```bash
cd backend && go test ./internal/util/httputil/... -count=1
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/util/httputil/decode.go backend/internal/util/httputil/decode_test.go
git commit -m "feat(tra-447): DecodeJSONStrict rejects unknown JSON body fields

Closes the silent-ignore category at the decode layer. Callers opt in
endpoint-by-endpoint; DecodeJSON remains permissive for backward compat."
```

---

## Phase 2 — DB migration (unblocks type-enum work)

### Task 3: Expand `assets.type` CHECK constraint

**Files:**
- Create: `backend/migrations/000028_assets_type_expand.up.sql`
- Create: `backend/migrations/000028_assets_type_expand.down.sql`
- Create: `backend/internal/storage/assets_type_check_test.go`

- [ ] **Step 1: Write storage-level integration test verifying non-`asset` values are now accepted**

Create `backend/internal/storage/assets_type_check_test.go`:

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

	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestAssets_TypeCheck_AcceptsPersonAndInventory(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	for _, kind := range []string{"asset", "person", "inventory"} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			a, err := store.CreateAsset(context.Background(), asset.Asset{
				OrgID:      orgID,
				Identifier: "tra447-" + kind,
				Name:       kind + " record",
				Type:       kind,
				ValidFrom:  time.Now(),
				IsActive:   true,
			})
			require.NoError(t, err)
			require.NotNil(t, a)
			assert.Equal(t, kind, a.Type)
		})
	}
}

func TestAssets_TypeCheck_RejectsUnknown(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	_, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID:      orgID,
		Identifier: "tra447-widget",
		Name:       "widget",
		Type:       "widget",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.Error(t, err, "widget must violate the type CHECK constraint")
}
```

- [ ] **Step 2: Run test — expect the `person` and `inventory` subtests to fail (CHECK still enforces `type = 'asset'`)**

```bash
cd backend && go test -tags=integration ./internal/storage/... -run TestAssets_TypeCheck -count=1
```

Expected: `person` and `inventory` fail with a CHECK violation error; `asset` subtest and `RejectsUnknown` both pass.

- [ ] **Step 3: Write the up migration**

Create `backend/migrations/000028_assets_type_expand.up.sql`:

```sql
SET search_path=trakrf,public;

ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;

ALTER TABLE assets
    ADD CONSTRAINT assets_type_check
    CHECK (type IN ('asset', 'person', 'inventory'));
```

- [ ] **Step 4: Write the down migration**

Create `backend/migrations/000028_assets_type_expand.down.sql`:

```sql
SET search_path=trakrf,public;

-- Revert any rows using the expanded values before restoring the narrow CHECK.
-- Pre-launch data loss is acceptable per spec risks section.
UPDATE assets SET type = 'asset' WHERE type IN ('person', 'inventory');

ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;

ALTER TABLE assets
    ADD CONSTRAINT assets_type_check
    CHECK (type = 'asset');
```

- [ ] **Step 5: Run test — expect all subtests to pass**

```bash
cd backend && go test -tags=integration ./internal/storage/... -run TestAssets_TypeCheck -count=1
```

Expected: all four cases pass (asset/person/inventory accepted, widget rejected). `testutil.SetupTestDB` runs migrations; the new file is picked up automatically via `backend/migrations/embed.go`.

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/000028_assets_type_expand.up.sql backend/migrations/000028_assets_type_expand.down.sql backend/internal/storage/assets_type_check_test.go
git commit -m "feat(tra-447): expand assets.type CHECK to {asset, person, inventory}

Matches the positioned product surface (asset, person, inventory tracking).
No behavioural change in code yet; type is stored and returned as-is."
```

---

## Phase 3 — Asset create/update path

### Task 4: Pointer-flip `CreateAssetRequest` + handler defaults + strict decode + storage deref

Big coordinated change across the asset request path. Everything below is one commit because Go compile-all-or-nothing.

**Files:**
- Modify: `backend/internal/models/asset/asset.go:28-39` (struct definition)
- Modify: `backend/internal/handlers/assets/assets.go:56-92` (Create handler)
- Modify: `backend/internal/handlers/assets/assets.go:143-175` (doUpdateAsset)
- Modify: `backend/internal/storage/assets.go:407-454` (CreateAssetWithIdentifiers)
- Modify: `backend/internal/services/bulkimport/service.go:224-240` (bulkimport adaptor)

- [ ] **Step 1: Update the `CreateAssetRequest` struct**

Replace at `backend/internal/models/asset/asset.go:28-39`:

```go
type CreateAssetRequest struct {
	OrgID             int                  `json:"-" swaggerignore:"true"`
	Identifier        string               `json:"identifier,omitempty" validate:"omitempty,max=255"`
	Name              string               `json:"name" validate:"required,min=1,max=255"`
	Type              string               `json:"type,omitempty" validate:"omitempty,oneof=asset person inventory" enums:"asset,person,inventory" example:"asset"`
	Description       string               `json:"description,omitempty" validate:"omitempty,max=1024"`
	CurrentLocationID *int                 `json:"current_location_id,omitempty" validate:"omitempty,min=1"`
	ValidFrom         *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01"`
	ValidTo           *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01"`
	Metadata          any                  `json:"metadata,omitempty"`
	IsActive          *bool                `json:"is_active,omitempty" example:"true"`
}
```

Also update the `Type` example/enum annotation on the base `Asset` struct at line 16 to list all three values:

```go
	Type              string     `json:"type" example:"asset" enums:"asset,person,inventory" extensions:"x-extensible-enum=true"`
```

- [ ] **Step 2: Update `Handler.Create` to apply defaults and use strict decode**

Replace at `backend/internal/handlers/assets/assets.go:56-92`:

```go
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetCreateFailed, "missing organization context", requestID)
		return
	}

	var request asset.CreateAssetWithIdentifiersRequest
	if err := httputil.DecodeJSONStrict(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	// Apply API-consumer defaults for fields the UI always sends explicitly
	// but API consumers commonly omit. Absence is distinguishable from zero
	// because these fields are pointer-typed.
	if request.Type == "" {
		request.Type = "asset"
	}
	if request.IsActive == nil {
		t := true
		request.IsActive = &t
	}
	if request.ValidFrom == nil || request.ValidFrom.IsZero() {
		fd := shared.FlexibleDate{Time: time.Now().UTC()}
		request.ValidFrom = &fd
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	request.OrgID = orgID

	result, err := handler.storage.CreateAssetWithIdentifiers(r.Context(), request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.AssetCreateFailed, err.Error(), requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/assets/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": asset.ToPublicAssetView(*result)})
}
```

Add `"time"` to the imports block at the top of `assets.go` if not already present.

- [ ] **Step 3: Swap decode in `doUpdateAsset` to strict**

Replace the first line of decode at `backend/internal/handlers/assets/assets.go:147-150` (the current `if err := httputil.DecodeJSON(req, &request)` → `DecodeJSONStrict`). Final shape:

```go
	var request asset.UpdateAssetRequest
	if err := httputil.DecodeJSONStrict(req, &request); err != nil {
		httputil.RespondDecodeError(w, req, err, reqID)
		return
	}
```

- [ ] **Step 4: Storage dereferences pointers with a sane fallback**

Replace at `backend/internal/storage/assets.go:407-454` — the entire `CreateAssetWithIdentifiers` body:

```go
func (s *Storage) CreateAssetWithIdentifiers(ctx context.Context, request asset.CreateAssetWithIdentifiersRequest) (*asset.AssetWithLocation, error) {
	// Auto-generate identifier if empty
	if strings.TrimSpace(request.Identifier) == "" {
		seq, err := s.GetNextAssetSequence(ctx, request.OrgID)
		if err != nil {
			return nil, fmt.Errorf("failed to generate identifier: %w", err)
		}
		request.Identifier = GenerateAssetIdentifier(seq)
	}

	identifiersJSON, err := identifiersToJSON(request.Identifiers)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize identifiers: %w", err)
	}

	// Handler normally applies defaults; storage re-applies as a safety net
	// for direct (non-handler) callers such as bulkimport.
	var validFrom time.Time
	if request.ValidFrom != nil && !request.ValidFrom.IsZero() {
		validFrom = request.ValidFrom.ToTime()
	} else {
		validFrom = time.Now().UTC()
	}
	var validTo *time.Time
	if request.ValidTo != nil && !request.ValidTo.IsZero() {
		t := request.ValidTo.ToTime()
		validTo = &t
	}
	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}
	assetType := request.Type
	if assetType == "" {
		assetType = "asset"
	}

	query := `SELECT * FROM trakrf.create_asset_with_identifiers($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	var assetID int
	var identifierIDs []int

	err = s.pool.QueryRow(ctx, query,
		request.OrgID,
		request.Identifier,
		request.Name,
		assetType,
		request.Description,
		request.CurrentLocationID,
		validFrom,
		validTo,
		isActive,
		request.Metadata,
		identifiersJSON,
	).Scan(&assetID, &identifierIDs)

	if err != nil {
		return nil, parseAssetWithIdentifiersError(err, request.Identifier)
	}

	return s.getAssetWithLocationByID(ctx, assetID)
}
```

- [ ] **Step 5: Adapt bulkimport call site to the new pointer shape**

Replace at `backend/internal/services/bulkimport/service.go:224-240`:

```go
		// Build CreateAssetWithIdentifiersRequest. The parsed row always has
		// concrete ValidFrom / IsActive values; wrap them as pointers.
		validFrom := shared.FlexibleDate{Time: pr.asset.ValidFrom}
		isActive := pr.asset.IsActive
		request := asset.CreateAssetWithIdentifiersRequest{
			CreateAssetRequest: asset.CreateAssetRequest{
				OrgID:       pr.asset.OrgID,
				Identifier:  pr.asset.Identifier,
				Name:        pr.asset.Name,
				Type:        pr.asset.Type,
				Description: pr.asset.Description,
				ValidFrom:   &validFrom,
				IsActive:    &isActive,
			},
			Identifiers: identifiers,
		}
		if pr.asset.ValidTo != nil {
			validTo := shared.FlexibleDate{Time: *pr.asset.ValidTo}
			request.ValidTo = &validTo
		}
```

- [ ] **Step 6: Confirm build is clean**

```bash
cd backend && go build ./internal/...
```

Expected: no output (success).

- [ ] **Step 7: Run the existing asset suites to catch unrelated literal breakage**

```bash
cd backend && go test -tags=integration -run '^TestCreateAsset|^TestUpdateAsset' ./internal/handlers/assets/... -count=1
cd backend && go test -tags=integration -run Asset ./internal/storage/... -count=1
```

Expected: pass.

If any pre-existing test fails to compile because it initializes `CreateAssetRequest` with `IsActive: true` or `ValidFrom: shared.FlexibleDate{...}`, update the literal to use pointer syntax. Pattern:

```go
// Before:
req := asset.CreateAssetRequest{ ..., IsActive: true, ValidFrom: shared.FlexibleDate{Time: t} }
// After:
active := true
vf := shared.FlexibleDate{Time: t}
req := asset.CreateAssetRequest{ ..., IsActive: &active, ValidFrom: &vf }
```

Known sites to update (verify via `grep -rn 'CreateAssetRequest{' backend/internal/handlers/assets`): `assets_integration_test.go` around the `reqBody := asset.CreateAssetRequest{...}` block (~line 717). There may be one or two more; fix each surgically.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/models/asset/asset.go backend/internal/handlers/assets/assets.go backend/internal/storage/assets.go backend/internal/services/bulkimport/service.go backend/internal/handlers/assets/assets_integration_test.go
git commit -m "feat(tra-447): asset create defaults, optional type, strict JSON decode

- CreateAssetRequest.IsActive: *bool (default true when omitted)
- CreateAssetRequest.ValidFrom: *FlexibleDate (default now() when omitted)
- CreateAssetRequest.Type: optional with server-side default 'asset',
  validates against {asset, person, inventory}
- Asset create + update now use DecodeJSONStrict to 400 unknown fields
- Storage dereferences pointers with a sane-default fallback so direct
  callers (bulkimport) don't need to know the defaults"
```

---

### Task 5: Integration tests for asset create/update contract

**Files:**
- Modify: `backend/internal/handlers/assets/public_write_integration_test.go`

- [ ] **Step 1: Append the new test cases**

Append to `backend/internal/handlers/assets/public_write_integration_test.go`:

```go
func TestCreateAsset_APIKey_DefaultsIsActiveToTrue(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-create-default-active")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write", "assets:read"})
	r := buildAssetsPublicWriteRouter(store)
	rRead := buildAssetsPublicReadRouter(store)

	body := `{"identifier":"tra447-def-active","name":"No flag"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, true, data["is_active"])

	// Appears in default list (which filters is_active=true).
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listW := httptest.NewRecorder()
	rRead.ServeHTTP(listW, listReq)
	require.Equal(t, http.StatusOK, listW.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &listResp))
	found := false
	for _, item := range listResp["data"].([]any) {
		if item.(map[string]any)["identifier"] == "tra447-def-active" {
			found = true
			break
		}
	}
	assert.True(t, found, "created asset must appear in default list view")
}

func TestCreateAsset_APIKey_DefaultsValidFromToNow(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-create-default-vf")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	before := time.Now().Add(-2 * time.Second)
	body := `{"identifier":"tra447-def-vf","name":"No date"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	vf, err := time.Parse(time.RFC3339, data["valid_from"].(string))
	require.NoError(t, err)
	after := time.Now().Add(2 * time.Second)
	assert.Truef(t, vf.After(before) && vf.Before(after),
		"valid_from %s must fall within [%s, %s]", vf, before, after)
}

func TestCreateAsset_APIKey_TypeInvalidListsAllowedValues(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-create-type-invalid")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	body := `{"identifier":"tra447-bad-type","name":"x","type":"widget"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var resp modelerrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "type", resp.Error.Fields[0].Field)
	assert.Equal(t, "invalid_value", resp.Error.Fields[0].Code)
	assert.Contains(t, resp.Error.Fields[0].Message, "asset")
	assert.Contains(t, resp.Error.Fields[0].Message, "person")
	assert.Contains(t, resp.Error.Fields[0].Message, "inventory")
	require.NotNil(t, resp.Error.Fields[0].Params)
	assert.ElementsMatch(t, []any{"asset", "person", "inventory"},
		resp.Error.Fields[0].Params["allowed_values"])
}

func TestCreateAsset_APIKey_TypeOmittedDefaultsToAsset(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-create-type-default")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	body := `{"identifier":"tra447-default-type","name":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "asset", data["type"])
}

func TestCreateAsset_APIKey_TypePerson_Accepted(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-create-type-person")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	body := `{"identifier":"tra447-a-person","name":"Jane","type":"person"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "person", resp["data"].(map[string]any)["type"])
}

func TestCreateAsset_APIKey_UnknownField_Rejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-create-unknown-field")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	body := `{"identifier":"x","name":"y","type":"asset","foo":"bar"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var resp modelerrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, string(modelerrors.ErrBadRequest), resp.Error.Type)
}

func TestUpdateAsset_APIKey_UnknownField_Rejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-update-unknown-field")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	_, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID: orgID, Identifier: "tra447-u-unknown", Name: "x", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildAssetsPublicWriteRouter(store)
	body := `{"name":"x","foo":"bar"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/assets/tra447-u-unknown", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCreateAsset_APIKey_ExplicitInactive_Respected(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-assets-create-explicit-inactive")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"assets:write"})
	r := buildAssetsPublicWriteRouter(store)

	body := `{"identifier":"tra447-inactive","name":"x","is_active":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["data"].(map[string]any)["is_active"])
}
```

If `buildAssetsPublicReadRouter` does not exist, add a twin of `buildAssetsPublicWriteRouter` that wires the read handlers (look for the existing `r.Get("/api/v1/assets", handler.ListAssets)` registration pattern and replicate into a new helper at the top of the file).

- [ ] **Step 2: Run the new tests — expect pass**

```bash
cd backend && go test -tags=integration -run 'TRA447|TestCreateAsset_APIKey_|TestUpdateAsset_APIKey_UnknownField' ./internal/handlers/assets/... -count=1
```

Expected: all new cases pass.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/assets/public_write_integration_test.go
git commit -m "test(tra-447): asset create/update contract — defaults, type enum, strict bodies"
```

---

## Phase 4 — Location create/update path

### Task 6: `parent_identifier` + defaults + strict decode + swaggerignore

Location side of the same shape, plus the `parent_identifier` natural-key resolution.

**Files:**
- Modify: `backend/internal/models/location/location.go:37-55`
- Modify: `backend/internal/handlers/locations/locations.go:53-168`
- Modify: `backend/internal/storage/locations.go:427-465`
- Modify: existing location integration/test files with compile-breaking literals

- [ ] **Step 1: Update `CreateLocationRequest` and `UpdateLocationRequest`**

Replace at `backend/internal/models/location/location.go:37-55`:

```go
type CreateLocationRequest struct {
	Name             string               `json:"name" validate:"required,min=1,max=255" example:"Warehouse 1"`
	Identifier       string               `json:"identifier" validate:"required,min=1,max=255" example:"wh1"`
	ParentLocationID *int                 `json:"parent_location_id,omitempty" swaggerignore:"true" validate:"omitempty,min=1"`
	ParentIdentifier *string              `json:"parent_identifier,omitempty" validate:"omitempty,min=1,max=255" example:"wh1"`
	Description      string               `json:"description,omitempty" validate:"omitempty,max=1024" example:"Main warehouse location"`
	ValidFrom        *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14"`
	ValidTo          *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14"`
	IsActive         *bool                `json:"is_active,omitempty" example:"true"`
}

type UpdateLocationRequest struct {
	Name             *string              `json:"name,omitempty" validate:"omitempty,min=1,max=255" example:"Warehouse 1"`
	Identifier       *string              `json:"identifier,omitempty" validate:"omitempty,min=1,max=255" example:"wh1"`
	ParentLocationID *int                 `json:"parent_location_id,omitempty" swaggerignore:"true" validate:"omitempty,min=1"`
	ParentIdentifier *string              `json:"parent_identifier,omitempty" validate:"omitempty,min=1,max=255" example:"wh1"`
	Description      *string              `json:"description,omitempty" validate:"omitempty,max=1024" example:"Updated description"`
	ValidFrom        *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14"`
	ValidTo          *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14"`
	IsActive         *bool                `json:"is_active,omitempty" example:"true"`
}
```

- [ ] **Step 2: Update `Handler.Create` — strict decode, defaults, parent_identifier resolution**

Replace at `backend/internal/handlers/locations/locations.go:53-87`:

```go
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", requestID)
		return
	}

	var request location.CreateLocationWithIdentifiersRequest
	if err := httputil.DecodeJSONStrict(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	// Resolve parent_identifier → internal surrogate FK (TRA-447).
	// parent_identifier is the API-consumer natural key; parent_location_id
	// stays for the UI (hidden from public OpenAPI).
	if request.ParentIdentifier != nil && *request.ParentIdentifier != "" {
		parent, err := handler.storage.GetLocationByIdentifier(r.Context(), orgID, *request.ParentIdentifier)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
				apierrors.LocationCreateFailed, err.Error(), requestID)
			return
		}
		if parent == nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.LocationCreateFailed,
				fmt.Sprintf("parent_identifier %q not found", *request.ParentIdentifier), requestID)
			return
		}
		if request.ParentLocationID != nil && *request.ParentLocationID != parent.ID {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.LocationCreateFailed,
				"parent_identifier and parent_location_id disagree", requestID)
			return
		}
		request.ParentLocationID = &parent.ID
	}

	// API create-path defaults (TRA-447). The UI always sends these; API
	// consumers commonly omit.
	if request.IsActive == nil {
		t := true
		request.IsActive = &t
	}
	if request.ValidFrom == nil || request.ValidFrom.IsZero() {
		fd := shared.FlexibleDate{Time: time.Now().UTC()}
		request.ValidFrom = &fd
	}

	result, err := handler.storage.CreateLocationWithIdentifiers(r.Context(), orgID, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.LocationCreateFailed, err.Error(), requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/locations/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": location.ToPublicLocationView(*result)})
}
```

Add `"time"` to the imports block at the top of `locations.go` if not already present.

- [ ] **Step 3: Update `doUpdate` — strict decode + parent_identifier resolution**

Replace at `backend/internal/handlers/locations/locations.go:136-168`:

```go
func (handler *Handler) doUpdate(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	var request location.UpdateLocationRequest
	if err := httputil.DecodeJSONStrict(req, &request); err != nil {
		httputil.RespondDecodeError(w, req, err, reqID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, req, err, reqID)
		return
	}

	// Resolve parent_identifier → parent_location_id (TRA-447). Empty
	// string is treated as nil (detach not supported in this ticket).
	if request.ParentIdentifier != nil && *request.ParentIdentifier != "" {
		parent, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, *request.ParentIdentifier)
		if err != nil {
			httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
				apierrors.LocationUpdateFailed, err.Error(), reqID)
			return
		}
		if parent == nil {
			httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.LocationUpdateFailed,
				fmt.Sprintf("parent_identifier %q not found", *request.ParentIdentifier), reqID)
			return
		}
		if request.ParentLocationID != nil && *request.ParentLocationID != parent.ID {
			httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.LocationUpdateFailed,
				"parent_identifier and parent_location_id disagree", reqID)
			return
		}
		request.ParentLocationID = &parent.ID
	}

	result, err := handler.storage.UpdateLocation(req.Context(), orgID, id, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.LocationUpdateFailed, err.Error(), reqID)
			return
		}
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": location.ToPublicLocationView(*result)})
}
```

- [ ] **Step 4: Storage dereferences with sane fallback**

Replace at `backend/internal/storage/locations.go:427-465`:

```go
// CreateLocationWithIdentifiers creates a location with tag identifiers in a single transaction
func (s *Storage) CreateLocationWithIdentifiers(ctx context.Context, orgID int, request location.CreateLocationWithIdentifiersRequest) (*location.LocationWithParent, error) {
	identifiersJSON, err := identifiersToJSON(request.Identifiers)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize identifiers: %w", err)
	}

	// Handler normally applies defaults; storage re-applies as a safety net.
	var validFrom time.Time
	if request.ValidFrom != nil && !request.ValidFrom.IsZero() {
		validFrom = request.ValidFrom.ToTime()
	} else {
		validFrom = time.Now().UTC()
	}
	var validTo *time.Time
	if request.ValidTo != nil && !request.ValidTo.IsZero() {
		t := request.ValidTo.ToTime()
		validTo = &t
	}
	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}

	query := `SELECT * FROM trakrf.create_location_with_identifiers($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	var locationID int
	var identifierIDs []int

	err = s.pool.QueryRow(ctx, query,
		orgID,
		request.Identifier,
		request.Name,
		request.Description,
		request.ParentLocationID,
		validFrom,
		validTo,
		isActive,
		nil, // metadata - not used in CreateLocationRequest
		identifiersJSON,
	).Scan(&locationID, &identifierIDs)

	if err != nil {
		return nil, parseLocationWithIdentifiersError(err, request.Identifier)
	}

	return s.getLocationWithParentByID(ctx, locationID)
}
```

- [ ] **Step 5: Confirm build**

```bash
cd backend && go build ./internal/...
```

- [ ] **Step 6: Fix any compile-breaking test literals**

Expected sites (verify via `grep -rn 'CreateLocationRequest{' backend/internal/handlers/locations backend/internal/storage`):
- `backend/internal/handlers/locations/integration_test.go` (5 literal blocks — tests at ~lines 72, 116, 208, 394, 418)

For each, add two local vars above the literal and swap the embedded values to pointer:

```go
validFromFD := shared.FlexibleDate{Time: validFrom}
active := true
// inside CreateLocationRequest{...}:
    ValidFrom: &validFromFD,
    IsActive:  &active,
```

Run the existing location suites until clean:

```bash
cd backend && go test -tags=integration -run '^TestCreateLocation|^TestUpdateLocation|^TestLocations' ./internal/handlers/locations/... -count=1
cd backend && go test -tags=integration -run Location ./internal/storage/... -count=1
```

Expected: all pre-existing tests pass.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/models/location/location.go backend/internal/handlers/locations/locations.go backend/internal/storage/locations.go backend/internal/handlers/locations/integration_test.go
git commit -m "feat(tra-447): location parent_identifier + defaults + strict JSON decode

- CreateLocationRequest / UpdateLocationRequest accept parent_identifier
  (natural key); handler resolves to internal parent_location_id.
- Both-and-disagree combinations return 400 rather than silently picking.
- parent_location_id swaggerignored on public spec; UI still sends it.
- is_active/valid_from default to true/now() on create when omitted.
- Create + update use DecodeJSONStrict to 400 unknown fields."
```

---

### Task 7: Integration tests for location contract

**Files:**
- Modify: `backend/internal/handlers/locations/public_write_integration_test.go`

- [ ] **Step 1: Append new test cases**

Append to `backend/internal/handlers/locations/public_write_integration_test.go`:

```go
func TestCreateLocation_APIKey_Defaults(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-create-defaults")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	before := time.Now().Add(-2 * time.Second)
	body := `{"identifier":"tra447-loc-def","name":"Defaults"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, true, data["is_active"])
	vf, err := time.Parse(time.RFC3339, data["valid_from"].(string))
	require.NoError(t, err)
	after := time.Now().Add(2 * time.Second)
	assert.Truef(t, vf.After(before) && vf.Before(after),
		"valid_from %s must fall within [%s, %s]", vf, before, after)
}

func TestCreateLocation_APIKey_ParentIdentifier_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-create-parent-happy")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"locations:write"})
	parent, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra447-parent", Name: "Parent",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	body := `{"identifier":"tra447-child","name":"Child","parent_identifier":"tra447-parent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "tra447-parent", data["parent_identifier"])
	depth, _ := data["depth"].(float64)
	assert.Equal(t, float64(parent.Depth+1), depth)
}

func TestCreateLocation_APIKey_ParentIdentifier_NotFound(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-create-parent-404")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	body := `{"identifier":"tra447-orphan","name":"x","parent_identifier":"ghost"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "not found")
}

func TestCreateLocation_APIKey_UnknownField_Rejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-create-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	_, token := seedOrgAndKey(t, pool, store, "", []string{"locations:write"})
	r := buildLocationsPublicWriteRouter(store)

	for _, field := range []string{"parent_path", "path", "parent"} {
		t.Run(field, func(t *testing.T) {
			body := fmt.Sprintf(`{"identifier":"tra447-u-%s","name":"x","%s":"anything"}`, field, field)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}

func TestUpdateLocation_APIKey_ParentIdentifier_HappyPath(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-update-parent")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"locations:write"})
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra447-u-parent", Name: "Parent",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra447-u-child", Name: "Child",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	body := `{"parent_identifier":"tra447-u-parent"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/tra447-u-child", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, "tra447-u-parent", data["parent_identifier"])
}

func TestUpdateLocation_APIKey_UnknownField_Rejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-update-unknown")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"locations:write"})
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra447-u-unknown", Name: "x",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	body := `{"name":"x","parent_path":"nope"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/tra447-u-unknown", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCreateLocation_APIKey_ParentDisagree_Rejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "pub-locations-create-parent-disagree")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedOrgAndKey(t, pool, store, "", []string{"locations:write"})
	parent, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra447-disagree-parent", Name: "Parent",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicWriteRouter(store)
	bogusID := parent.ID + 9999
	body := fmt.Sprintf(`{"identifier":"tra447-d-child","name":"x","parent_identifier":"tra447-disagree-parent","parent_location_id":%d}`, bogusID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "disagree")
}
```

Add imports if missing at the top: `"fmt"`.

- [ ] **Step 2: Run and expect pass**

```bash
cd backend && go test -tags=integration -run 'TestCreateLocation_APIKey_(Defaults|ParentIdentifier|UnknownField|ParentDisagree)|TestUpdateLocation_APIKey_(ParentIdentifier|UnknownField)' ./internal/handlers/locations/... -count=1
```

Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/locations/public_write_integration_test.go
git commit -m "test(tra-447): location create/update contract — parent_identifier, defaults, strict bodies"
```

---

## Phase 5 — Docs + verification

### Task 8: Regenerate OpenAPI + inspect diff

**Files:**
- Regenerated: `docs/api/openapi.public.{json,yaml}`
- Regenerated: `backend/docs/swagger.{json,yaml}`

- [ ] **Step 1: Regenerate the spec**

```bash
cd /home/mike/platform/.worktrees/tra-447
touch frontend/dist/.placeholder 2>/dev/null || mkdir -p frontend/dist && touch frontend/dist/.placeholder
cd backend && just openapi
```

If a `just openapi` recipe does not exist, run the component commands directly (see `backend/justfile` — typically `swag init -g main.go --parseDependency --parseInternal -o docs` followed by `go run ./internal/tools/apispec --in docs/swagger.json --public-out ../docs/api/openapi.public --internal-out internal/handlers/swaggerspec/openapi.internal`). Restore the placeholder removal afterwards if you created one.

- [ ] **Step 2: Inspect the diff on the public spec**

```bash
git diff -- docs/api/openapi.public.yaml
```

Verify:
- `CreateAssetRequest.type` lists enum values `asset`, `person`, `inventory`; field is marked optional.
- `CreateAssetRequest.is_active` / `valid_from` are optional (no `required:` inclusion).
- `CreateLocationRequest.parent_identifier` present with description.
- `CreateLocationRequest.parent_location_id` is *not* present (swaggerignored).
- `UpdateLocationRequest.parent_identifier` present; `parent_location_id` absent.
- `FieldError.params` present on the shared error envelope schema.
- No unexpected removals or shape changes elsewhere.

- [ ] **Step 3: Commit the regenerated spec**

```bash
git add docs/api/openapi.public.json docs/api/openapi.public.yaml backend/docs/swagger.json backend/docs/swagger.yaml
git commit -m "docs(tra-447): regenerate OpenAPI for create-path polish"
```

---

### Task 9: UI compatibility audit in preview

**Files:** (read-only — this is a manual verification step)

- [ ] **Step 1: Push the branch and open a draft PR**

```bash
cd /home/mike/platform/.worktrees/tra-447
git push -u origin miks2u/tra-447-api-create-path-defaults
gh pr create --draft --title "feat(tra-447): API create-path defaults + polish" --body-file /dev/stdin <<'EOF'
## Summary
Closes TRA-447. Implements the four black-box findings (D4, D5/B3, D6/B4, D7/B5) plus the savvy-customer polish from the design doc: optional `type` with three-value enum, `parent_identifier` natural key on location create+update, `DecodeJSONStrict` on all four affected endpoints, structured `FieldError.Params`, `swaggerignore` on `parent_location_id` for the public surface.

Design: `docs/superpowers/specs/2026-04-22-tra-447-api-create-path-defaults-design.md`

## Test plan
- [ ] UI asset list view shows new asset created without `is_active` set
- [ ] UI location tree shows child created with parent via API
- [ ] UI asset/location edit flows unchanged
- [ ] Manual `curl` replay of D4/D5/D6/D7 returns the documented shapes
- [ ] Unknown field in create body returns 400 from preview

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
```

- [ ] **Step 2: Wait for preview deployment to complete (see `.github/workflows/sync-preview.yml`), then drive the UI**

Open `https://app.preview.trakrf.id` and exercise:
- Create asset with and without `is_active` / `valid_from` in the UI form
- Create location with parent selected (UI uses `parent_location_id`)
- Edit asset and location — save a small change each
- Confirm list views and detail views render correctly (no phantom fields, no 400s)

If any flow regresses:
- Capture the request body that broke (browser devtools network tab)
- Identify whether the UI sends a field that is not in the Go struct (common causes: new UI state that wasn't wired to the API)
- Fix in the same PR: either add the field to the appropriate request struct or drop it from the UI payload

- [ ] **Step 3: Run the four black-box replay curls against preview**

Export a preview API key via the UI, then:

```bash
API_KEY=<paste>
BASE=https://app.preview.trakrf.id
# D6: is_active default
curl -s -XPOST -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" \
  -d '{"identifier":"tra447-bb1","name":"bb"}' $BASE/api/v1/assets | jq .data.is_active
# expect: true

# D7: valid_from default
curl -s -XPOST -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" \
  -d '{"identifier":"tra447-bb2","name":"bb"}' $BASE/api/v1/assets | jq .data.valid_from
# expect: an RFC3339 timestamp close to now

# D4: type enum error lists allowed values with params
curl -s -XPOST -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" \
  -d '{"identifier":"tra447-bb3","name":"bb","type":"widget"}' $BASE/api/v1/assets | jq .error.fields
# expect: params.allowed_values = ["asset","person","inventory"]

# D5: parent_identifier works; unknown parent rejected
curl -s -XPOST -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" \
  -d '{"identifier":"tra447-bb-p","name":"Parent"}' $BASE/api/v1/locations
curl -s -XPOST -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" \
  -d '{"identifier":"tra447-bb-c","name":"Child","parent_identifier":"tra447-bb-p"}' $BASE/api/v1/locations | jq .data.parent_identifier
# expect: "tra447-bb-p"
```

Paste relevant output snippets into the PR description under "Test plan" as evidence.

---

### Task 10: Finalize and ship

**Files:** N/A — finalization.

- [ ] **Step 1: Run the full backend suite once more**

```bash
cd /home/mike/platform/.worktrees/tra-447/backend && just test
```

Expected: all unit and integration tests pass.

- [ ] **Step 2: Run the validator**

```bash
cd /home/mike/platform/.worktrees/tra-447 && just validate
```

Expected: lint + test pass in both workspaces.

- [ ] **Step 3: Mark PR ready for review**

```bash
gh pr ready
```

- [ ] **Step 4: Update the Linear ticket**

From the PR description or comments:
- Link the PR
- Move TRA-447 from Backlog → In Review (or the project's equivalent)

Optional: add a comment summarizing what went in and what was explicitly deferred (bulk import, type-differentiated behavior).

---

## Self-review notes

- **Spec coverage:** D4 → Task 3 + Task 4 + Task 5. D5/B3 → Task 6 + Task 7. D6/B4 → Task 4 + Task 5 / Task 6 + Task 7. D7/B5 → same. Strict bodies → Task 2 + Task 4 + Task 6 + Tasks 5/7 tests. Structured errors → Task 1 + Task 5 asserts. OpenAPI → Task 8. UI audit → Task 9.
- **No placeholders:** migration uses concrete number `000028` (next free slot after `000027_api_keys`). Code blocks are complete.
- **Type consistency:** `FieldError.Params map[string]any` named consistently; `DecodeJSONStrict` named consistently; `parent_identifier` JSON key and `ParentIdentifier` Go field consistent.
