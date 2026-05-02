# TRA-573 — Sort parameter enum (W5 spec fix)

**Ticket:** [TRA-573](https://linear.app/trakrf/issue/TRA-573/bb14-docspec-corrections-w3-path-format-w4-put-field-count-w5-sort)
**Scope:** Platform repo only. W3, W4, and the W5 *doc* fix (replacing the broken `?sort=-is_active,external_key` example) are handled in a separate docs-session PR.

## Problem

The `sort` query parameter on every public list endpoint is declared in OpenAPI as a bare `string` with no enum and no collection-format hint. Generated client SDKs cannot validate sort fields at compile time. Live behavior, by contrast, *does* validate — `httputil.ParseListParams` rejects unknown sort fields with a 400 (this is exactly what surfaced W5: a doc example using `is_active`, which is not in any sort allowlist).

Result: the spec under-promises what the server enforces, and integrators have no compile-time guardrail.

## Goal

Align the OpenAPI spec for every public list endpoint with the server's existing sort allowlist by emitting `type: array, items: {type: string, enum: [...]}` for the `sort` parameter. No server behavior change.

## Endpoints in scope

| Endpoint | Server sort allowlist | Existing `@Param sort`? |
|---|---|---|
| `GET /api/v1/assets` | `external_key, name, created_at, updated_at` | yes (bare string) |
| `GET /api/v1/locations` | `path, external_key, name, created_at` | yes (bare string) |
| `GET /api/v1/locations/current` | `last_seen, asset, location` | yes (bare string) |
| `GET /api/v1/assets/{id}/history` | `timestamp` | **missing entirely** |

The fourth row is a latent doc gap discovered while scoping this fix: the endpoint accepts `?sort=timestamp` (the server has it in the allowlist) but never declares a `sort` `@Param`. Including it here means we ship one consistent rule rather than three corrected and one still wrong.

Source-of-truth references:
- `backend/internal/handlers/assets/assets.go:391` — assets sort allowlist
- `backend/internal/handlers/locations/locations.go:369` — locations sort allowlist
- `backend/internal/handlers/reports/current_locations.go:70` — current_locations sort allowlist
- `backend/internal/handlers/reports/asset_history.go:95` — asset_history sort allowlist

## Annotation shape

For each endpoint, replace (or add) the `@Param sort` line with:

```
// @Param sort query []string false "comma-separated; prefix '-' for DESC" collectionFormat(csv) Enums(<field>,-<field>,...)
```

Both ascending and descending forms are enumerated explicitly (`name, -name`). OpenAPI's `enum` is a value-equality constraint, not a regex; without enumerating both forms, descending sorts would fail strict generator validation. The cost is doubling the enum size (≤ 8 entries on the largest endpoint), which is well within readability.

### Per-endpoint Enums()

- **assets:** `Enums(external_key,-external_key,name,-name,created_at,-created_at,updated_at,-updated_at)`
- **locations:** `Enums(path,-path,external_key,-external_key,name,-name,created_at,-created_at)`
- **current_locations:** `Enums(last_seen,-last_seen,asset,-asset,location,-location)`
- **asset_history:** `Enums(timestamp,-timestamp)` — and add the `@Param sort` line, which does not exist today

## Generation pipeline

Source: swag annotations → `swag init` → `docs/swagger.json` (OpenAPI 2.0) → `apispec` tool → split into public/internal OpenAPI 3.0 → `docs/api/openapi.public.{json,yaml}` (committed) and embedded copies in `swaggerspec/`.

The conversion uses `openapi2conv.ToV3` from `github.com/getkin/kin-openapi`, which translates Swagger 2.0 `collectionFormat: csv` on an array param into OpenAPI 3 `style: form, explode: false`. No changes are required to the `apispec` tool.

Expected post-regeneration shape for each `sort` parameter (OpenAPI 3 yaml):

```yaml
- description: comma-separated; prefix '-' for DESC
  in: query
  name: sort
  schema:
    type: array
    items:
      type: string
      enum:
        - external_key
        - -external_key
        - name
        - -name
        # ...
  style: form
  explode: false
```

## Design decisions

**Why array-of-enum instead of a bare-string enum?** A bare string with `Enums(...)` would emit `enum:` on the joined value. `?sort=name,external_key` would fail strict validation because the literal `"name,external_key"` is not in the enum set. That produces a spec that lies about what the server accepts — strictly worse than no enum.

**Why include the descending forms in `Enums()` rather than use `pattern`?** OpenAPI's `pattern` is supported unevenly across generators; `enum` is universally honored. Enumerating both forms costs ≤ 8 entries on the largest endpoint. Pattern stays available as an escape hatch if a future endpoint has a sortable-field count where enumerating both forms is unwieldy.

**Why include `path` in the locations sort enum?** The server allowlist accepts it. W3's guidance is that `path` is informational and should never be *parsed for identity* — sorting is unrelated. Removing `path` from the sort allowlist would be a server behavior change, which the ticket explicitly excludes.

**Why include `-timestamp` for asset_history when ascending sort is nonsensical?** The server accepts it. Spec consistency with the other three endpoints (always-pair `field, -field`) is more valuable than this one micro-optimization, and removing ascending support would be a server behavior change.

**Why establish the `Enums()` pattern here?** No prior `@Param` in the codebase uses `Enums()` or `collectionFormat()` (verified by grep). This PR is the precedent. No other parameters need conversion at this time.

## Acceptance

- [ ] Four `@Param sort` annotations updated/added per the table above.
- [ ] `just backend api-spec` regenerates the spec cleanly.
- [ ] Each of the four `sort:` blocks in `docs/api/openapi.public.yaml` shows `type: array`, `items.enum: [...]`, `style: form`, `explode: false`.
- [ ] `just backend api-lint` passes (Redocly recommended ruleset).
- [ ] `just backend test` passes — no behavior change, but confirms annotations parse and the embedded specs are valid.
- [ ] PR description references TRA-573 and explicitly notes that the doc-side fixes (W3, W4, W5 example) ship in a separate trakrf-docs PR.

## Out of scope

- W3 — resource-identifiers doc rewrite (`path` legacy-format acknowledgment).
- W4 — PUT "strip read-only fields" generic-rule rewrite.
- W5 doc — replacing the broken `?sort=-is_active,external_key` pagination example with a valid one.
- Removing `path` from the locations sort allowlist (server behavior change; not covered by W5).
- Removing ascending sort from asset_history (server behavior change; not covered by W5).
- Tightening any other parameter to `Enums()`.
