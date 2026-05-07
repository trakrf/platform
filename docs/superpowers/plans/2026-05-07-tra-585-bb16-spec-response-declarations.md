# TRA-585 BB16 Spec Response Declarations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Apply seven BB16 spec-side findings (S1/S2/S3/S4/S5/S6/S10) in a single OpenAPI annotation pass with one regenerated public spec and one PR.

**Architecture:** Per-operation, per-handler changes go in swag `@…` comments on Go handler files. Structural rewrites (errors envelope description, scope-array stripping) go in the `apispec` postprocess pipeline so the change applies uniformly without 19 hand-edits. After all source edits, regenerate the spec with `just backend api-spec`, run unit tests for the new postprocess pass, and run a typescript-fetch codegen smoke test.

**Tech Stack:** Go (swaggo annotations + `apispec` tool using `kin-openapi`), `just`, `pnpm dlx redocly`, `pnpm dlx @openapitools/openapi-generator-cli`.

**Branch:** `feat/tra-585-bb16-spec-response-declarations` (already created, design doc committed).

---

## File Structure

| Path | Purpose | Change |
| --- | --- | --- |
| `backend/internal/tools/apispec/postprocess.go` | apispec postprocess pipeline | edit `annotateErrorEnvelope`; add `stripBearerScopeArrays` |
| `backend/internal/tools/apispec/postprocess_test.go` | apispec postprocess tests | add unit tests for new pass + S1 wording |
| `backend/internal/handlers/orgs/public.go` | `GET /api/v1/orgs/me` | add `@Failure 429` + `@Header 429 Retry-After` |
| `backend/internal/handlers/assets/assets.go` | assets handlers | S3 (Create); S4 (Create, Update, AddTag); S6 (RemoveTag) |
| `backend/internal/handlers/locations/locations.go` | locations handlers | S3 (Create); S4 (Create, Update, AddTag); S6 (RemoveTag); S10 (ListLocations) |
| `backend/internal/handlers/assets/bulkimport.go` | bulk asset import | S4 |
| `backend/internal/handlers/auth/auth.go` | auth POSTs | S4 (Signup, Login, ForgotPassword, ResetPassword, AcceptInvite) |
| `backend/internal/handlers/inventory/save.go` | inventory save | S4 |
| `backend/internal/handlers/lookup/lookup.go` | batch lookup | S4 (BatchLookup) |
| `backend/internal/handlers/orgs/api_keys.go` | org API keys | S4 (CreateAPIKey) |
| `backend/internal/handlers/orgs/invitations.go` | org invitations | S4 (Create, Resend) |
| `backend/internal/handlers/orgs/me.go` | SetCurrentOrg | S4 |
| `backend/internal/handlers/orgs/members.go` | members | S4 (UpdateMember PUT) |
| `backend/internal/handlers/orgs/orgs.go` | org admin | S4 (Create, Update PUT) |
| `backend/internal/handlers/users/users.go` | user admin | S4 (Create, Update PUT) |
| `docs/api/openapi.public.yaml`, `docs/api/openapi.public.json` | regenerated public spec | regen output |
| `backend/internal/handlers/swaggerspec/openapi.public.{yaml,json}`, `…internal.{yaml,json}` | regenerated embedded specs | regen output (via `just backend api-spec`) |

---

## Task 1: Add S1 — error envelope description rewrite

**Files:**
- Modify: `backend/internal/tools/apispec/postprocess.go` (lines 218–220)
- Test: `backend/internal/tools/apispec/postprocess_test.go`

- [ ] **Step 1: Write the failing test**

Append to `postprocess_test.go`:

