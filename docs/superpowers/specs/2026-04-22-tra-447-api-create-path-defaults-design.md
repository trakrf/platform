# TRA-447 — API create-path defaults, type enum, parent natural key, strict bodies

**Linear:** [TRA-447](https://linear.app/trakrf/issue/TRA-447/api-create-path-defaults-is-active-valid-from-type-enum-parent)
**Related:** TRA-210 (public read-endpoint shapes), TRA-429 (unified write responses), TRA-446 (hierarchy API-key auth, parallel)
**Date:** 2026-04-22
**Status:** Approved — ready for implementation plan

## Problem

Black-box evaluation of the public asset/location create paths (2026-04-22) surfaced four consumer-facing defects — D4, D5/B3, D6/B4, D7/B5 — all rooted in the same shape: handlers were written for the UI (which sends complete payloads) and rely on Go zero-value defaults that are wrong for API consumers. This is iteration 5 of the black-box → fix cycle, so the bar is **close all obvious gaps now** rather than patching to the letter of each finding and shipping the remaining noise back through testing.

The target customer is API-savvy. First-impression polish wins over expedience.

## Findings in scope

| Id | Surface | Symptom |
|----|---------|---------|
| D4 | `POST /api/v1/assets` | `type: "widget"` → 400 with `invalid_value`, no list of accepted values. `type` documented as closed enum of `"asset"` only. |
| D5/B3 | `POST /api/v1/locations` | `parent`, `parent_identifier`, `parent_path`, or `path` in body → 201 with `depth: 1`. Silently ignored. No natural-key way for customers to create children. |
| D6/B4 | `POST /api/v1/assets` | Omit `is_active` → response reports `false`. Default list view filters `is_active=true`, so the record is invisible after create. |
| D7/B5 | `POST /api/v1/assets` (and `/locations`) | Omit `valid_from` → record stored with Go `time.Time{}` zero-value (`0001-01-01T00:00:00Z`). Looks like a bug to any consumer. |

## Out of scope (explicit)

- **Bulk import** (`POST /api/v1/assets/bulk`): defaults live in CSV parser, separate surface, already a backlog ticket.
- **Type-differentiated behavior**: list filtering by type works as a pass-through already; reports, UI labels, kind-specific state machines are downstream tickets.
- **Asset identifier auto-generation for locations** (assets already have it; locations still require caller-supplied `identifier`). Inconsistent but out of spec.
- **Global 401 error-taxonomy refactor** (noted in TRA-446 parent ticket).

## Approach

Ship a clean public create/update contract across both resources in one PR. Nine coordinated changes:

### 1. Three-value asset `type` enum backed by a DB check

Product positions asset/person/inventory tracking as first-class. The current DB `CHECK (type = 'asset')` enforces a narrower claim than the product makes. Replace with `CHECK (type IN ('asset', 'person', 'inventory'))`. DB default `'asset'` stays. Code does not differentiate behavior by value yet; kind-specific features are separate tickets. Docs will say "value is stored and returned; kind-specific behavior is planned."

- New migration pair under `backend/migrations/`:
  - `NNNN_assets_type_expand.up.sql`: drop `assets_type_check`, add `CHECK (type IN ('asset','person','inventory'))`.
  - `NNNN_assets_type_expand.down.sql`: re-apply single-value CHECK (after first UPDATE-ing any non-`asset` rows to `asset` — defensive for future reverts).

### 2. Make `type` optional on `CreateAssetRequest`

Customers shouldn't have to send the field at all. Change struct tag:
```go
Type string `json:"type,omitempty" validate:"omitempty,oneof=asset person inventory" enums:"asset,person,inventory" example:"asset"`
```
Handler sets `type = "asset"` if empty-string after decode (relies on `omitempty` not firing for empty string → we use an explicit post-decode default to be clear). DB already defaults, but making it explicit in the handler keeps the Go storage layer honest.

### 3. Pointer-flip `IsActive` and `ValidFrom` on the create-request structs

Absence must be distinguishable from Go's zero value.

- `CreateAssetRequest.IsActive`: `bool` → `*bool`
- `CreateAssetRequest.ValidFrom`: `shared.FlexibleDate` → `*shared.FlexibleDate`
- `CreateLocationRequest.IsActive`: `bool` → `*bool`
- `CreateLocationRequest.ValidFrom`: `shared.FlexibleDate` → `*shared.FlexibleDate`

Handler applies defaults after decode + validate:
```go
if request.IsActive == nil { t := true; request.IsActive = &t }
if request.ValidFrom == nil || request.ValidFrom.IsZero() {
    fd := shared.FlexibleDate{Time: time.Now().UTC()}
    request.ValidFrom = &fd
}
```

Explicit `null` is treated as "not provided" (defaults apply). Explicit `false` on `is_active` is respected.

Storage dereferences the pointers with a `nil → sane-default` fallback so any direct storage caller (bulk import, future) doesn't silently insert garbage if they forget to set them.

**Update paths are unaffected.** `UpdateAssetRequest` / `UpdateLocationRequest` already use pointers with `nil = don't change` semantics — existing and correct.

### 4. `parent_identifier` on location create and update

Add `ParentIdentifier *string` (validated `omitempty,min=1,max=255`) to both `CreateLocationRequest` and `UpdateLocationRequest`. Handler resolves to the internal surrogate via `storage.GetLocationByIdentifier(orgID, *request.ParentIdentifier)` before calling the storage write.

Failure modes:
- Parent not found (including cross-org, which returns nil from the store) → `400 bad_request`, message `"parent_identifier 'foo' not found"`.
- Both `parent_identifier` and `parent_location_id` sent and they disagree → `400 bad_request`, message `"parent_identifier and parent_location_id disagree"`. (Belt-and-suspenders: the UI uses `parent_location_id`; the public API uses `parent_identifier`; a request with both is almost certainly a bug.)
- Both sent and they agree → accepted, resolved value wins (same either way).
- Empty-string `parent_identifier: ""` on update is NOT a "detach" operation (treat as nil). Detach is not in this ticket's DoD.

Update path extension beyond the ticket's literal DoD is intentional: asymmetric create/update (create accepts `parent_identifier`, update silently ignores it) would be its own black-box finding next iteration.

### 5. `swaggerignore` on public OpenAPI for `parent_location_id`

The surrogate FK is an internal perf detail per the platform memo. Public docs should show only the natural key. Add `swaggerignore:"true"` to `ParentLocationID` on `CreateLocationRequest` / `UpdateLocationRequest`. The field remains in the Go struct (so the UI, which sends it, keeps working) — it just doesn't appear in the generated public spec.

Accept that a customer who guesses `parent_location_id: 42` will still have it decoded (rule 6 catches genuinely unknown fields, not documented-but-ignored ones). That's an acceptable compromise: the public docs are clean, and a customer who goes out of their way to send an undocumented integer gets what they get.

### 6. `DisallowUnknownFields` on all four affected endpoints

`POST /assets`, `PUT /assets/{identifier}` (and the `by-id` internal variant), `POST /locations`, `PUT /locations/{identifier}` (and `by-id`). Any JSON field not named on the target Go struct → `400 bad_request`, message `"unknown field: <name>"`.

Implementation: extend `httputil.DecodeJSON` with a `DecodeJSONStrict` variant that calls `dec.DisallowUnknownFields()` before decoding. Wire strict variant into these six handlers.

Closes D5's silent-ignore bug at the root and catches every future silent-ignore across these surfaces. UI send-list needs a compatibility audit (below).

### 7. Structured validation errors via `FieldError.Params`

Extend `apierrors.FieldError`:
```go
type FieldError struct {
    Field   string         `json:"field"`
    Code    string         `json:"code"`
    Message string         `json:"message"`
    Params  map[string]any `json:"params,omitempty"`
}
```

Populate from validator tag + param in `httputil.validation.go`:

| Tag | Code | Params |
|-----|------|--------|
| `oneof` | `invalid_value` | `allowed_values: [...]` (split `fe.Param()` on whitespace) |
| `min` on string/slice | `too_short` | `min_length: N` |
| `max` on string/slice | `too_long`  | `max_length: N` |
| `min` on numeric | `too_small` | `min: N` |
| `max` on numeric | `too_large` | `max: N` |

Message stays human-readable and now embeds the values (e.g. `"type must be one of: asset, person, inventory"`). `Params` gives programmatic access. `omitempty` keeps responses compact for tags that don't populate params.

This is a shared-envelope change. Ships with this PR under the "one-time first-impression polish" framing.

### 8. OpenAPI regeneration

Drives from code annotations: `swag init` → `docs/swagger.json` → `apispec` tool → `docs/api/openapi.public.{json,yaml}` (committed) + internal (gitignored). Changes:

- `CreateAssetRequest.Type`: `enums:"asset,person,inventory"`, optional, default doc.
- `CreateAssetRequest.ValidFrom` / `CreateLocationRequest.ValidFrom`: optional with "defaults to current time if omitted" description.
- `CreateAssetRequest.IsActive` / `CreateLocationRequest.IsActive`: optional with "defaults to true if omitted" description.
- `CreateLocationRequest.ParentIdentifier` / `UpdateLocationRequest.ParentIdentifier`: documented with example + "resolves to internal parent_location_id server-side."
- `CreateLocationRequest.ParentLocationID` / `UpdateLocationRequest.ParentLocationID`: hidden (`swaggerignore`).
- `FieldError.Params`: documented as `object` with a short description and note that populated keys depend on `code`.
- Response docs for 400 mention the unknown-field rejection on affected endpoints.

### 9. UI compatibility audit

Before merging, verify the UI still works by running through the asset/location create + edit flows in the preview environment. Specific checks:

- UI sends `is_active` / `valid_from` / `type` explicitly on create? If yes, no change. If UI omits them, defaults kick in — still correct.
- UI sends `parent_location_id` on location create/update? Expected yes — the `swaggerignore`-hidden field remains decoded. Good.
- UI never sends fields not present in the Go struct? Needs empirical verification to avoid `DisallowUnknownFields` regressions.

If the UI audit finds incompatible sends, fix them in the same PR (they're bugs either way).

## Integration test coverage

All under `//go:build integration`. Added to `backend/internal/handlers/assets/public_write_integration_test.go` and `backend/internal/handlers/locations/public_write_integration_test.go` (co-located with existing public-API tests).

**Asset create:**
- Omit `is_active` → `201`; response `is_active: true`; subsequently appears in default `GET /assets` list view.
- Omit `valid_from` → `201`; response `valid_from` within ±5s of request time.
- `type: "widget"` → `400 validation_error`; `fields[0].code = "invalid_value"`; `fields[0].message` contains "asset, person, inventory"; `fields[0].params.allowed_values` = `["asset", "person", "inventory"]`.
- Omit `type` → `201`; response `type: "asset"`.
- `type: "person"` → `201`; response `type: "person"`.
- Unknown field `foo: "bar"` → `400 bad_request` with "unknown field: foo".
- Explicit `is_active: false` → `201`; response `is_active: false`; record absent from default list (expected).

**Location create:**
- Omit `is_active` / `valid_from` → defaults (mirrors asset cases).
- `parent_identifier: "<existing>"` → `201`; response `parent_identifier` matches, `depth > 1`.
- `parent_identifier: "ghost"` → `400` with "not found" message.
- Both `parent_identifier` + matching `parent_location_id` → `201`.
- Both `parent_identifier` + mismatching `parent_location_id` → `400` with "disagree" message.
- Unknown field `parent_path` (and separately `parent`, `path`) → `400` with "unknown field: ..." message.

**Location update:**
- `parent_identifier: "<existing>"` on a root → `200`; parent set correctly, `depth` now `>1`.
- `parent_identifier: "ghost"` → `400`.
- Unknown field → `400`.

**Validation envelope:**
- `TestRespondValidationError_*` in `validation_test.go` — extend to assert the new `Params` field for `oneof`, `min`, `max`.

## Risks

- **UI regression from `DisallowUnknownFields`.** Mitigated by the audit step; we fix any UI-side over-shares in the same PR.
- **Pointer-flip test literal churn.** ~15 test-file sites initialize `CreateAssetRequest` / `CreateLocationRequest` literals with `IsActive: true` / `ValidFrom: shared.FlexibleDate{...}`. All must be converted to pointer syntax. Mechanical but grep-and-edit.
- **`FieldError.Params` envelope addition.** `omitempty` means existing consumers see no change on validation codes we don't populate. New consumers can opt in.
- **Migration + rollback defensiveness.** The down migration UPDATE-s any `person`/`inventory` rows to `asset` before restoring the narrow CHECK. This is data loss in the reverted column sense; acceptable for pre-launch.

## Files changed (estimated)

- `backend/migrations/NNNN_assets_type_expand.{up,down}.sql` (new)
- `backend/internal/models/asset/asset.go` (struct changes, OpenAPI annotations)
- `backend/internal/models/location/location.go` (struct changes, OpenAPI annotations)
- `backend/internal/models/errors/errors.go` (FieldError.Params)
- `backend/internal/handlers/assets/assets.go` (defaults, strict decode)
- `backend/internal/handlers/locations/locations.go` (defaults, parent resolve, strict decode)
- `backend/internal/util/httputil/decode.go` (DecodeJSONStrict)
- `backend/internal/util/httputil/validation.go` (params extraction)
- `backend/internal/util/httputil/validation_test.go` (assert Params on unit tests)
- `backend/internal/storage/assets.go` (deref pointers)
- `backend/internal/storage/locations.go` (deref pointers)
- `backend/internal/services/bulkimport/service.go` (adapt to pointer fields)
- `backend/internal/handlers/assets/*_test.go` (literal updates + new cases)
- `backend/internal/handlers/locations/*_test.go` (literal updates + new cases)
- `docs/api/openapi.public.{json,yaml}` (regenerated, committed)

## Success criteria

1. All 20+ new integration tests pass against a real DB.
2. Existing test suite continues to pass.
3. `swag init` + `apispec` regeneration produces a clean diff limited to the intended changes.
4. UI preview environment exercises asset + location create/edit without regression.
5. A manual `curl` replay of the four original black-box findings (D4, D5, D6, D7) shows the documented behavior, not the ticketed bugs.
