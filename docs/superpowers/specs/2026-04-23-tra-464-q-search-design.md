# TRA-464 — Fix `q` search docs (substring, not fuzzy) and add identifier field search

**Linear:** https://linear.app/trakrf/issue/TRA-464
**Date:** 2026-04-23
**Branch:** `miks2u/tra-464-fix-q-search-docs-substring-not-fuzzy-and-add-identifier`
**Status:** Design approved, ready for plan

## Problem

Two defects on the `q` query parameter exposed by black-box evaluation #5:

1. **Docs lie.** OpenAPI and swagger comments describe `q` as "fuzzy search." Actual behavior is `ILIKE '%term%'` — case-insensitive substring. A developer building a typo-tolerant search UI will ship it broken.
2. **Identifier blindness.** `q=10023` (a valid RFID identifier value) returns 0 results on `/assets` and `/locations`, because the `q` clause only searches `name`, `identifier`, and `description` columns on the parent table — never the `identifiers.value` column. This blocks the TeamCentral "scan a tag, look up the asset" integration.

`/locations/current` already searches `identifiers.value` via an `EXISTS` subquery — so there's a proven in-repo pattern to copy.

## Scope

Three endpoints, two behavior changes, one docs pass:

| Endpoint | Current `q` fields | After |
|---|---|---|
| `GET /assets` | `a.name`, `a.identifier`, `a.description` | + `identifiers.value` where `asset_id = a.id AND is_active = true AND deleted_at IS NULL` |
| `GET /locations` | `l.name`, `l.identifier`, `l.description` | + `identifiers.value` where `location_id = l.id AND is_active = true AND deleted_at IS NULL` |
| `GET /locations/current` | already includes `identifiers.value` with `is_active = true` | add `AND deleted_at IS NULL` for consistency |

**In scope:**
- Storage-layer SQL changes on the three endpoints above.
- Swagger comment rewording on the three handlers.
- Regeneration of `docs/api/openapi.public.yaml`.
- Integration tests for the new identifier-match behavior and the `is_active`/`deleted_at` negative cases.

**Out of scope:**
- Actual fuzzy / Levenshtein / trigram implementation (explicit ticket carve-out).
- Full-text search infra (`pg_trgm`, tsvector, etc.).
- Ranking or relevance scoring.
- Refactor of identifier lifecycle semantics (`is_active` vs `deleted_at` independence). Acknowledged over-engineered but not this ticket's problem.
- Frozen plan docs that mention "fuzzy" (`docs/superpowers/plans/2026-04-19-*`) — historical.

## Storage changes

Mirror the `EXISTS` subquery pattern already live in `backend/internal/storage/reports.go:186-190`. Parameter `$N` is the existing `%term%` argument — no extra bind needed.

### `/assets` — `backend/internal/storage/assets.go:808-813`

Before:
```go
if f.Q != nil {
    args = append(args, "%"+*f.Q+"%")
    idx := len(args)
    clauses.append(fmt.Sprintf(
        "(a.name ILIKE $%d OR a.identifier ILIKE $%d OR a.description ILIKE $%d)",
        idx, idx, idx))
}
```

After:
```go
if f.Q != nil {
    args = append(args, "%"+*f.Q+"%")
    idx := len(args)
    clauses.append(fmt.Sprintf(
        "(a.name ILIKE $%d OR a.identifier ILIKE $%d OR a.description ILIKE $%d "+
        "OR EXISTS (SELECT 1 FROM trakrf.identifiers i "+
        "WHERE i.asset_id = a.id AND i.is_active = true "+
        "AND i.deleted_at IS NULL AND i.value ILIKE $%d))",
        idx, idx, idx, idx))
}
```

### `/locations` — `backend/internal/storage/locations.go:791-796`

Same shape, swapping `a.` → `l.` and `i.asset_id = a.id` → `i.location_id = l.id`.

### `/locations/current` — `backend/internal/storage/reports.go:186-190` (both variants)

Existing subquery already filters `ai.is_active = true`. Add `AND ai.deleted_at IS NULL`. The DISTINCT ON variant near `reports.go:153-157` gets the same addition.

### Why `EXISTS` over `JOIN`

