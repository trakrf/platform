# TRA-578 — Public-spec cleanup: drop programmatic mint, rename scans:read → history:read

## Goal

Two coupled BB15 cleanups that finish work TRA-568 and TRA-571 closed too narrowly.

1. **O-1** — Remove the `/api/v1/orgs/{id}/api-keys*` endpoints from the public OpenAPI spec so the published surface matches the "browser-mediated by design" framing in `Authentication`.
2. **C-5** — Rename the `scans:read` scope to `history:read` so the scope vocabulary aligns with the actual endpoints (`/locations/current`, `/assets/{id}/history`) rather than a non-existent `/scans` resource.

This is a **pre-launch** cleanup. Per project memory, no production API keys exist; preview keys are disposable. Hard cuts are acceptable where they simplify the change.

## Scope decisions (resolved up front)

### O-1 — flip ALL four api-keys ops to internal, not just POST

The ticket title says "drop programmatic mint" (POST), but the underlying goal is: the published spec must match Authentication's "browser-mediated by design" claim. Leaving `GET`/`DELETE`/`DELETE-by-jti` public while removing `POST` is incoherent — clients have no way to obtain a key with `keys:admin`, so the remaining ops are dead surface.

The four public api-keys operations all become internal:

| Method | Path | Current tag | New tag |
| --- | --- | --- | --- |
| POST | `/api/v1/orgs/{id}/api-keys` | `api-keys,public` | `api-keys,internal` |
| GET | `/api/v1/orgs/{id}/api-keys` | `api-keys,public` | `api-keys,internal` |
| DELETE | `/api/v1/orgs/{id}/api-keys/{key_id}` | `api-keys,public` | `api-keys,internal` |
| DELETE | `/api/v1/orgs/{id}/api-keys/by-jti/{jti}` | `api-keys,public` | `api-keys,internal` |

The endpoints stay implemented and routed — only the public-spec presence changes. The SPA continues to use them (session-auth) and any internal-only api-key tooling continues to work.

### C-5 — hard cut, no alias

The ticket allows "migration or alias". Per memory `prelaunch_no_prod_keys` (expires 2026-05-08), production has no real keys; preview keys are disposable. Hard-cutting:

