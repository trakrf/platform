# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- TRA-692: contract-test coverage gate that asserts every value in `components.schemas.FieldErrorCode.enum` was observed at least once in a real `validation_error` response during `just backend test-contract`. Catches "declared in the enum but never wired to emission" drift across the whole enum, not just the single case that surfaces in any given BB cycle. Deterministic supplementary case runner (`backend/contract-tests/explicit_error_cases.py`) probes each code; gate (`check_field_error_coverage.py`) fails CI with the missing list.
- Initial project documentation structure
- Business Source License 1.1 with Additional Use Grant
- Code of Conduct (Contributor Covenant 2.1)
- Security policy with vulnerability reporting procedures
- Contributing guidelines with code examples and testing requirements
- CLAUDE.md for AI assistant guidance

### Changed
- TRA-692 (BB30 §1.2, behavior change): omitted or explicit-null required fields on public POST/PATCH now emit `validation_error` with `fields[].code: "required"` instead of the prior TRA-675 collapse to `too_short`. Empty string on a `min_length:1` field still emits `too_short`. Affects every public POST/PATCH that carries a length-bearing required body field — assets POST/PATCH/rename/tags, locations POST/PATCH/rename/tags. Integrators branching on `code === "required"` per the errors docs page (rather than parsing `params.min_length`) now see the documented behavior; integrators that were relying on the §1.2-buggy `too_short` for omitted fields must switch to `code === "required"` for that case (`too_short` remains correct for "value supplied but shorter than allowed").
- Migrating handheld React app as frontend component
- TRA-684 (BB29 F9 / C3, breaking): `tree_path` and `depth` removed from the `LocationView` response shape, from the locations sort enum (`?sort=tree_path` / `-tree_path` no longer recognised — fall back to `external_key`, `name`, or `created_at`), and from the strip-on-PATCH allow list. The underlying `locations.path` ltree column, the generated `depth` column, the `update_location_path` BEFORE trigger and the `cascade_location_path_change` AFTER trigger are dropped in migration `000042_drop_location_path_and_depth`. Hierarchy queries (`/locations/{id}/ancestors`, `/locations/{id}/descendants`, `GET /locations` filtered by parent, internal subtree counts) now walk `parent_id` via recursive CTE; ancestors are still root-first and descendants are still depth-first by lowercased `external_key` segments (parity with the prior ltree order at typical scale). Default list sort is now `external_key ASC, id ASC` (was `path ASC`). `POST /locations/{id}/rename` still mutates this row's `external_key` and still returns `descendant_count_affected` (the live count reachable through `parent_id`) so integrators can refresh derived natural-key joins, but no descendant row is rewritten on the server. Frontend `LocationBar` derives hierarchy depth/order from the cached locations list (parent_id walk) instead of consuming `tree_path`/`depth`. Closes BB29 F9 (case-collision footgun: `LOSSY-CASE` and `lossy-case` no longer fold to the same materialized path — they now coexist as case-distinct siblings); BB29 C3 reduces to docs-only (the misleading "join key" field is gone).
- TRA-682 BB28 fix wave (consolidated; pre-launch breaking changes):
  - **Scope rename:** `history:read` → `tracking:read`. The scope gates both `/assets/{asset_id}/history` (time-series) and `/reports/asset-locations` (current-state snapshot); the new name better describes "where things are and have been" and pairs with `assets:read` / `locations:read`. Regenerate keys with the new scope name; existing preview keys are migrated by `000041_rename_history_read_to_tracking_read`. SPA scope picker label updated from "History" to "Tracking".
  - **Breaking change for generated clients:** PATCH operation IDs renamed from `patchAsset` / `patchLocation` to `updateAsset` / `updateLocation`. Regenerate clients from the updated spec.
  - **PATCH content-type tightened (RFC 7396 strict):** the two PATCH endpoints (`/api/v1/assets/{asset_id}`, `/api/v1/locations/{location_id}`) now reject `application/json` with 415 `unsupported_media_type`. Only `application/merge-patch+json` is accepted on PATCH; POST and PUT keep `application/json`. The 415 detail string is method-aware and names the correct content type per method. Enforcement is per-route so PATCH probes against POST-only paths (`/tags`, `/rename`) keep returning 405 from chi.
  - **FieldError enum cleanup:** `immutable_field` removed (retired in TRA-674 read-only-strip work); `unknown_field` added so integrators can branch on a wrong-field-name vs wrong-field-value without parsing detail strings.
  - **Internal references stripped from spec descriptions:** four leaks of `TRA-###` / `BB##` references from swag annotations into generated SDK docstrings cleaned up. New Spectral rule `trakrf-no-internal-references-in-descriptions` guards regression.
  - **New Spectral rule** `trakrf-patch-merge-patch-ct-only` asserts every PATCH `requestBody.content` declares only `application/merge-patch+json` so the spec cannot drift back to also declaring `application/json`.