- Avoids row-duplication when an asset has multiple matching identifiers.
- Planner can short-circuit on first match.
- Existing indexes cover this: `idx_identifiers_value`, `idx_identifiers_asset`, `idx_identifiers_location`, `idx_identifiers_active`.

## Docs changes

Swagger comments are the source of truth; `docs/api/openapi.public.yaml` regenerates from them.

| File:Line | Current | New |
|---|---|---|
| `backend/internal/handlers/assets/assets.go:337` | `"fuzzy search on name / identifier / description"` | `"substring search (case-insensitive) on name, identifier, description, and active identifier values"` |
| `backend/internal/handlers/locations/locations.go:325` | `"fuzzy search on name, identifier, description"` | `"substring search (case-insensitive) on name, identifier, description, and active identifier values"` |
| `backend/internal/handlers/reports/current_locations.go:36` | `"fuzzy search on asset name / identifier"` | `"substring search (case-insensitive) on asset name, identifier, and active identifier values"` |
| `docs/api/openapi.public.yaml:405` | top-level `"...fuzzy search"` on assets list | `"...substring search"` |

Regenerate `openapi.public.yaml` via the repo's swag recipe (verify exact command during plan execution — likely `just backend openapi` or equivalent). Commit regenerated yaml alongside the Go edits so CI sees a consistent diff.

## Tests

TDD order: write the failing identifier-match test first, then implement the subquery.

### `backend/internal/storage/assets_integration_test.go`

Extend existing `TestListAssetsFiltered_Q` (line 144) with subtests, or add a sibling `TestListAssetsFiltered_QMatchesIdentifier`:

- Seed asset with a linked active identifier `value = "RFID-10023"`.
- `q=10023` → returns the asset. (New positive case.)
- `q=<value>` against an `is_active = false` identifier → 0 results.
- `q=<value>` against a `deleted_at IS NOT NULL` identifier → 0 results.

### `backend/internal/storage/locations_integration_test.go`

No `q` test exists today. Add `TestListLocationsFiltered_Q` covering:

- Basic positive on `l.name`, `l.identifier`, `l.description` (backfill).
- Positive match on linked active identifier value.
- Negative cases on `is_active = false` and `deleted_at IS NOT NULL`.

### `backend/internal/storage/reports_integration_test.go`

No `q` integration test exists for `/locations/current`. Add `TestCurrentLocations_QMatchesIdentifier` that:

- Matches an active identifier value.
- Rejects a soft-deleted identifier (the new `deleted_at IS NULL` guard).

### Not adding

Handler-level tests for `q`. The existing handler tests already cover param plumbing; this change is purely in the storage layer, so integration tests on the storage functions give real behavior signal.

## Risk & rollout

**Behavior change callout**: a client that previously observed `q=<tag-value>` returning empty will now get matches on `/assets` and `/locations`. This is a fix, not a regression, but worth noting in the PR description and commit.

**Query-plan risk**: `EXISTS` subquery adds cost proportional to matched asset/location rows; indexes on `identifiers(asset_id)`, `identifiers(location_id)`, `identifiers(value)`, `identifiers(is_active)` all exist, and the `/locations/current` endpoint has been running the same shape in prod.

**OpenAPI regen**: if the swag recipe isn't wired into `just`, fall back to invoking `swag` directly in the plan. Will verify during plan execution.

**Rollout**: single PR on `miks2u/tra-464-fix-q-search-docs-substring-not-fuzzy-and-add-identifier`, merge commit (per project convention), preview deploy auto-triggers on push for black-box verification before merge.

## Definition of done

- [ ] `/assets` and `/locations` `q` parameter matches `identifiers.value` for active, non-deleted identifiers.
- [ ] `/locations/current` `q` parameter also filters on `deleted_at IS NULL`.
- [ ] Swagger comments on all three handlers reword "fuzzy" → "substring (case-insensitive)" and list fields searched.
- [ ] `docs/api/openapi.public.yaml` regenerated and committed.
- [ ] Integration tests added for the new positive and negative cases on each endpoint.
- [ ] Existing tests pass unchanged (no behavior regression on `name`/`identifier`/`description` match paths).
