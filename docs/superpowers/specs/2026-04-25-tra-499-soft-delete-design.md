# TRA-499: Soft-Delete Documentation & Lifecycle Lock-In

**Linear:** [TRA-499](https://linear.app/trakrf/issue/TRA-499/delete-assetlocation-hard-deletes-despite-docs-claiming-soft-delete)
**Branch:** `miks2u/tra-499-delete-soft-delete`
**Date:** 2026-04-25

## Premise reversal

The Linear ticket reports that `DELETE /api/v1/assets/{identifier}` and `DELETE /api/v1/locations/{identifier}` hard-delete records despite docs claiming soft-delete. Investigation shows the runtime behavior is already a soft-delete: both handlers issue `UPDATE … SET deleted_at = now()` and never call `DELETE FROM`.

The actual defects are:

1. **Misleading documentation.** Swagger annotations describe DELETE as "marks the asset inactive," conflating the soft-delete marker (`deleted_at`) with the unrelated business-state column (`is_active`).
2. **Missing test coverage.** No end-to-end test locks in the full lifecycle (POST → DELETE → GET 404 → re-POST same identifier → 201).
3. **Reporter expectation mismatch.** The ticket's acceptance criterion `GET /api/v1/assets?is_active=false` returns the soft-deleted record was based on the same conflation; this design **rejects** that criterion and keeps `is_active` semantically distinct.

## Decisions

- **Soft-deleted records remain invisible to the public API.** No new query parameter, no payload field, no opt-in. Soft-delete is an internal mechanism that supports identifier reuse and audit retention; external consumers see only live records. (`is_active` is preserved as an independent business-state flag.)
- **Re-DELETE of an already-deleted record returns `404`.** Matches Stripe's convention. Idempotent retries are a separate concern best handled via `Idempotency-Key` headers in a future change.
- **GET-by-identifier of a soft-deleted record returns `404`.** Consistent with list exclusion.
- **Identifier reuse after soft-delete is supported.** A `POST` with the identifier of a previously-deleted record creates a new row with a fresh surrogate ID. Already enabled by the partial unique index from TRA-475 (migration 000031).

## Out of scope

- Hard-delete via a separate endpoint (no customer demand).
- Cascade rules for child records of soft-deleted parents.
- UI changes to surface soft-deleted records.
- Adding `Idempotency-Key` header support.
- Refactoring `WHERE deleted_at IS NULL` filtering into a shared scope helper.
- Exposing `deleted_at` on response payloads.

## Implementation surface

### Documentation changes

Swagger annotations in:

- `backend/internal/handlers/assets/assets.go`
  - DELETE description (~line 324)
  - GET-by-identifier description (~line 517)
  - `is_active` query param description on list endpoint (~line 419)
- `backend/internal/handlers/locations/locations.go`
  - DELETE description (~line 269)
  - GET-by-identifier description (~line 461)
  - `is_active` query param description on list endpoint (~line 368)

Resulting OpenAPI spec must be regenerated via the existing toolchain (`swag` + `redocly`) and committed.

#### New annotation copy

**DELETE endpoint description (both resources):**

> Soft-delete a {resource}. The record is removed from all subsequent queries and its identifier becomes immediately available for reuse. Soft-deleted records are retained internally for audit purposes but are not retrievable via this API. Returns 204 on success, 404 if the {resource} does not exist or has already been deleted.

**GET-by-identifier description (both resources):**

> Retrieve a {resource} by its natural identifier. Returns 404 if the {resource} does not exist or has been soft-deleted.

**`is_active` query param description (both resources):**

> Filter by the active business-state flag. Independent of soft-delete: soft-deleted records are excluded from results regardless of `is_active` value.

### Handler / storage changes

None.

### Schema changes

None. Migration 000031 (TRA-475) already provides the partial unique index that allows identifier reuse after soft-delete.

### Test additions

Two new integration test files, each containing a single end-to-end lifecycle test that hits real Postgres via the existing harness:

- `backend/internal/handlers/assets/soft_delete_lifecycle_integration_test.go`
- `backend/internal/handlers/locations/soft_delete_lifecycle_integration_test.go`

#### Lifecycle test contract

For each resource:

1. `POST /api/v1/{resource}s {identifier: "TRA499-1"}` → `201`, capture surrogate ID #1.
2. `GET /api/v1/{resource}s/TRA499-1` → `200`.
3. `DELETE /api/v1/{resource}s/TRA499-1` → `204`.
4. `GET /api/v1/{resource}s/TRA499-1` → `404`.
5. `GET /api/v1/{resource}s` (default list) → response does not contain `TRA499-1`.
6. `GET /api/v1/{resource}s?is_active=false` → response does not contain `TRA499-1`.
7. `POST /api/v1/{resource}s {identifier: "TRA499-1"}` → `201`, surrogate ID #2 ≠ #1.
8. `DELETE /api/v1/{resource}s/TRA499-1` → `204` (deletes surrogate #2).
9. `DELETE /api/v1/{resource}s/TRA499-1` (re-delete, no live row) → `404`.

Tests use the same APIKey-authenticated integration pattern as `public_write_integration_test.go`. No mocks.

## Acceptance

- Swagger / OpenAPI accurately describe DELETE, GET-by-identifier, and `is_active` query param semantics for both assets and locations.
- Regenerated `openapi.yaml` is committed.
- New lifecycle integration tests pass against real Postgres.
- Existing integration tests continue to pass with no behavior changes.

## Risks

- **Spec regeneration drift.** If the OpenAPI generation tool emits unrelated formatting churn, the diff balloons. Mitigation: regenerate before annotation edits to capture baseline, then again after, so the meaningful diff is isolated.
- **Public-API consumers (TeamCentral) relying on undocumented behavior.** None known; the runtime behavior is unchanged, only the docs.
