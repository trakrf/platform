# TRA-579 — BB15 D-4/D-6/D-10 platform-side fixes

Filter conventions and error envelope alignment. Service-side only — docs
work (D-9, C-4, errors-page table, pagination doc) is being handled in a
parallel session in trakrf-docs.

## Scope

Three platform-side findings:

- **D-4** `/lookup` silently picks first when `external_key` is repeated
- **D-6** `error.title` not stable per `error.type`; `bad_request` titles embed
  user input; wrong-resource title bug on tags POST conflict
- **D-10** `parent_id` rejected on `GET /api/v1/locations`

Out of scope (parallel docs session):

- D-6 errors-page authoritative table
- D-9 pagination doc fix (`limit > 200` → `validation_error`)
- C-4 resource-identifiers `is_active` vs `valid_*` precedence note

## Goals

1. `error.title` is a fixed string per `error.type`. Per-call specifics live
   only in `error.detail` and `error.fields[]`.
2. `/api/v1/assets/lookup` and `/api/v1/locations/lookup` reject duplicate
   `external_key` query parameters with `400 bad_request`.
3. `GET /api/v1/locations` accepts `parent_id`, mutually exclusive with
   `parent_external_key`.
4. Wrong-resource bug at `assets.go:635` (`AssetCreateFailed` title used on
   tags POST conflict) is fixed.

## Non-goals

- Restructuring the envelope shape (S-2, separate ticket).
- Adding `is_active_effective` (deferred per ticket).
- Adding `parent_id` mutual-exclusion behaviour to `assets.list`'s
  `location_id` / `location_external_key` (out of ticket scope; flag if
  encountered).
- New error types beyond the existing `ErrorType` enum.

## Design

### D-6 — title pinning

`models/errors.ErrorType` already enumerates the 11 valid `error.type`
values. Add `TitleForType(t ErrorType) string` returning the canonical
title for each. Authoritative table:

| `error.type`              | `error.title`            |
|---------------------------|--------------------------|
| `validation_error`        | Validation failed        |
| `bad_request`             | Bad request              |
| `unauthorized`            | Unauthorized             |
| `forbidden`               | Forbidden                |
| `not_found`               | Not found                |
| `conflict`                | Conflict                 |
| `rate_limited`            | Rate limited             |
| `internal_error`          | Internal server error    |
| `method_not_allowed`      | Method not allowed       |
| `unsupported_media_type`  | Unsupported media type   |
| `missing_org_context`     | Missing org context      |

Drop the `title` parameter from `WriteJSONError` and
`WriteJSONErrorWithFields`. Title is derived from `errType` internally.
Any caller that passed dynamic title text (e.g.
`fmt.Sprintf(AssetGetInvalidID, idParam)`) folds the dynamic context into
`detail`. Where the existing detail was `err.Error()` and the title carried
the human-friendly framing, the detail becomes the human-friendly message
(or a sanitized join). Where the title was a fixed decorative string like
`AssetCreateFailed`, it is dropped — `error.type` + `error.detail` cover
the same information.

Existing helpers (`Respond401`, `Respond404`, `RespondStorageError`,
`RespondValidationError`, `RespondMissingOrgContext`,
`RespondListParamError`, `RespondDecodeError`) already pin title; their
inline `WriteJSONError` calls are updated to drop the title arg.

`apierrors/messages.go` constants used only as titles are removed. Constants
that surface in `detail` (e.g. `LookupNotFound = "No entity found with this
tag"`) stay. Format strings like `AssetGetInvalidID = "Invalid Asset ID:
%s"` remain but their results now flow into `detail` instead of `title`.

The wrong-resource bug at `assets.go:635` is fixed by emitting the conflict
through `RespondStorageError` (or by passing a tag-specific detail) instead
of `AssetCreateFailed`.

### D-4 — duplicate `external_key` rejection

In `assets.Lookup` and `locations.Lookup`, after parsing the query string,
check `len(q["external_key"]) > 1`. If so, return `400 bad_request`,
detail `"exactly one of: external_key"` — same wording as the existing
"missing" and "multiple natural-key" branches so the contract matches what
docs claim today.

The check runs before the existing "exactly one" gate so the duplicate
case is reported cleanly. Empty values still count as "missing" per
existing behavior.

### D-10 — `parent_id` on `GET /api/v1/locations`

Extend `locations.ListLocations`:

- `ListAllowlist.Filters` adds `"parent_id"`.
- After `ParseListParams`, if `parent_id` and `parent_external_key` are
  both provided → `400 validation_error` with field `parent_external_key`,
  code `invalid_value`, message
  `"parent_id and parent_external_key are mutually exclusive"`.
- Parse each `parent_id` value as a positive integer; on failure emit
  `validation_error` with field `parent_id`, code `invalid_value` — same
  pattern `assets.list` uses for `location_id`.
- `location.ListFilter` gains `ParentIDs []int`.
- `buildLocationsWhere` adds `p.id = ANY($N::int[])` when `ParentIDs` is
  non-empty (joined parent table is already in the SELECT).
- Swagger annotation adds `// @Param parent_id query int false ...`.

## Test plan

Per-finding integration tests against the existing suite shape:

- D-4: duplicate `external_key` → 400 `bad_request`, detail wording
  asserted, on both assets and locations lookup endpoints.
- D-6: representative handlers (asset get-by-id with bad id, asset lookup
  with no key, asset tags POST on duplicate, validation failures) all
  produce the canonical title for their `error.type`. The dynamic context
  (the bad id, the missing key) lands in `detail`.
- D-10: list locations with `parent_id=N` returns the children of N; with
  unknown `parent_id` returns empty; with `parent_id` + `parent_external_key`
  returns 400 `validation_error`; with non-integer `parent_id` returns 400
  `validation_error` with field-level diagnostic.

Plus the lint + full test suite (`just validate`) before pushing.

## Migration / rollout

No data migration. No client-visible API surface added except `parent_id`
and the canonical title strings. Client integrators were already told the
title strings would stabilize; the `error.type` is the stable branch key
and is unchanged.

## Risks

- Title-string tests across the suite that assert exact strings will need
  updating; assertions on `error.type` are unaffected.
- A small number of internal handlers may pass title text only in `title`
  with empty `detail`. Each of those needs the message moved into `detail`
  to preserve diagnostic value.