```go
// TestPostprocess_ErrorEnvelopeDescriptionMatchesDocs locks in TRA-585 S1.
// The errors page declares the envelope is "modeled on RFC 7807 but not
// 7807-compliant" — the spec description must match instead of claiming
// full RFC 7807 compliance.
func TestPostprocess_ErrorEnvelopeDescriptionMatchesDocs(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	// Inject errors.ErrorResponse the way swaggo emits it.
	doc.Components.Schemas["errors.ErrorResponse"] = &openapi3.SchemaRef{
		Value: &openapi3.Schema{
			Type: &openapi3.Types{openapi3.TypeObject},
			Properties: map[string]*openapi3.SchemaRef{
				"error": {Value: &openapi3.Schema{
					Type:       &openapi3.Types{openapi3.TypeObject},
					Properties: map[string]*openapi3.SchemaRef{
						"title":  {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
						"detail": {Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}},
					},
				}},
			},
		},
	}
	require.NoError(t, postprocessPublic(doc))

	desc := doc.Components.Schemas["errors.ErrorResponse"].Value.Description
	assert.Contains(t, desc, "modeled on RFC 7807 but not 7807-compliant",
		"description must match the docs/api/errors page wording (TRA-585 S1)")
	assert.Contains(t, desc, "application/json",
		"description must call out that content-type is application/json, not application/problem+json")
	assert.Contains(t, desc, "nested under `error.*`",
		"description must call out the non-7807 nesting")
	assert.NotContains(t, desc, "RFC 7807 Problem Details envelope.",
		"old wording must be gone — it implies full compliance")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
just backend test ./internal/tools/apispec -run TestPostprocess_ErrorEnvelopeDescriptionMatchesDocs
```

Expected: FAIL — current description still says `RFC 7807 Problem Details envelope.`

- [ ] **Step 3: Edit `annotateErrorEnvelope`**

In `backend/internal/tools/apispec/postprocess.go`, replace the `ref.Value.Description = ...` assignment in `annotateErrorEnvelope` (currently lines ~218–220):

```go
ref.Value.Description = "TrakRF error envelope, modeled on RFC 7807 but not 7807-compliant. " +
    "Fields are nested under `error.*` and content-type is `application/json` (not `application/problem+json`). " +
    "Generated clients should branch on `error.type` and `error.title`, not `error.detail`. " +
    "`error.title` is a stable, machine-readable summary that does not vary between calls for the same condition. " +
    "`error.detail` is the specific, human-readable cause of this particular failure and may be empty when title alone fully describes the condition."
```

The per-property `title` / `detail` description assignments below stay unchanged.

- [ ] **Step 4: Run test to verify it passes**

```bash
just backend test ./internal/tools/apispec -run TestPostprocess_ErrorEnvelopeDescriptionMatchesDocs
```

Expected: PASS.

- [ ] **Step 5: Run all apispec tests to make sure nothing else broke**

```bash
just backend test ./internal/tools/apispec
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/tools/apispec/postprocess.go backend/internal/tools/apispec/postprocess_test.go
git commit -m "feat(apispec): TRA-585 S1 align ErrorResponse description with errors page

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Add S2 — strip scope arrays + inject scope into description

**Files:**
- Modify: `backend/internal/tools/apispec/postprocess.go`
- Test: `backend/internal/tools/apispec/postprocess_test.go`

- [ ] **Step 1: Write the failing test**

Append to `postprocess_test.go`:

```go
// TestPostprocess_StripsBearerScopeArrays_InjectsDescription locks in
// TRA-585 S2. OpenAPI 3.0 §4.8.30 forbids non-empty scope arrays on
// non-oauth2 / non-openIdConnect schemes. Swaggo's
// `@Security APIKey[assets:read]` syntax produces an invalid spec under
// http-bearer. The pass strips the arrays and prepends a
// "**Required scope:** `<scope>`" line to the operation description.
func TestPostprocess_StripsBearerScopeArrays_InjectsDescription(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	// Synthesize an operation with a scoped APIKey requirement and
	// an existing description.
	op := doc.Paths.Find("/assets").Get
	require.NotNil(t, op)
	op.Description = "Paginated list of assets."
	op.Security = openapi3.NewSecurityRequirements().With(
		openapi3.SecurityRequirement{"APIKey": []string{"assets:read"}},
	)

	require.NoError(t, postprocessPublic(doc))

	require.Len(t, *op.Security, 1)
	assert.Equal(t, []string{}, (*op.Security)[0]["APIKey"],
		"scope array must be empty after the pass — non-empty arrays are invalid for http-bearer")

	assert.True(t, strings.HasPrefix(op.Description, "**Required scope:** `assets:read`"),
		"description must start with the scope marker, got %q", op.Description)
	assert.Contains(t, op.Description, "Paginated list of assets.",
		"original description content must be preserved")
}

