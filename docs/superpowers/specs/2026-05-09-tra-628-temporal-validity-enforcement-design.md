# TRA-628: Temporal-Validity Predicate Enforcement on Public Read Paths

**Linear:** [TRA-628](https://linear.app/trakrf/issue/TRA-628/bb20-c2-document-the-currently-effective-rule-is-active-valid)
**Branch:** `feat/tra-628-temporal-validity-enforcement`
**Date:** 2026-05-09

## Premise reversal

TRA-628 was filed as a docs-only ticket to publish the "currently effective" rule:

> An asset is **currently effective** iff `is_active = true` AND `valid_from ≤ now` AND (`valid_to IS NULL` OR `valid_to > now`).

A platform audit before publishing the rule showed it is **not enforced anywhere on the public read path**. Default list scope only honors `deleted_at IS NULL` and an optional `?is_active=` filter. `valid_from`/`valid_to` are ignored by every list, single-resource get, and reporting endpoint. Publishing the rule as written would mislead ingestion partners.

A wider sweep (assets, locations, tags, reports, api_keys) found additional drift:

- `valid_from`/`valid_to` ignored on assets, locations, and the location join inside `/locations/current` and `/assets/{id}/history`.
- Tag embed query hardcodes `is_active = true` but ignores the temporal columns.
- `api_keys.expires_at` is never checked when listing keys (separate concern, deferred to its own ticket).

This design enforces the temporal-validity predicate on public read paths so the documentation TRA-628 ships next describes truthful behavior.

## Decisions

- **Predicate is purely temporal.** The enforced predicate is `valid_from ≤ now AND (valid_to IS NULL OR valid_to > now)`. `is_active` is **not** part of the predicate; it remains an independent business-state dimension exposed via the existing `?is_active=` filter and the UI's Active/Inactive/All controls.
- **No temporal bypass param in v1.** Default scope is "currently temporally valid." If a future consumer needs historical reconstruction or future-dated reads, an `?as_of=<timestamp>` or `?validity=any` param can be added later — purely additive, non-breaking.
- **Path-`{id}` GET overrides the predicate.** `GET /api/v1/assets/{id}` and `GET /api/v1/locations/{id}` return any non-deleted row regardless of temporal validity. List endpoints, natural-key lookup (`?external_key=`), and reporting endpoints apply the predicate. Rationale: path addressing is a coarse authorization signal — if a caller has the surrogate ID they can fetch the row to see what's wrong with its temporal data. `deleted_at IS NULL` still wins.
- **Tag embed query** is updated to apply the temporal predicate. The original embed query did not filter on `is_active` — that detail in the audit was incorrect — so this is purely additive: `deleted_at IS NULL` + temporal predicate. Embedded tags continue to surface regardless of `is_active`, in alignment with the assets/locations behavior.
- **Q-search tag subqueries** (asset / location / report search-by-tag-value) **retain** the existing `is_active = true` clause and add the temporal predicate alongside it. Retired tags (`is_active=false`) remain excluded from search corpus by deliberate business choice; integration tests `TestListLocationsFiltered_Q` and friends document this behavior.
- **Reports apply the predicate on entity joins.** `/locations/current` filters both the asset side and the location side. `/assets/{id}/history` follows path-`{id}` semantics for the asset existence check (no predicate), but the location join inside each history row applies the predicate (a row with a temporally-invalid location shows NULL/empty location info, matching today's `deleted_at IS NULL` join behavior).
- **Soft-delete (`deleted_at IS NULL`) remains separately enforced everywhere.** It composes with the temporal predicate; neither replaces the other.

## Out of scope

- `?as_of=<timestamp>` or `?validity=` query params (deferred — purely additive when needed).
- `api_keys.expires_at` enforcement (separate ticket; security concern, not bitemporal).
- Backfill of the `2099-12-31` `valid_to` sentinel still present on 20 prod assets (predicate handles it correctly; cleanup is hygiene-only).
- Removing `is_active` from the schema (it's used by the UI; non-goal).
- Refactoring all `WHERE deleted_at IS NULL` clauses into a shared helper (separate cleanup).
- Documentation of the rule in customer-facing docs — that is TRA-628's docs PR (separate, in trakrf-docs repo, follows backend reality once this lands).

## Implementation surface

### New helper

`backend/internal/storage/temporal.go` exposing a single helper that returns a raw SQL fragment keyed on a table alias. Matches the existing storage-layer pattern (string concatenation against `clauses []string`); no parameters are needed since the predicate uses server-side `NOW()`.

```go
// temporallyEffective returns a SQL fragment matching rows that are currently
// effective per the bitemporal validity columns. Composes with deleted_at IS NULL
// and any other filters via AND.
//
// alias is the SQL alias the surrounding query uses for the table being filtered
// (e.g. "a" for assets, "l" for locations, "i" for tags).
func temporallyEffective(alias string) string
```

Pattern returned: `(<alias>.valid_from IS NULL OR <alias>.valid_from <= NOW()) AND (<alias>.valid_to IS NULL OR <alias>.valid_to > NOW())`.

Note the `valid_from IS NULL` branch is defensive — current data has no NULL `valid_from`, but the column is nullable in the schema and treating NULL as "always-was" is the safe default. Without it, a future row with NULL `valid_from` would be silently hidden.

### Storage call sites

| File | Function | Change |
|---|---|---|
| `backend/internal/storage/assets.go` | `buildAssetsWhere` (~line 848) | Append `temporallyEffective("a")` to default clauses. Q-search tag subquery (~line 884) keeps `i.is_active = true` and adds `temporallyEffective("i")` alongside. |
| `backend/internal/storage/assets.go` | `getAssetWithLocationByID` (~line 510) | **No change.** Path-`{id}` override. |
| `backend/internal/storage/locations.go` | `buildLocationsWhere` (~line 890) | Append `temporallyEffective("l")`. Q-search tag subquery keeps `i.is_active = true` and adds `temporallyEffective("i")` alongside. |
| `backend/internal/storage/locations.go` | `GetLocationByID` (~line 99) | **No change.** Path-`{id}` override. |
| `backend/internal/storage/reports.go` | `ListCurrentLocations` (~line 47) | Apply `temporallyEffective("a")` and `temporallyEffective("l")` to the JOIN/WHERE construction (both DistinctOn and TimescaleLast variants). |
| `backend/internal/storage/reports.go` | `ListAssetHistory` / `CountAssetHistory` (~line 207, 268) | Apply `temporallyEffective("l")` on the location join only. Asset existence check follows path-`{id}` override (no predicate on `a`). |
| `backend/internal/storage/tags.go` | Tag embed query (~line 20-21, 53-54) | Replace hardcoded `i.is_active = true` with `temporallyEffective("i")`. |

### Handler changes

None. Default scope changes are entirely below the handler boundary.

### OpenAPI / spec changes

Single addition to the `assets` and `locations` list endpoint descriptions plus the `/locations/current` description: a one-line note that default scope returns currently-effective rows (temporally valid) and that `?is_active=` filters independently. Path-`{id}` GET descriptions get a one-line note that they return any addressable row regardless of temporal validity.

`?include_inactive=`, `?as_of=`, `?validity=` are explicitly **not added** to the spec.

### Schema changes

None.

### Test additions

Integration tests against real Postgres in the existing harness pattern:

- `backend/internal/handlers/assets/temporal_validity_integration_test.go`
- `backend/internal/handlers/locations/temporal_validity_integration_test.go`
- `backend/internal/handlers/reports/temporal_validity_integration_test.go`

Each test seeds rows in five categories and asserts default-scope behavior:

| Category | Setup | Expected |
|---|---|---|
| Currently effective | `valid_from < NOW()`, `valid_to NULL`, `deleted_at NULL` | List 200, includes; GET-by-id 200 |
| Open-ended valid_from NULL | `valid_from NULL`, `valid_to NULL`, `deleted_at NULL` | List 200, includes; GET-by-id 200 |
| Future-dated | `valid_from > NOW()`, `deleted_at NULL` | List 200, **excludes**; GET-by-id **200** (override) |
| Expired | `valid_to < NOW()`, `deleted_at NULL` | List 200, **excludes**; GET-by-id **200** (override) |
| Soft-deleted | `deleted_at = NOW()` | List 200, excludes; GET-by-id 404 |

Plus:

- `?is_active=true` and `?is_active=false` continue to filter independently across all temporal categories.
- `?external_key=` lookup applies the predicate (excludes future/expired rows).
- `/locations/current` excludes assets and locations failing the predicate.

Existing integration tests should continue to pass; only the Warehouse 1 preview row (`id 424420088`, `valid_to 2025-11-17`) becomes hidden from default list views. No production rows change visibility.

## Acceptance

- `effectivePredicate` helper exists and is invoked at every site listed in the surface table.
- Default list scope on `/api/v1/assets`, `/api/v1/locations` excludes future-dated and expired rows; includes them when path-`{id}` is used directly.
- `/locations/current` and `/assets/{id}/history` apply the predicate on entity joins as specified.
- Tag embed query no longer references `is_active`; predicate applied instead.
- New integration tests pass against real Postgres.
- Existing integration tests pass with no other behavior change.
- OpenAPI spec regenerated and committed; new descriptions match the truth.

## Risks

- **Test-data assumptions.** Existing integration tests may seed rows without `valid_from` and rely on them appearing in list output. The defensive `valid_from IS NULL → effective` branch covers this, but if any test sets `valid_to` to a past sentinel (e.g., the `2099-12-31` value TRA-624 stopped emitting) for unrelated reasons, the predicate may silently exclude. Mitigation: full integration suite run; failures will surface immediately.
- **Reports `DistinctOn` plus predicate interaction.** Adding the predicate to a `DISTINCT ON` query may change which row wins per partition if the previously-winning row is now filtered out. Tests cover the case (an asset whose most-recent scan is at an expired location should fall through to the next valid location, or be excluded if no valid location exists).
- **Behavioral break for the one preview anomaly row.** Warehouse 1 (preview only) currently appears in list output; it will disappear after this change. Acceptable — it's the documented bug being fixed.
- **Performance.** The predicate adds two simple indexable comparisons per row. No index changes proposed; existing indexes on `valid_from`/`valid_to` (or absence thereof) match current query shape. Run `EXPLAIN` on the largest tenant before merge if regression is suspected.
