# TRA-580 — BB15 spec naming and enum hygiene

**Status:** design
**Branch:** `feat/tra-580-bb15-spec-naming-enum-hygiene`
**Linear:** https://linear.app/trakrf/issue/TRA-580
**Blocks-by:** TRA-576 (merged in #261, 2026-05-02) — satisfied
**Pairs with:** trakrf-docs session (S-3 prose + customer-facing docs)

## Summary

Five BB15 findings touching OpenAPI naming and enum semantics. Bundled into one platform PR + one docs PR per the ticket's coordination guidance: a single SDK regeneration cycle and a single changelog item.

This spec is the **platform-side** half. The trakrf-docs session covers S-3 prose and the customer-facing rewrites of any prose that references the renamed fields. Code-level prose comments in this repo (handler doc-comments, struct godoc) stay in scope here.

## Decisions taken without user input

The user is stepping away. Open questions resolved by best judgement, called out so the user can override before merge:

1. **C-3 scope extends to request types.** The ticket acceptance only names `asset.PublicAssetView`. I am also renaming `current_location_id` / `current_location_external_key` on `CreateAssetRequest` and `UpdateAssetRequest`. Reason: a wire schema where you POST `{"current_location_id": 42}` and GET back `{"location_id": 42}` is a worse cross-endpoint trap than the one C-3 set out to fix. We are pre-launch with no production keys (memory: `project_prelaunch_no_prod_keys`), so write-side rename is free.
2. **DB columns stay.** All three of `location.path`, `assets.current_location_id`, `apikey response.key` are wire-only renames. The SQL columns / Go internal field names are out of scope; the ticket explicitly says so for C-1 and is silent on the others. Symmetric treatment keeps the diff narrow and avoids touching migrations.
3. **C-1 / C-3 also rename Go field names** on the public DTO (`PublicAssetView`, `PublicLocationView`) so Go code reads the same as the wire. This is internal-only and prevents the `Path string \`json:"tree_path"\`` cognitive-mismatch trap.
4. **S-2 is a no-op.** `error.type` already has `x-extensible-enum: true` in the committed `docs/api/openapi.public.yaml` (line 214) — the swag annotation in `errors.go` line 56 emits it and the postprocess preserves it. Verified during exploration. Acceptance is met; closing without code change.
5. **S-3 stays out of this PR.** Per user direction, prose (the codegen-limitation caveat) is a docs-repo change. The platform-side changelog entry will reference the docs PR rather than duplicate prose.
6. **Frontend stays in this PR.** The frontend types are hand-rolled (not openapi-typescript-generated), so renaming the wire fields without updating the frontend would break the SPA against its own backend. Update them together.

## In-scope changes (platform repo)

### S-2 — `error.type` extensible enum
- No code change. Verify the spec already carries `x-extensible-enum: true` and call it out in PR description.

### C-1 — `location.path` → `tree_path`
- `backend/internal/models/location/location.go` — rename `Location.Path` Go field → `TreePath`, json tag `path` → `tree_path`.
- `backend/internal/models/location/public.go` — same rename on `PublicLocationView` and the `ToPublicLocationView` projection.
- `backend/internal/handlers/locations/locations.go:357` — update `@Param sort` enum: `path,-path` → `tree_path,-tree_path`.
- `backend/internal/handlers/locations/locations.go:377` — update `Sorts` allowlist `"path"` → `"tree_path"`.
- `backend/internal/storage/locations.go` — sort-column mapping (line ~921 `"l.path ASC"` is already SQL-only and stays; only the API-facing sort key string changes).
- `backend/internal/tools/apispec/postprocess.go:160` — required-fields entry `"path"` → `"tree_path"` for `location.PublicLocationView`.
- Frontend: `frontend/src/types/locations/index.ts` (`path: string` → `tree_path: string`); `frontend/src/components/inventory/LocationBar.tsx:27` (`a.path.localeCompare(b.path)` → `a.tree_path...`).
- Regenerate `docs/api/openapi.public.{json,yaml}` and the embedded `swaggerspec/` copies via `just backend api-spec`.

SQL column stays `path`. ltree behavior unchanged.

### C-2 — apikey response `key` → `token`
- `backend/internal/models/apikey/apikey.go:46` — `APIKeyCreateResponse.Key` → `Token`, json tag `key` → `token`. Update the godoc comment that says "Key is the full JWT".
- `backend/internal/handlers/apikeys/*.go` — any constructor / handler reference to `.Key` updates to `.Token`. Verify search.
- `backend/internal/models/apikey/apikey_wire_test.go` — update the wire test if it asserts on `key`.
- Frontend: `frontend/src/types/apiKey.ts:28` (`key: string` → `token: string`); `frontend/src/components/APIKeysScreen.tsx:176` (`newKey.key` → `newKey.token`); any hooks/components that pass the response on. Verify search.
- Regenerate spec.

### C-3 — asset `current_location_*` → `location_*` (wire only)
- `backend/internal/models/asset/public.go` — `PublicAssetView.CurrentLocationID/ExternalKey` Go fields renamed → `LocationID/LocationExternalKey`, json tags drop `current_` prefix. `ToPublicAssetView` projection updated.
- `backend/internal/models/asset/asset.go` — `CreateAssetRequest`, `UpdateAssetRequest`, `AssetWithLocation` — rename Go fields and json tags the same way (best-judgement extension, see Decisions).
- `backend/internal/handlers/assets/assets.go:40-80` — `resolveCurrentLocation` helper: rename internal references and the `Field:` value in `FieldError`s from `current_location_external_key` → `location_external_key` so 400s match the new schema. Update godoc comments to use new names.
- `backend/internal/handlers/assets/current_location_consistency_integration_test.go` — update assertions that check the JSON key. The DB-insert SQL inside the test (`current_location_id` column) stays; only the wire-shape assertions change. Test file rename is optional; leave it for now.
- `backend/internal/storage/assets.go` — Go-side: any reference to `req.CurrentLocationID` / `req.CurrentLocationExternalKey` updates to `req.LocationID` / `req.LocationExternalKey`. SQL column references (`current_location_id` in INSERT/SELECT) stay unchanged. The `current_location_id_fkey` error-string match also stays.
- `backend/internal/tools/apispec/postprocess.go:130, 157` — `nullableFields` and `requiredFields` for `asset.PublicAssetView` rename `current_location_*` → `location_*`. Same for any other place these field strings appear.
- Frontend: `frontend/src/types/assets/index.ts`, `AssetForm.tsx`, `AssetCard.tsx`, `AssetDetailsModal.tsx`, `lib/asset/filters.ts`, `utils/export/assetExport.ts` (+ tests), `lib/api/lookup/lookup.test.ts`. All `current_location_*` references on the wire-bound type rename to `location_*`. The Zustand `getLocationByIdentifier(asset.location_external_key)` substitution is mechanical.

### Spec regeneration
After the above land, run `just backend api-spec` (delegates to `swag init` + `apispec` postprocessor) and commit:
- `docs/api/openapi.public.{json,yaml}`
- `backend/internal/handlers/swaggerspec/openapi.{public,internal}.{json,yaml}`

Verify the spec lints (`just backend lint-spec` → `redocly lint`) and that `openapi-typescript` would generate the expected types if regenerated. (Skip actually running it — the frontend is hand-rolled.)

### Changelog
One entry under "Unreleased" in `CHANGELOG.md`:
> **TRA-580 / BB15** — spec naming hygiene. Renamed `location.path` → `tree_path`, `asset.current_location_id` → `location_id` and `asset.current_location_external_key` → `location_external_key` (request and response), `apikey-create-response.key` → `token`. Annotated `error.type` as `x-extensible-enum`. Frontend updated to match. SDK regen required for downstream consumers.

## Out of scope (this PR)

- Anything in `trakrf-docs` repo. S-3 prose, customer-facing docs reflecting these renames.
- Renaming the underlying SQL columns or Go internal-model field names beyond the public DTOs.
- `key_id` path parameter on api-keys endpoints.
- S-7 schema-name prefix consistency (post-launch).
- Renaming `external_key` itself.
- Bulk-import CSV header semantics — handled in docs session if needed; the bulkimport service already treats CSV columns separately from JSON struct tags.

## Verification

- `just backend test` — all integration tests pass, including the renamed asset wire-shape assertions and the apikey wire test.
- `just backend lint` — passes.
- `just frontend test` — passes after type and component updates.
- `just frontend typecheck` — passes (will catch any missed `current_location_*` reference).
- `just backend lint-spec` — Redocly clean.
- Manual `psql $PG_URL_PREVIEW -c "\d trakrf.assets"` confirms `current_location_id` column unchanged. (Sanity check: no migration in the diff.)
- Pre-PR: curl preview deployment for one asset GET and verify response shape carries `location_id` after the deploy.

## Risk

- Frontend exhaustively updated against the wire type — TypeScript compilation is the safety net.
- Integration tests assert on the new wire shape, so any handler that still emits `current_location_*` blows up the test suite.
- No DB migration, so no rollback risk.
- Worst case: a hand-rolled CSV header in bulk import or an integration test still references the old name. Caught by typecheck (Go) or test run (Go + TS).

## Coordinates with trakrf-docs session

After this PR merges, trakrf-docs picks up:
- S-3 prose — codegen-limitation caveat for `x-extensible-enum`.
- Any customer-facing doc that names the renamed wire fields.
- Reference to the changelog entry merged here.