// TestPostprocess_StripsBearerScopeArrays_Idempotent verifies the pass is
// safe to run twice (the YAML round-trip in CI runs postprocess once but
// future drift checks may re-run).
func TestPostprocess_StripsBearerScopeArrays_Idempotent(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	op := doc.Paths.Find("/assets").Get
	op.Description = "Paginated list of assets."
	op.Security = openapi3.NewSecurityRequirements().With(
		openapi3.SecurityRequirement{"APIKey": []string{"assets:read"}},
	)

	require.NoError(t, postprocessPublic(doc))
	first := op.Description
	require.NoError(t, postprocessPublic(doc))
	assert.Equal(t, first, op.Description,
		"second invocation must not double-prepend the scope marker")
}

// TestPostprocess_StripsBearerScopeArrays_NoOpWithoutScopes verifies an
// op with an already-empty scope array is left untouched (no marker
// injected, description unchanged).
func TestPostprocess_StripsBearerScopeArrays_NoOpWithoutScopes(t *testing.T) {
	withEmptyRequiredFields(t)
	doc := loadAndConvert(t, "testdata/minimal-v2.json")
	op := doc.Paths.Find("/assets").Get
	op.Description = "Paginated list of assets."
	op.Security = openapi3.NewSecurityRequirements().With(
		openapi3.SecurityRequirement{"APIKey": []string{}},
	)

	require.NoError(t, postprocessPublic(doc))
	assert.Equal(t, "Paginated list of assets.", op.Description,
		"no scopes => no marker injected")
}
```

Add `"strings"` to the imports if not already present.

- [ ] **Step 2: Run tests to verify they fail**

```bash
just backend test ./internal/tools/apispec -run TestPostprocess_StripsBearerScopeArrays
```

Expected: FAIL — `stripBearerScopeArrays` does not exist; the existing pipeline preserves scope arrays.

- [ ] **Step 3: Implement `stripBearerScopeArrays`**

Add to `backend/internal/tools/apispec/postprocess.go` (place after `injectTopLevelSecurity`):

```go
// stripBearerScopeArrays strips non-empty scope arrays from operation-level
// SecurityRequirements where the underlying scheme is http or apiKey.
// OpenAPI 3.0 §4.8.30 only permits scope arrays for oauth2 and openIdConnect
// schemes; swaggo's `@Security APIKey[assets:read]` syntax produces invalid
// arrays under http-bearer. To preserve the scope information for human
// readers, the captured scopes are injected into the operation's
// description as a "**Required scope:** `<scope>`" markdown line. The pass
// is idempotent — repeated runs do not double-prepend.
func stripBearerScopeArrays(doc *openapi3.T) {
	if doc.Paths == nil {
		return
	}
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return
	}
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op == nil || op.Security == nil {
				continue
			}
			scopes := stripScopesFromRequirements(op.Security, doc.Components.SecuritySchemes)
			if len(scopes) == 0 {
				continue
			}
			op.Description = injectScopeMarker(op.Description, scopes)
		}
	}
}

// stripScopesFromRequirements walks every SecurityRequirement in the slice,
// finds entries whose scheme is http or apiKey, captures and zeroes their
// scope arrays. Returns the captured scope names in declaration order with
// duplicates removed.
func stripScopesFromRequirements(reqs *openapi3.SecurityRequirements, schemes openapi3.SecuritySchemes) []string {
	if reqs == nil {
		return nil
	}
	var captured []string
	seen := map[string]bool{}
	for _, req := range *reqs {
		for name, arr := range req {
			if !isBearerLikeScheme(schemes, name) {
				continue
			}
			if len(arr) == 0 {
				continue
			}
			for _, s := range arr {
				if !seen[s] {
					seen[s] = true
					captured = append(captured, s)
				}
			}
			req[name] = []string{}
		}
	}
	return captured
}

// isBearerLikeScheme returns true if the named scheme is one of the OpenAPI
// 3.0 types where non-empty scope arrays are invalid (http, apiKey, mutualTLS).
// Schemes the function can't resolve are conservatively treated as
// bearer-like — they should not carry scope arrays in our spec.
func isBearerLikeScheme(schemes openapi3.SecuritySchemes, name string) bool {
	ref := schemes[name]
	if ref == nil || ref.Value == nil {
		return true
	}
	switch ref.Value.Type {
	case "oauth2", "openIdConnect":
		return false
	default:
		return true
	}
}

const scopeMarkerPrefix = "**Required scope:**"

