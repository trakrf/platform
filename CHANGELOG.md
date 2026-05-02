# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial project documentation structure
- Business Source License 1.1 with Additional Use Grant
- Code of Conduct (Contributor Covenant 2.1)
- Security policy with vulnerability reporting procedures
- Contributing guidelines with code examples and testing requirements
- CLAUDE.md for AI assistant guidance

### Changed
- Migrating handheld React app as frontend component
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