- TRA-660 BB25 C1 public-spec schema namespace restructure (breaking for SDK consumers; no published SDK yet):
  - Schema components no longer carry Go-package prefixes. `asset.PublicAssetView` → `AssetView`, `location.UpdateLocationRequest` → `UpdateLocationRequest`, `errors.ErrorResponse` → `ErrorResponse`, `shared.Tag` → `Tag`, etc. Codegen tools that flatten `.` to a legal identifier (most do) no longer emit doubled-prefix model classes (`AssetPublicAssetView`).
  - The redundant `Public` qualifier is dropped — the spec is the public surface; the Go-side distinction is invisible to SDK consumers.
  - Where the clean name would collide across resources, the rename keeps a resource prefix in verb-noun form: `asset.AddTagResponse` → `AddAssetTagResponse`, `location.AddTagResponse` → `AddLocationTagResponse`.
  - Report-package wrappers fold onto resource-shaped names matching what the operation returns: `report.ListCurrentLocationsResponse` → `AssetLocationsResponse`, `report.PublicCurrentLocationItem` → `AssetLocationItem`.
  - operationIds adopt camelCase `verbResource` form: `assets.create` → `createAsset`, `locations.tags.add` → `addLocationTag`, `reports.asset-locations` → `getAssetLocations`. Generated SDK calls read `client.createAsset()` rather than `client.assets_create()`.
  - Top-level `tags` array now carries descriptions for each resource grouping (assets, locations, orgs, reports) — used by docs renderers.
  - Internal spec is unchanged. Go source is unchanged (rename happens entirely in the apispec transformer). No wire-level behavior changes.
- TRA-602 BB17 S2 schema namespace consolidation (breaking for SDK consumers; no published SDK yet):
  - Asset, location, and report schema components are now under a single (singular) namespace: `asset.*`, `location.*`, `report.*`. Response wrappers that previously lived under `assets.*` / `locations.*` / `reports.*` (e.g. `assets.CreateAssetResponse`, `locations.ListLocationsResponse`, `reports.AssetHistoryResponse`) are renamed to the singular form (`asset.CreateAssetResponse`, `location.ListLocationsResponse`, `report.AssetHistoryResponse`).
  - Org schemas are now under `org.*` (matches the `/api/v1/orgs/...` URL prefix). Public spec: `orgs.GetOrgMeResponse` → `org.GetOrgMeResponse`, `orgs.OrgMeView` → `org.OrgMeView`. Internal spec also folds the model package `organization.*` (full word) onto `org.*` for consistency.
  - Internal-spec audit extension: `users.ListResponse` → `user.ListResponse`; the swag-emission long-form `github_com_trakrf_platform_backend_internal_models_user.User` → `user.User`. `errors.*`, `shared.*`, `apikey.*`, and the remaining single-package families (`auth`, `bulkimport`, `health`, `inventory`, `lookup`, `storage`) are unchanged.
  - SDK regen required for downstream consumers; pre-launch with no published SDKs, the break has no current cost.