// injectScopeMarker prepends a "**Required scope:** `<scope>`" line to the
// description. Idempotent: returns the original string if the marker is
// already present at the beginning.
func injectScopeMarker(description string, scopes []string) string {
	if strings.HasPrefix(description, scopeMarkerPrefix) {
		return description
	}
	marker := scopeMarkerPrefix + " `" + strings.Join(scopes, ", ") + "`"
	if description == "" {
		return marker
	}
	return marker + "\n\n" + description
}
```

Add `"strings"` to the postprocess.go imports.

Wire the pass into both pipelines. In `postprocessPublic` (around line 30), insert `stripBearerScopeArrays(doc)` after `injectTopLevelSecurity(doc)` and before `stripBearerAuthScheme(doc)`. In `postprocessInternal` (around line 60), insert `stripBearerScopeArrays(doc)` after `normalizeArrayQueryParams(doc)`.

- [ ] **Step 4: Run tests to verify they pass**

```bash
just backend test ./internal/tools/apispec -run TestPostprocess_StripsBearerScopeArrays
```

Expected: PASS for all three tests.

- [ ] **Step 5: Run full apispec suite**

```bash
just backend test ./internal/tools/apispec
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/tools/apispec/postprocess.go backend/internal/tools/apispec/postprocess_test.go
git commit -m "feat(apispec): TRA-585 S2 strip scope arrays from http-bearer ops

Scope arrays on http and apiKey schemes are invalid OpenAPI 3.0; the
pass zeros the arrays and prepends '**Required scope:** \`<scope>\`' to
the operation description so codegen sees a valid spec while humans
keep the scope guidance.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: S3 — `Location` header annotation on POST 201

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (Create handler, around line 95)
- Modify: `backend/internal/handlers/locations/locations.go` (Create handler, around line 87)

- [ ] **Step 1: Edit `assets.go` Create annotation**

Find the block immediately after `// @Success      201  {object}  assets.CreateAssetResponse` and insert:

```
// @Header       201  {string}  Location  "Canonical URL of the created resource"
```

So the resulting block reads:

```
// @Success      201  {object}  assets.CreateAssetResponse
// @Header       201  {string}  Location  "Canonical URL of the created resource"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
```

- [ ] **Step 2: Edit `locations.go` Create annotation**

Find the block immediately after `// @Success      201  {object}  locations.CreateLocationResponse` and insert:

```
// @Header       201  {string}  Location  "Canonical URL of the created resource"
```

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/assets/assets.go backend/internal/handlers/locations/locations.go
git commit -m "feat(api): TRA-585 S3 declare Location header on POST 201

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: S4 — declare 415 on body-accepting endpoints

**Files:** all POST/PUT handlers listed in the design doc.

The annotation to add is identical everywhere:

```
// @Failure 415 {object} modelerrors.ErrorResponse "unsupported_media_type"
```

Insert directly after the existing `// @Failure 400 ...` line (or, for handlers without a 400 declaration, immediately before the first `@Failure` of any kind, keeping numerical ordering: 400 < 401 < 403 < 404 < 409 < 415 < 429 < 500).

- [ ] **Step 1: `assets/assets.go`**

Add 415 declarations on:
- `Create` (insert between 409 and 429)
- `Update` (insert between 409 and 429)
- `AddTag` (insert between 404 and 429 — this op has no 409 today)

- [ ] **Step 2: `assets/bulkimport.go`**

Read the Create/POST annotation block (around lines 23–28) and insert 415 in numerical order with the existing `@Failure` lines. If the block is:

```
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid CSV..."
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 404 {object} modelerrors.ErrorResponse "Job not found or access denied"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
```

Add 415 between the 404 and 500 lines.

- [ ] **Step 3: `auth/auth.go`**

Add 415 to all five POST handlers: `Signup` (line ~50), `Login` (~130), `ForgotPassword` (~165), `ResetPassword` (~200), `AcceptInvite` (~250). For each, insert 415 in numerical order with the existing failures.

- [ ] **Step 4: `inventory/save.go`**

Add 415 to the `Save` handler annotation block (around lines 67–71).

- [ ] **Step 5: `lookup/lookup.go`**

Add 415 to `BatchLookup` (around lines 92–96). The existing block has 400 and 500; insert 415 between them.

- [ ] **Step 6: `locations/locations.go`**