- Drops `scans:read` from `ValidScopes`, adds `history:read`.
- Updates `RequireScope("scans:read")` → `RequireScope("history:read")` on the two affected routes.
- Updates `@Security APIKey[scans:read]` → `@Security APIKey[history:read]` on the two report handlers.
- Migration rewrites `scans:read` → `history:read` in `api_keys.scopes` for active rows so reading the table reflects the new vocabulary. Does NOT mutate JWTs already in flight (we don't store them).

JWTs minted before the migration with a literal `scans:read` claim will return 403 against the renamed routes. **This is acceptable.** Preview-only keys will be re-minted.

No middleware alias. The "deprecated alias for one release" lane the ticket mentions is the safer-but-stickier path; with no real customers to break, the alias is pure carrying cost.

### UI label rename: "Scans" → "History"

The new-key form in the SPA lists resources as `Assets / Locations / Scans`. After the rename, the row would emit `history:read` — keeping the "Scans" label is a fresh inconsistency. Rename the UI label and the internal `ResourceKey` to `history`.

### Docs repo (trakrf-docs) — separate follow-up PR

Per project feedback `docs_behind_backend`, never publish docs ahead of backend reality. The Authentication doc edits (remove "Programmatic key rotation" + "Required scopes on /api/v1/orgs/{id}/api-keys" sections; rename `scans:read` row; update quickstart) ride in a **separate PR** in `/home/mike/trakrf-docs`, opened after this platform PR merges.

The TRA-578 ticket has both `repo:docs` and `repo:platform` labels. This spec covers both repos; the platform PR lands first, then the docs PR.

## Files touched — platform repo

### Backend code

- `backend/internal/handlers/orgs/api_keys.go` — flip 4 swag annotations: `// @Tags api-keys,public` → `// @Tags api-keys,internal` (lines 37, 146, 214, 261).
- `backend/internal/handlers/reports/asset_history.go` — `// @Security APIKey[scans:read]` → `// @Security APIKey[history:read]` (line 64).
- `backend/internal/handlers/reports/current_locations.go` — same rename (line 56).
- `backend/internal/cmd/serve/router.go` — `RequireScope("scans:read")` → `RequireScope("history:read")` on both `/api/v1/locations/current` and `/api/v1/assets/{id}/history` (lines 168, 169). Update the comment on line 166.
- `backend/internal/models/apikey/apikey.go` — `ValidScopes`: drop `"scans:read"`, add `"history:read"` (line 17).

### Backend tests

- `backend/internal/handlers/orgs/api_keys_integration_test.go` — rename `TestCreateAPIKey_AcceptsScansRead` → `TestCreateAPIKey_AcceptsHistoryRead`, update body to `"scopes":["history:read"]` (lines 459-471).
- Audit any other tests using `scans:read` as a literal — find/replace.

### Migration

- `backend/migrations/000039_rename_scans_read_scope.up.sql`:
  ```sql
  SET search_path=trakrf,public;
  UPDATE api_keys
     SET scopes = array_replace(scopes, 'scans:read', 'history:read')
   WHERE 'scans:read' = ANY(scopes);
  COMMENT ON COLUMN api_keys.scopes IS
      'Subset of ValidScopes in models/apikey: assets:read, assets:write, locations:read, locations:write, history:read, scans:write, keys:admin';
  ```
- `backend/migrations/000039_rename_scans_read_scope.down.sql`:
  ```sql
  SET search_path=trakrf,public;
  UPDATE api_keys
     SET scopes = array_replace(scopes, 'history:read', 'scans:read')
   WHERE 'history:read' = ANY(scopes);
  COMMENT ON COLUMN api_keys.scopes IS
      'Subset of ValidScopes in models/apikey: assets:read, assets:write, locations:read, locations:write, scans:read, scans:write, keys:admin';
  ```

### Frontend

- `frontend/src/types/apiKey.ts` — `Scope` union: `'scans:read'` → `'history:read'`. `ALL_SCOPES`: same swap (lines 6, 42).
- `frontend/src/components/apikeys/ScopeSelector.tsx` — `RESOURCES` entry `{ key: 'scans', label: 'Scans', ... }` → `{ key: 'history', label: 'History', ... }`. Update `ResourceKey` type literal `'scans'` → `'history'` (lines 11, 16).
- `frontend/src/components/apikeys/ScopeSelector.test.tsx` — update assertions:
  - "emits scans:read for 'Read' on Scans" → "emits history:read for 'Read' on History" (line 51).
  - The `scans:write`/`Read+Write` negative test stays — but its label-match `getByLabelText(/scans/i)` becomes `/history/i`. Comment updates.

### Generated artifacts

- `backend/docs/swagger.json`, `backend/docs/swagger.yaml`, `backend/docs/docs.go` — regenerated by `just backend api-spec`. Committed.
- `docs/api/openapi.public.json`, `docs/api/openapi.public.yaml` — regenerated by `just backend api-spec`. Committed.
- `backend/internal/handlers/swaggerspec/openapi.public.{json,yaml}` — gitignored, regenerated.

### Changelog

- `CHANGELOG.md` — entry under unreleased noting:
  - `POST/GET/DELETE /api/v1/orgs/{id}/api-keys*` removed from public spec (browser-mediated mint only).
  - `scans:read` scope renamed to `history:read`. Existing preview keys with `scans:read` must be re-minted.

## Files touched — trakrf-docs repo (separate follow-up PR)

- `docs/api/authentication.md`:
  - Remove `## Programmatic key rotation` section entirely (lines ~130-175).
  - Remove `### Required scopes on /api/v1/orgs/{id}/api-keys` subsection.
  - Remove `keys:admin` row from the scopes table — without public mint, `keys:admin` has no public surface to gate.
  - Rename `scans:read` row → `history:read` in scopes table; update endpoint description.
  - Update `Scans → Read` row in UI-label table → `History → Read` mapping to `history:read`.
  - Drop the `keys:admin is not exposed in the form — admin-tier keys are minted via API` aside (it referenced the now-internal section).
  - Update the two non-obvious-pairing bullets that explain `/locations/current` and `/assets/{id}/history` being gated by `scans:read` — restate with `history:read`.
- `docs/api/private-endpoints.md` — add four `/api/v1/orgs/{id}/api-keys*` rows to the endpoint table marked Internal.
- `docs/api/quickstart.mdx`, `docs/getting-started/api.mdx` — replace `scans:read` mentions with `history:read`; update the `403` example detail string.
- `static/api/openapi.json`, `static/api/openapi.yaml` — refreshed from platform `docs/api/openapi.public.{json,yaml}` after the platform PR merges.

## Verification

### Codegen smoke

After regenerating the public spec:

```bash
just backend api-spec
grep -c '"scans:read"' docs/api/openapi.public.json    # expect 0
grep -c '"history:read"' docs/api/openapi.public.json  # expect ≥ 2 (current_locations, asset_history)
grep -c '/orgs/{id}/api-keys' docs/api/openapi.public.json  # expect 0
```

Acceptance check (per ticket): `openapi-typescript` against the cleaned spec produces no `/orgs/{id}/api-keys` POST and the scope literal type includes `history:read` not `scans:read`.

### Migration on preview

Capture before/after snapshot:

```bash
psql "$PG_URL_PREVIEW" -c "SELECT count(*) FROM trakrf.api_keys WHERE 'scans:read' = ANY(scopes);"
# apply migration via the deploy
psql "$PG_URL_PREVIEW" -c "SELECT count(*) FROM trakrf.api_keys WHERE 'scans:read' = ANY(scopes);"   # 0
psql "$PG_URL_PREVIEW" -c "SELECT count(*) FROM trakrf.api_keys WHERE 'history:read' = ANY(scopes);" # equals before-count
```

### Backend tests

- `just backend test` — unit tests pass (ValidScopes change won't break unrelated suites).
- `just backend test-integration` — integration tests pass; renamed test runs.

### Frontend tests

- `just frontend test` — Vitest passes; ScopeSelector tests assert `history:read`.

### Black-box (preview, post-deploy)

Per memory `blackbox_batched`, API-batch verification — minted via SPA after deploy:

1. Mint a key with `History → Read` only.
2. `curl -H "Authorization: Bearer …" $BASE_URL/api/v1/locations/current` → 200.
3. `curl …/api/v1/assets/{id}/history` → 200.
4. Old preview keys still showing `scans:read` (post-migration: none) would 403 on the same endpoints — this verifies the hard cut.
5. Public spec at `https://app.preview.trakrf.id/api` does NOT show api-keys POST / GET / DELETE; scope enums show `history:read` not `scans:read`.

## Out of scope

- Removing `keys:admin` from `ValidScopes` or revoking existing keys with that scope. The internal mint endpoint still recognizes `keys:admin` for self-rotation flows.
- Adding `/api/v1/scans*` endpoints to make the original name fit. The whole point of C-5 is the inverse.
- Reversing TRA-566 W1 (browser-mediated decision).
- Backwards-compat alias for `scans:read` in the scope checker.
- Reworking the SPA's ScopeSelector beyond the label rename (e.g., adding tooltips, restructuring resources). Anything visual stays as-is.
