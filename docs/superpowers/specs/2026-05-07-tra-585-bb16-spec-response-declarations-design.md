---
ticket: TRA-585
title: BB16 S1/S2/S3/S4/S5/S6/S10 — spec response declarations and security scheme
date: 2026-05-07
status: design
---

# TRA-585 — BB16 spec response declarations and security scheme

## Goal

Seven BB16 spec-side findings, batched because they share a single OpenAPI annotation pass and one SDK regeneration cycle. Spec-only; no service-behavior changes.

| Finding | Surface | Mechanism |
| --- | --- | --- |
| S1 | `errors.ErrorResponse.description` claims RFC 7807 | postprocess edit |
| S2 | scope arrays on `http-bearer` security (invalid OpenAPI 3.0) | postprocess pass |
| S3 | POST 201 missing `Location` header | swag annotation |
| S4 | POST/PUT/PATCH missing `415` declaration | swag annotation |
| S5 | `GET /api/v1/orgs/me` missing `429` | swag annotation |
| S6 | tag-delete declares `404` but service is idempotent | swag annotation |
| S10 | `q` parameter wording inconsistent across endpoints | swag annotation |

## Source-of-truth strategy

Spec generation pipeline:

```
backend handlers (swag annotations) → swag init → docs/swagger.json
  → apispec convert → openapi3 doc → apispec postprocess → public/internal yaml
```

Two reasonable places to encode each fix:
- **Swag annotation on the handler.** Naturally per-operation. Self-documenting alongside the implementation. Limit: swag's grammar — can't express scope-array stripping.
- **`apispec` postprocess pass** (`backend/internal/tools/apispec/postprocess.go`). One-stop for invasive structural rewrites that swag can't or shouldn't express.

Rule applied:
- If the change is per-operation and maps cleanly to a swag annotation: handler.
- If it's a structural rewrite or applies uniformly across many ops: postprocess.

## Findings — implementation detail

### S1 — `errors.ErrorResponse` description

**Location.** `backend/internal/tools/apispec/postprocess.go`, `annotateErrorEnvelope`.

Replace the existing `Description = "RFC 7807 Problem Details envelope. ..."` with the docs-aligned wording:

> TrakRF error envelope, modeled on RFC 7807 but not 7807-compliant. Fields are nested under `error.*` and content-type is `application/json` (not `application/problem+json`). Generated clients should branch on `error.type` and `error.title`, not `error.detail`. `error.title` is a stable, machine-readable summary that does not vary between calls for the same condition. `error.detail` is the specific, human-readable cause of this particular failure and may be empty when title alone fully describes the condition.

The per-property descriptions on `title` and `detail` already in `annotateErrorEnvelope` stay as-is.

### S2 — scope arrays on `http-bearer`

**Decision: postprocess.** Source remains `@Security APIKey[<scope>]` in handlers (compact, structured, lives next to the implementation). Postprocess strips the scope arrays and injects `**Required scope:** \`<scope>\`` into each operation's `description`.

**New postprocess pass.** Add `stripBearerScopeArrays(doc)` to both `postprocessPublic` and `postprocessInternal`. Behavior, per `op` in `doc.Paths`:

1. For each `securityRequirement` in `op.Security`, for each scheme name where `scheme.Type` (resolved through `doc.Components.SecuritySchemes`) is `http`, `apiKey`, `mutualTLS`, or `openIdConnect`-not-applicable:
   - Capture `scopes := requirement[name]` if non-empty.
   - Set `requirement[name] = []string{}`.
2. If any scopes were captured for the operation, prepend a single line to `op.Description`:
   ```
   **Required scope:** `<comma-joined scopes>`
   ```
   Idempotent: detect the existing prefix and skip re-injection. Multi-scope handlers (none today, but possible) join with `, `.

The pass runs after `rewriteBearerSchemes` (so we look at the post-rewrite scheme type) and before `injectTopLevelSecurity` ordering doesn't matter.

**Coverage.** All 19 handlers using `@Security APIKey[<scope>]` (asset, location, history, scans, keys scopes). Plus the document-level `Security` array if non-empty (currently `injectTopLevelSecurity` writes `[]` so this is a no-op there, but the function should be safe against future changes).

**Test.** Add a unit test in `postprocess_test.go` that:
- Sets up an op with `Security = [{APIKey: ["assets:read"]}]` and `Type = http`.
- Asserts after pass: `Security = [{APIKey: []}]` and op `Description` starts with `**Required scope:** \`assets:read\``.
- Idempotent on second invocation.
- Untouched if scheme type is `oauth2`.

### S3 — `Location` header on POST 201

**Annotation.** Add to `backend/internal/handlers/assets/assets.go` `Create` and `backend/internal/handlers/locations/locations.go` `Create`:

```
// @Header 201 {string} Location "Canonical URL of the created resource"
```

Tag-add POSTs (`/assets/{id}/tags`, `/locations/{id}/tags`) are explicitly **out of scope** per the ticket — they don't currently emit `Location` and that's a service behavior the ticket doesn't mandate changing.