Add 415 to `Create` (around line 87), `Update` (around line 165), and `AddTag` (around line 781). Insert in numerical order.

- [ ] **Step 7: `orgs/api_keys.go`**

Add 415 to `CreateAPIKey` (around line 43).

- [ ] **Step 8: `orgs/invitations.go`**

Add 415 to `Create` (around line 58) and `Resend` (around line 172). For `Resend`, the request body may be empty — check the handler. If `@Accept json` is declared, add 415; otherwise skip.

Verify with: `grep -A 3 '@Router.*invitations.*\[post\]' backend/internal/handlers/orgs/invitations.go` — both should have `@Accept json`.

- [ ] **Step 9: `orgs/me.go`**

Add 415 to `SetCurrentOrg` (around line 67).

- [ ] **Step 10: `orgs/members.go`**

Add 415 to `UpdateMember` (PUT, around line 60).

- [ ] **Step 11: `orgs/orgs.go`**

Add 415 to `Create` (around line 65) and `Update` (PUT, around line 165).

- [ ] **Step 12: `users/users.go`**

Add 415 to `Create` (around line 120) and `Update` (PUT, around line 175).

- [ ] **Step 13: Verify count**

```bash
grep -rn "@Failure 415" backend/internal/handlers/ | wc -l
```

Expected: 16 (Create + Update + AddTag for assets and locations = 6; bulkimport = 1; 5 auth POSTs; inventory save; batch lookup; api_keys create; 2 invitations; SetCurrentOrg; UpdateMember; orgs Create+Update; users Create+Update = 6+1+5+1+1+1+2+1+1+2+2 = 23). Recount actual:

```bash
grep -rn "@Failure 415" backend/internal/handlers/
```

Expected output: 23 distinct `@Failure 415` lines across 14 files. Cross-check that every `@Router .* \[(post|put|patch)\]` operation in `backend/internal/handlers/` (minus paths in swaggerspec/) has a sibling `@Failure 415` within its annotation block:

```bash
grep -B 20 "@Router.*\[\(post\|put\|patch\)\]" backend/internal/handlers/{assets,auth,inventory,locations,lookup,orgs,users}/*.go | grep -c "@Failure 415"
```

Result must equal the number of POST/PUT/PATCH routes (23 today).

- [ ] **Step 14: Commit**

```bash
git add backend/internal/handlers/
git commit -m "feat(api): TRA-585 S4 declare 415 on body-accepting endpoints

Service emits 415 unsupported_media_type when Content-Type is not
application/json; spec must declare it. Adds @Failure 415 to all
POST/PUT/PATCH operations.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: S5 — 429 on `GET /api/v1/orgs/me`

**Files:**
- Modify: `backend/internal/handlers/orgs/public.go` (GetOrgMe annotation, lines 24–34)

- [ ] **Step 1: Insert 429 + Retry-After**

Replace the annotation block:

```go
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security APIKey
// @Router /api/v1/orgs/me [get]
```

with:

```go
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Header  429 {integer} Retry-After "Seconds to wait before retrying"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security APIKey
// @Router /api/v1/orgs/me [get]
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/handlers/orgs/public.go
git commit -m "feat(api): TRA-585 S5 declare 429 on GET /api/v1/orgs/me

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: S6 — drop 404 on tag-delete; document idempotency

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (RemoveTag annotation, around lines 671–687)
- Modify: `backend/internal/handlers/locations/locations.go` (RemoveTag annotation, around lines 838–849)

- [ ] **Step 1: `assets.go` RemoveTag**

In the annotation block:
1. Append to `@Description`:
   ```
   // @Description  Idempotent: returns 204 whether or not the tag was associated. Repeated calls are safe.
   ```
   (This is added on its own `@Description` continuation line — swag concatenates multi-line `@Description` automatically.)
2. Delete the line `// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"`.

- [ ] **Step 2: `locations.go` RemoveTag**

Same change: append the idempotency line to `@Description`, delete the `@Failure 404` declaration.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/assets/assets.go backend/internal/handlers/locations/locations.go
git commit -m "feat(api): TRA-585 S6 idempotent tag-delete; drop 404 from spec