- TRA-603 BB17 S1 request body content-type alignment:
  - `POST /api/v1/locations/{location_id}/tags` now declares `application/json` for its request body (previously `*/*`), matching the sibling `POST /api/v1/assets/{asset_id}/tags` endpoint. No wire-level behavior change — the server already required `application/json` — but strict generators (Java/Go) no longer drop the typed body.
- TRA-586 BB16 S7 path-param naming sweep (breaking for SDK consumers; no published SDK yet):
  - Public API path parameters are now consistently qualified across all asset and location operations: `{id}` is renamed to `{asset_id}` on `/api/v1/assets/{asset_id}{,/history,/tags}` and to `{location_id}` on `/api/v1/locations/{location_id}{,/ancestors,/children,/descendants,/tags}`. The actual URL paths are unchanged — only the OpenAPI parameter names.
  - Generated `typescript-fetch` SDK now uses consistent parameter names per resource: `assetsTagsAdd({ assetId, ... })` and `assetsTagsRemove({ assetId, tagId })` — same `assetId` field across both calls. Same on `locationsTagsAdd` / `locationsTagsRemove` (`locationId`).
- TRA-579 BB15 D-4/D-6/D-10 platform-side fixes:
  - `error.title` is now a fixed string per `error.type` (e.g. `validation_error` → "Validation failed", `bad_request` → "Bad request"). Per-call specifics live in `error.detail` and `error.fields[]`. Generated clients should branch on `error.type` and `error.fields[].code`.
  - `GET /api/v1/assets/lookup` and `GET /api/v1/locations/lookup` now reject duplicate `external_key` query parameters with `400 bad_request` (previously: silent first-wins).
  - `GET /api/v1/locations` now accepts `parent_id` (canonical) as a filter, mutually exclusive with `parent_external_key`.
  - Wrong-resource title bug on tags POST conflict ("Failed to create asset" emitted on `/assets/{id}/tags`) is fixed; the conflict still returns 409 with the underlying duplicate-tag detail.
- TRA-580 BB15 spec naming hygiene (S-2/C-1/C-2/C-3, breaking renames):
  - `location.path` is now `tree_path` on the wire (request and response), and `tree_path` replaces `path` in the locations sort enum. The underlying ltree column is unchanged.
  - `asset.current_location_id` and `asset.current_location_external_key` are now `location_id` and `location_external_key` on the wire (request and response), aligning with the report row shape returned by `GET /locations/current`. The underlying SQL column `current_location_id` is unchanged.
  - `POST /api/v1/orgs/{id}/api-keys` response field `data.key` is now `data.token`. Avoids confusion with the human-readable `name` of an API key (and the LLM-leak risk of a "key" field). Endpoint is internal; SPA only.
  - `error.type` is annotated `x-extensible-enum: true` (existing behavior; no client-visible change). The codegen-limitation caveat on `x-extensible-enum` will land in the docs PR.
  - Frontend updated to match the new wire fields. SDK regen required for downstream consumers.
- TRA-578 Public API surface cleanup:
  - `POST/GET/DELETE /api/v1/orgs/{id}/api-keys*` removed from the public OpenAPI spec. Key minting remains browser-mediated by design (see Authentication docs). The endpoints are still implemented and used by the SPA's avatar menu.
  - Renamed scope `scans:read` → `history:read` to align with the `/assets/{id}/history` and `/locations/current` endpoint vocabulary. Existing keys are migrated by `000039_rename_scans_read_scope`. JWTs minted before the migration with a literal `scans:read` claim will return 403 — pre-launch hard cut, no production keys exist.
  - SPA "Scans" row in the new-key form is renamed to "History" to match the new scope name.

## [0.1.0] - 2025-10-11

### Added
- Initial project structure and licensing
- Core documentation for open source project
- .gitignore with Go backend and Node.js frontend support