### S4 — declare 415 on body-accepting endpoints

**Annotation.** Add the following line to every `POST`/`PUT`/`PATCH` handler with an `@Accept json` body:

```
// @Failure 415 {object} modelerrors.ErrorResponse "unsupported_media_type"
```

Handler enumeration (verified by grep `@Router.*\[(post|put|patch)\]` minus pure-method ops with no body):

- `assets/assets.go`: `Create`, `Update`, `AddTag`
- `assets/bulkimport.go`: bulk-create
- `auth/auth.go`: `Signup`, `Login`, `ForgotPassword`, `ResetPassword`, `AcceptInvite`
- `inventory/save.go`: `Save`
- `locations/locations.go`: `Create`, `Update`, `AddTag`
- `lookup/lookup.go`: `BatchLookup`
- `orgs/api_keys.go`: `CreateAPIKey`
- `orgs/invitations.go`: `Create`, `Resend`
- `orgs/me.go`: `SetCurrentOrg`
- `orgs/members.go`: `UpdateMember` (PUT)
- `orgs/orgs.go`: `Create`, `Update` (PUT)
- `users/users.go`: `Create`, `Update` (PUT)

Internal-only handlers get the annotation too — cheap, keeps the internal spec accurate.

### S5 — 429 on `GET /api/v1/orgs/me`

**Annotation.** In `backend/internal/handlers/orgs/public.go` `GetOrgMe`:

```
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Header  429 {integer} Retry-After "Seconds to wait before retrying"
```

Existing 401/500 declarations remain.

### S6 — drop 404 on tag-delete; document idempotency

**Annotation.** Two handlers:

- `backend/internal/handlers/assets/assets.go` `RemoveTag`
- `backend/internal/handlers/locations/locations.go` `RemoveTag`

In each:
- Remove `// @Failure 404 {object} modelerrors.ErrorResponse ...` line.
- Append to `@Description`: `Idempotent: returns 204 whether or not the tag was associated. Repeated calls are safe.`

### S10 — `q` parameter wording

Verified via `backend/internal/storage/reports.go:113` (`ILIKE '%' || ... || '%'`): `/locations/current` is genuinely case-insensitive substring. The "fuzzy" wording mentioned in the ticket is stale — current annotations already say substring. Only divergence is `(case-insensitive)` is missing on locations:

**Annotation.** In `backend/internal/handlers/locations/locations.go`, `ListLocations`, change:

```
// @Param q ... "substring search on name, external_key, description, and active tag values"
```

to:

```
// @Param q ... "substring search (case-insensitive) on name, external_key, description, and active tag values"
```

Other two already match.

## Out of scope

- oauth2 `clientCredentials` swap (post-launch; would reverse W1 programmatic-mint policy held in TRA-583).
- Param naming `{id}` → `{asset_id}` etc. (TRA-586).
- `readOnly: true` on schema fields (TRA-587).
- trakrf-docs site updates — done in a separate session; ticket comments capture decisions affecting the docs.

## Verification

After implementation:

1. `just backend api-spec` to regenerate.
2. `just lint` (root) — picks up backend lint + redocly lint of regenerated yaml.
3. `just backend test` — exercises new postprocess unit tests + existing apispec tests.
4. Codegen smoke: `pnpm --package=@openapitools/openapi-generator-cli dlx openapi-generator-cli generate -i docs/api/openapi.public.yaml -g typescript-fetch -o /tmp/tra585-client` — must complete without warnings beyond the existing baseline.
5. `git diff --stat docs/api/ backend/internal/handlers/swaggerspec/` to confirm only expected files churned.

CI workflow `.github/workflows/api-spec.yml` will rerun lint + validation against the PR.

## Acceptance

- [ ] S1 — `errors.ErrorResponse` spec description matches the errors page.
- [ ] S2 — scope arrays empty on every operation; `**Required scope:** \`<scope>\`` lines in operation descriptions where scopes were defined.
- [ ] S3 — 201 responses on POST `/assets` and POST `/locations` declare `Location` header.
- [ ] S4 — 415 declared on every POST/PUT/PATCH operation listed above.
- [ ] S5 — 429 + `Retry-After` on `GET /api/v1/orgs/me`.
- [ ] S6 — tag-delete operations declare 204 only; idempotent semantics in description.
- [ ] S10 — three `q` descriptions all carry the `(case-insensitive)` qualifier.
- [ ] Codegen smoke test passes.
- [ ] Single PR against `main`; CI green.

## Risks & mitigations

- **Postprocess pass touching every operation's `Security`.** Risk of clobbering future per-op overrides. Mitigation: pass only edits scope arrays in-place, never adds/removes requirement entries. Unit-tested.
- **Description prepend on operations with multi-paragraph existing description.** Mitigation: prepend with a trailing blank line so existing markdown still renders correctly.
- **Frontend session JWT (`BearerAuth`) is still in the internal spec with no scopes — must not get a `**Required scope:**` injection.** Mitigation: pass only injects when scopes were captured; sessionless schemes contribute nothing.