Service returns 204 for both fresh and repeat calls. The 404
declaration was wrong; idempotent semantics are now noted in the
operation description.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: S10 — align `q` parameter wording

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` (ListLocations annotation, line 356)

- [ ] **Step 1: Edit `q` description**

Change:

```go
// @Param q                   query string false "substring search on name, external_key, description, and active tag values"
```

to:

```go
// @Param q                   query string false "substring search (case-insensitive) on name, external_key, description, and active tag values"
```

- [ ] **Step 2: Verify the three descriptions are aligned**

```bash
grep -h '@Param q ' backend/internal/handlers/assets/assets.go backend/internal/handlers/locations/locations.go backend/internal/handlers/reports/current_locations.go
```

Expected output: three lines, each containing `substring search (case-insensitive)`. The fields they search differ (assets/locations include `description`; current_locations uses `asset name`) — that's intentional, since the underlying schemas differ.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/locations/locations.go
git commit -m "feat(api): TRA-585 S10 align q parameter wording across endpoints

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Regenerate spec

**Files:**
- `docs/api/openapi.public.{yaml,json}` (regenerated)
- `backend/internal/handlers/swaggerspec/openapi.{public,internal}.{yaml,json}` (regenerated)

- [ ] **Step 1: Regenerate**

```bash
just backend api-spec
```

Expected: completes without error; "✅ Public spec" and "✅ Internal spec" lines printed.

- [ ] **Step 2: Sanity-check the diff**

```bash
git diff --stat docs/api/ backend/internal/handlers/swaggerspec/
git diff docs/api/openapi.public.yaml | head -200
```

Expected churn:
- Many `description: '**Required scope:** \`<scope>\` ...'` insertions on operations.
- Removal of all `- APIKey: [assets:read]`-style entries; replaced with `- APIKey: []`.
- New `415` response declarations on body-accepting ops.
- New `Location` header on POST `/assets` and POST `/locations` 201s.
- New `429` + `Retry-After` on `/orgs/me`.
- Removal of `404` from tag-delete responses; `Idempotent` text in their descriptions.
- `(case-insensitive)` added to `/locations` `q` description.
- `errors.ErrorResponse` description rewritten to docs-aligned wording.

If anything else changed (parameter renames, schema reshape), STOP and investigate.

- [ ] **Step 3: Commit regenerated spec**

```bash
git add docs/api/openapi.public.json docs/api/openapi.public.yaml backend/internal/handlers/swaggerspec/
git commit -m "chore(api): TRA-585 regenerate spec for BB16 batch

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Lint, test, codegen smoke

- [ ] **Step 1: Backend lint**

```bash
just backend lint
```

Expected: PASS.

- [ ] **Step 2: Backend tests**

```bash
just backend test
```

Expected: PASS — including the three new `stripBearerScopeArrays` tests and the `errorEnvelopeDescription` test.

- [ ] **Step 3: Redocly lint of public spec**

```bash
pnpm --package=@redocly/cli dlx redocly lint docs/api/openapi.public.yaml --extends=recommended
```

Expected: PASS (or no new warnings beyond the pre-existing baseline).

- [ ] **Step 4: OpenAPI generator validation**

```bash
pnpm --package=@openapitools/openapi-generator-cli dlx openapi-generator-cli validate -i docs/api/openapi.public.yaml
```

Expected: `No validation issues detected.` Any new warnings about scope arrays or response declarations indicate a problem.

- [ ] **Step 5: typescript-fetch codegen smoke**

```bash
rm -rf /tmp/tra585-client
pnpm --package=@openapitools/openapi-generator-cli dlx openapi-generator-cli generate \
    -i docs/api/openapi.public.yaml \
    -g typescript-fetch \
    -o /tmp/tra585-client
```

Expected: succeeds without errors. Spot-check `/tmp/tra585-client/apis/AssetsApi.ts` for:
- `createAsset` handler signature exists.
- The generated comment for `createAsset` mentions `Required scope` (since the description carries the marker).

```bash
grep -c "Required scope" /tmp/tra585-client/apis/*.ts
```

Expected: ≥ 19 (one per scoped op, possibly more if the marker is duplicated to multiple files).

---

## Task 10: Push branch, open PR

- [ ] **Step 1: Final status check**

```bash
git status
git log --oneline origin/main..HEAD
```

Expected: working tree clean; commits include the design doc plus per-finding commits and the regen commit.

- [ ] **Step 2: Push**

```bash
git push -u origin feat/tra-585-bb16-spec-response-declarations
```

- [ ] **Step 3: Open PR**

```bash
gh pr create --title "feat(api): TRA-585 BB16 spec response declarations and security scheme" --body "$(cat <<'EOF'
## Summary

BB16 batch: seven spec-side findings (S1/S2/S3/S4/S5/S6/S10) in one OpenAPI annotation pass and one regenerated public spec. Spec-only — no service-behavior changes.

- **S1** ErrorResponse description aligned with docs/api errors page (no longer claims full RFC 7807).
- **S2** Scope arrays stripped from http-bearer security; scope guidance now lives in operation description as \`**Required scope:** \\\`<scope>\\\`\` (apispec postprocess).
- **S3** \`Location\` header declared on POST \`/assets\` and POST \`/locations\` 201s.
- **S4** \`@Failure 415\` declared on every POST/PUT/PATCH operation that accepts a body.
- **S5** \`429\` + \`Retry-After\` declared on \`GET /api/v1/orgs/me\`.
- **S6** \`404\` removed from tag-delete declarations; idempotent 204 semantics noted in op description.
- **S10** \`q\` parameter wording aligned with \`(case-insensitive)\` qualifier across all three endpoints.

Closes TRA-585.

## Test plan

- [ ] \`just backend test\` passes (includes new apispec unit tests for S1 description and S2 scope-stripping).
- [ ] \`just backend lint\` passes.
- [ ] \`pnpm dlx @redocly/cli lint docs/api/openapi.public.yaml\` passes.
- [ ] \`pnpm dlx @openapitools/openapi-generator-cli validate -i docs/api/openapi.public.yaml\` reports no issues.
- [ ] typescript-fetch codegen smoke generates without error and surfaces \`Required scope\` lines in operation comments.
- [ ] Preview deploy renders the spec at \`https://app.preview.trakrf.id/swagger/openapi.public.yaml\`.

## Out of scope

- oauth2 swap (post-launch).
- Param renames (\`{id}\` → \`{asset_id}\`) — TRA-586.
- \`readOnly\` annotations — TRA-587.
- trakrf-docs site updates — separate session per ticket comments.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 4: Capture PR URL** for the wrap-up message.

---

## Task 11: Add docs-affecting comments to TRA-585

Use the `mcp__linear-server__save_comment` tool with `issueId: "TRA-585"` for each.

- [ ] **Comment 1 — S2 implementation note for docs site**

> S2 implemented in apispec postprocess: scope arrays are zeroed and scope info is prepended to each operation's `description` as `**Required scope:** \`<scope>\``. Anything in trakrf-docs that referenced the spec's `security` block to surface scope info needs to look at the operation description instead.

- [ ] **Comment 2 — S1 docs alignment note**

> S1 spec description now matches the wording on docs/api/errors. If the errors page changes again, we need to keep `apispec` postprocess `annotateErrorEnvelope` in sync — they're now coupled by literal string content.

- [ ] **Comment 3 — S6 idempotency note**

> S6 spec change adds idempotency note to the operation description for `DELETE /api/v1/{assets,locations}/{id}/tags/{tag_id}`. Tag-management page in trakrf-docs should surface this — the docs site should call out idempotent behavior so partner integrators don't write retry-with-suppression logic for 404.

- [ ] **Comment 4 — S10 verification note**

> S10 verified `/locations/current` is genuinely substring (`backend/internal/storage/reports.go` uses `ILIKE '%' || q || '%'`). The "fuzzy" wording in the original ticket text was stale — current annotations already say substring.

---

## Self-Review

**Spec coverage:**
- S1 → Task 1 ✓
- S2 → Task 2 ✓
- S3 → Task 3 ✓
- S4 → Task 4 ✓
- S5 → Task 5 ✓
- S6 → Task 6 ✓
- S10 → Task 7 ✓
- regen → Task 8 ✓
- verification → Task 9 ✓
- PR + Linear comments → Tasks 10–11 ✓

**Placeholder scan:** none.

**Type consistency:** `stripBearerScopeArrays`, `stripScopesFromRequirements`, `isBearerLikeScheme`, `injectScopeMarker`, `scopeMarkerPrefix` referenced consistently throughout Task 2. `**Required scope:** \`<scope>\`` literal repeats verbatim across Task 2 implementation, Task 9 codegen check, Task 11 Linear comment.
