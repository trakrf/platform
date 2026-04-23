# TRA-454 API Contract Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix three API contract inconsistencies surfaced in black-box evaluation #4: case-strict `Bearer` scheme, unwrapped api-keys create response, and raw validator/decoder error leaks on the api-keys handler.

**Architecture:** Three independent backend fixes plus one frontend adaptation. C3 touches three auth middlewares with the same pattern. C4 wraps one handler response and updates its frontend consumer. C5 adopts the existing `httputil.RespondValidationError` helper and registers a JSON tag-name function on the orgs-package validator. Each change is tested at the handler / middleware level via Go integration tests, plus a vitest update for the frontend fixture.

**Tech Stack:** Go 1.22 (chi, go-playground/validator/v10, stretchr/testify), TypeScript / React (axios, vitest), `just` as task runner.

**Spec:** `docs/superpowers/specs/2026-04-23-tra-454-api-contract-polish-platform-design.md`

**Branch:** `miks2u/tra-454-api-contract-polish` (already created, spec already committed).

---

## File Structure

**Backend — modified:**
- `backend/internal/middleware/middleware.go` — line 149, `Auth` scheme check
- `backend/internal/middleware/apikey.go` — line 43, `APIKeyAuth` scheme check
- `backend/internal/middleware/either_auth.go` — line 37, `EitherAuth` scheme check
- `backend/internal/middleware/middleware_test.go` — add casing tests for `Auth` and `APIKeyAuth`
- `backend/internal/middleware/either_auth_test.go` — add casing test for `EitherAuth`
- `backend/internal/handlers/orgs/orgs.go` — line 19, validator init
- `backend/internal/handlers/orgs/api_keys.go` — lines 26, 52-60, 107 (decoder, validator, swagger, response)
- `backend/internal/handlers/orgs/api_keys_integration_test.go` — update wrapped-shape assertions, add empty-body and validation tests

**Frontend — modified:**
- `frontend/src/lib/api/apiKeys.ts` — line 14-17, unwrap in module
- `frontend/src/lib/api/apiKeys.test.ts` — wrap mocked fixture

**No new files are created.** All tests land in existing files co-located with the code under test.

---

## Preflight

- [ ] **Step 0.1: Confirm branch and clean tree**

Run: `git status && git rev-parse --abbrev-ref HEAD`
Expected: branch `miks2u/tra-454-api-contract-polish`, working tree clean (the spec commit `ae3c51f` is the tip).

---

## Task 1: C3a — Case-insensitive Bearer in `Auth` (session JWT)

**Files:**
- Modify: `backend/internal/middleware/middleware.go:149`
- Modify: `backend/internal/middleware/middleware_test.go` (add after existing `TestAuth_InvalidToken_Respond401`)

**Rationale:** Session-JWT middleware currently rejects `bearer <jwt>` with 401 and the canonical "Authorization header must be Bearer <token>" detail. RFC 6750 §2.1 / RFC 7235 §2.1 make the scheme name case-insensitive.

- [ ] **Step 1.1: Add failing table-driven casing test**

Append this function to `backend/internal/middleware/middleware_test.go` after the existing `TestAuth_InvalidToken_Respond401`:

```go
func TestAuth_BearerSchemeCaseInsensitive(t *testing.T) {
	cases := []string{"Bearer", "bearer", "BEARER", "BeArEr"}
	for _, scheme := range cases {
		t.Run(scheme, func(t *testing.T) {
			h := Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("should not reach handler for invalid token")
			}))
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/x", nil)
			r.Header.Set("Authorization", scheme+" not-a-valid-jwt")
			h.ServeHTTP(w, r)

			if w.Code != 401 {
				t.Fatalf("status = %d, want 401", w.Code)
			}
			var resp apierrors.ErrorResponse
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			// Must reach the token-validation branch, not the scheme-rejection branch.
			if resp.Error.Detail != "Bearer token is invalid or expired" {
				t.Errorf("detail = %q, want token-validation detail (scheme should have been accepted)", resp.Error.Detail)
			}
		})
	}
}
```

- [ ] **Step 1.2: Run the test and confirm it fails**

Run: `just backend test -run TestAuth_BearerSchemeCaseInsensitive ./internal/middleware/...`
Expected: three of the four subtests (`bearer`, `BEARER`, `BeArEr`) fail with `detail = "Authorization header must be Bearer <token>"`, the `Bearer` subtest passes.

- [ ] **Step 1.3: Apply the middleware fix**

Edit `backend/internal/middleware/middleware.go:149`:

```go
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
```

Leave `strings.Split` alone; only the comparison changes.

- [ ] **Step 1.4: Run the casing test and confirm it passes**

Run: `just backend test -run TestAuth_BearerSchemeCaseInsensitive ./internal/middleware/...`
Expected: all four subtests PASS.

- [ ] **Step 1.5: Run the full middleware unit suite to catch regressions**

Run: `just backend test ./internal/middleware/...`
Expected: all existing tests still PASS. In particular, `TestAuth_MalformedHeader_Respond401` (which uses `"Basic abc123"`) must still return "Authorization header must be Bearer <token>".

- [ ] **Step 1.6: Commit**

```bash
git add backend/internal/middleware/middleware.go backend/internal/middleware/middleware_test.go
git commit -m "fix(tra-454): case-insensitive Bearer scheme in session auth (C3)"
```

---

## Task 2: C3b — Case-insensitive Bearer in `APIKeyAuth`

**Files:**
- Modify: `backend/internal/middleware/apikey.go:43`
- Modify: `backend/internal/middleware/middleware_test.go` (add after existing `TestAPIKey_InvalidJWT_Respond401`)

**Rationale:** `APIKeyAuth` has the same case-strict check and must reject unknown schemes (`Basic`, `Token`) while accepting any casing of `Bearer`.

- [ ] **Step 2.1: Add failing casing test (unit — no DB needed)**

Append this function to `backend/internal/middleware/middleware_test.go` after the existing `TestAPIKey_InvalidJWT_Respond401`:

```go
func TestAPIKey_BearerSchemeCaseInsensitive(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	cases := []string{"Bearer", "bearer", "BEARER", "BeArEr"}
	for _, scheme := range cases {
		t.Run(scheme, func(t *testing.T) {
			h := APIKeyAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("should not reach handler for invalid token")
			}))
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/x", nil)
			r.Header.Set("Authorization", scheme+" not-a-valid-jwt")
			h.ServeHTTP(w, r)

			if w.Code != 401 {
				t.Fatalf("status = %d, want 401", w.Code)
			}
			var resp apierrors.ErrorResponse
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			// The response for invalid-JWT must be reached — scheme must have been accepted.
			if resp.Error.Detail == "Authorization header must be Bearer <token>" {
				t.Errorf("scheme %q was rejected as malformed; want token-validation branch reached", scheme)
			}
		})
	}
}
```

- [ ] **Step 2.2: Run the test and confirm it fails**

Run: `just backend test -run TestAPIKey_BearerSchemeCaseInsensitive ./internal/middleware/...`
Expected: three subtests fail (lowercase/upper/mixed) with the malformed-header detail; `Bearer` passes.

- [ ] **Step 2.3: Apply the middleware fix**

Edit `backend/internal/middleware/apikey.go:43`:

```go
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
```

- [ ] **Step 2.4: Run the casing test and confirm it passes**

Run: `just backend test -run TestAPIKey_BearerSchemeCaseInsensitive ./internal/middleware/...`
Expected: all four subtests PASS.

- [ ] **Step 2.5: Run the full middleware unit suite**

Run: `just backend test ./internal/middleware/...`
Expected: all tests PASS. `TestAPIKey_MalformedHeader_Respond401` (uses `"Basic abc123"`) still returns the malformed detail.

- [ ] **Step 2.6: Commit**

```bash
git add backend/internal/middleware/apikey.go backend/internal/middleware/middleware_test.go
git commit -m "fix(tra-454): case-insensitive Bearer scheme in api-key auth (C3)"
```

---

## Task 3: C3c — Case-insensitive Bearer in `EitherAuth`

**Files:**
- Modify: `backend/internal/middleware/either_auth.go:37`
- Modify: `backend/internal/middleware/either_auth_test.go` (add alongside existing casing tests)

**Rationale:** `EitherAuth` multiplexes session + api-key auth. The scheme check happens before token-type detection, so the casing bug affects both paths. The existing test file is integration-tagged (`//go:build integration`) because the api-key path hits storage; we add the casing test in the same file for consistency.

- [ ] **Step 3.1: Add failing casing test**

Append this function to `backend/internal/middleware/either_auth_test.go`:

```go
func TestEitherAuth_BearerSchemeCaseInsensitive(t *testing.T) {
	store, cleanup, _, _, _, sessTok := setupEitherAuth(t)
	defer cleanup()

	cases := []string{"Bearer", "bearer", "BEARER", "BeArEr"}
	for _, scheme := range cases {
		t.Run(scheme, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("Authorization", scheme+" "+sessTok)
			w := httptest.NewRecorder()
			middleware.EitherAuth(store)(http.HandlerFunc(echoPrincipalHandler)).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, "session", w.Header().Get("X-Principal"))
		})
	}
}
```

- [ ] **Step 3.2: Run the test and confirm it fails**

Run: `just backend test-integration -run TestEitherAuth_BearerSchemeCaseInsensitive ./internal/middleware/...`

Note: `just backend test-integration` (or the project's equivalent recipe that includes `-tags=integration`) must be used because the file has the integration build tag. If the recipe is named differently, find it via `just --list backend`.

Expected: three subtests (`bearer`, `BEARER`, `BeArEr`) fail with 401 instead of 200; `Bearer` passes.

- [ ] **Step 3.3: Apply the middleware fix**

Edit `backend/internal/middleware/either_auth.go:37`:

```go
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
```

- [ ] **Step 3.4: Run the casing test and confirm it passes**

Run: `just backend test-integration -run TestEitherAuth_BearerSchemeCaseInsensitive ./internal/middleware/...`
Expected: all four subtests PASS.

- [ ] **Step 3.5: Run the full middleware integration suite**

Run: `just backend test-integration ./internal/middleware/...`
Expected: all tests PASS.

- [ ] **Step 3.6: Commit**

```bash
git add backend/internal/middleware/either_auth.go backend/internal/middleware/either_auth_test.go
git commit -m "fix(tra-454): case-insensitive Bearer scheme in either auth (C3)"
```

---

## Task 4: C5a — Register `JSONTagNameFunc` on the orgs-package validator

**Files:**
- Modify: `backend/internal/handlers/orgs/orgs.go:19`

**Rationale:** The orgs-package validator currently reports Go struct field names. Once `CreateAPIKey` switches to `RespondValidationError` (Task 5), reported field names must be the JSON names (`name`, `scopes`), matching the public docs. This is a one-line change that sets up Task 5; its behavior is verified as part of Task 6.

- [ ] **Step 4.1: Make the change**

Edit `backend/internal/handlers/orgs/orgs.go:19`:

From:
```go
var validate = validator.New()
```

To:
```go
var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	return v
}()
```

Make sure `httputil` is imported in that file. Check by searching for the `"github.com/trakrf/platform/backend/internal/util/httputil"` line in the imports block; if absent, add it. (The sibling file `api_keys.go` already imports it, so the module is in the dependency graph.)

- [ ] **Step 4.2: Compile and run the orgs handler unit tests (smoke)**

Run: `just backend test ./internal/handlers/orgs/...`
Expected: compile succeeds. Any pure-unit tests (non-integration) still PASS. No behavior change expected yet — no handler in the package currently uses `RespondValidationError`.

- [ ] **Step 4.3: Commit**

```bash
git add backend/internal/handlers/orgs/orgs.go
git commit -m "refactor(tra-454): register JSON tag-name fn on orgs validator (C5)"
```

---

## Task 5: C5b + C5c — Use shared translator + strip decoder error in `CreateAPIKey`

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys.go:52-60`

**Rationale:** Two back-to-back changes in the same handler block: replace the decoder's raw `err.Error()` with a generic detail, and route validator failures through `httputil.RespondValidationError`. Verification tests land in Task 6.

- [ ] **Step 5.1: Replace the decoder error detail**

Edit `backend/internal/handlers/orgs/api_keys.go:52-55`:

From:
```go
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid JSON", err.Error(), reqID)
		return
	}
```

To:
```go
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid JSON body", "", reqID)
		return
	}
```

- [ ] **Step 5.2: Route validator errors through `RespondValidationError`**

Edit `backend/internal/handlers/orgs/api_keys.go:57-60`:

From:
```go
	if err := validate.Struct(req); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			"Validation failed", err.Error(), reqID)
		return
	}
```

To:
```go
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
```

- [ ] **Step 5.3: Compile check**

Run: `just backend build` (or `just backend check`, whichever compiles the module).
Expected: the package compiles; `modelerrors` is still imported by other branches (conflict / internal / unauthorized) so no import cleanup needed.

- [ ] **Step 5.4: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys.go
git commit -m "fix(tra-454): clean decoder + validator errors on CreateAPIKey (C5)"
```

---

## Task 6: C5 tests — Empty body + validation envelope assertions

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys_integration_test.go`

**Rationale:** Verifies 5a/5b/5c end-to-end: empty body no longer leaks `"EOF"`, validation errors use the field envelope, and field names come from JSON tags.

- [ ] **Step 6.1: Add two new integration tests**

Append the following to `backend/internal/handlers/orgs/api_keys_integration_test.go`, after `TestCreateAPIKey_NonAdminForbidden`:

```go
func TestCreateAPIKey_EmptyBody_CleanMessage(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-crud")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, sessionToken := seedAdminUser(t, pool, orgID)

	r := newAdminRouter(t, store)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID), bytes.NewReader(nil))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	body := w.Body.String()
	assert.NotContains(t, body, "EOF", "raw decoder error must not leak to clients")
	assert.NotContains(t, body, "unexpected end", "raw decoder error must not leak to clients")

	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Title   string `json:"title"`
			Message string `json:"message"`
			Detail  string `json:"detail"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "bad_request", resp.Error.Code)
	assert.Equal(t, "Invalid JSON body", resp.Error.Message)
	assert.Empty(t, resp.Error.Detail, "detail must not carry runtime error text")
}

func TestCreateAPIKey_ValidationFailed_JSONFieldNames(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-crud")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, sessionToken := seedAdminUser(t, pool, orgID)

	r := newAdminRouter(t, store)

	// Valid JSON, missing required `name` and `scopes` fields.
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID),
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	body := w.Body.String()
	assert.NotContains(t, body, "CreateAPIKeyRequest", "raw validator output must not leak Go struct name")
	assert.NotContains(t, body, "'Name'", "field names must be JSON names, not Go struct names")
	assert.NotContains(t, body, "'Scopes'", "field names must be JSON names, not Go struct names")

	var resp struct {
		Error struct {
			Code   string `json:"code"`
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_failed", resp.Error.Code)

	fieldNames := make([]string, 0, len(resp.Error.Fields))
	for _, f := range resp.Error.Fields {
		fieldNames = append(fieldNames, f.Field)
	}
	assert.Contains(t, fieldNames, "name")
	assert.Contains(t, fieldNames, "scopes")
}
```

- [ ] **Step 6.2: Run the new tests**

Run: `just backend test-integration -run "TestCreateAPIKey_EmptyBody_CleanMessage|TestCreateAPIKey_ValidationFailed_JSONFieldNames" ./internal/handlers/orgs/...`
Expected: both tests PASS (Tasks 4+5 already landed).

If `TestCreateAPIKey_ValidationFailed_JSONFieldNames` fails because the JSON envelope uses a different top-level key for field errors (e.g. `error.errors` instead of `error.fields`), open `backend/internal/util/httputil/validation.go` (line 152-153) and `backend/internal/models/errors/` to confirm the actual key, then correct the struct tag in the test. The existing `assets` tests that use `RespondValidationError` are the ground truth — grep them for the pattern (`grep -rn "json:\"fields\"\|json:\"errors\"" backend/internal/handlers/assets/ backend/internal/models/errors/`) before editing.

- [ ] **Step 6.3: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys_integration_test.go
git commit -m "test(tra-454): cover clean decoder + validator messages on CreateAPIKey (C5)"
```

---

## Task 7: C4a — Wrap the CreateAPIKey response in `{"data": ...}`

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys.go:26` (swagger annotation)
- Modify: `backend/internal/handlers/orgs/api_keys.go:107` (response write)
- Modify: `backend/internal/handlers/orgs/api_keys_integration_test.go:84-88` (existing success assertion)

**Rationale:** Single-resource responses are `{"data": ...}`. The ListAPIKeys handler in the same file already follows this shape; CreateAPIKey must match. The existing `TestCreateAPIKey_Admin` test currently unmarshals the response directly into `APIKeyCreateResponse` and will break once the wrapping lands — we update it in the same commit so the build stays green.

- [ ] **Step 7.1: Update the existing success test to expect wrapping**

Edit `backend/internal/handlers/orgs/api_keys_integration_test.go:83-93` (the assertion block inside `TestCreateAPIKey_Admin`):

From:
```go
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp apikey.APIKeyCreateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Key)
	assert.Equal(t, "TeamCentral sync", resp.Name)
	assert.Equal(t, []string{"assets:read", "locations:read"}, resp.Scopes)

	// Key must validate as an api-key JWT
	claims, err := jwt.ValidateAPIKey(resp.Key)
	require.NoError(t, err)
	assert.Equal(t, orgID, claims.OrgID)
```

To:
```go
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var envelope struct {
		Data apikey.APIKeyCreateResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	// Response must be wrapped in {"data": {...}}; a top-level "key" field must NOT exist.
	var flat map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &flat))
	assert.NotContains(t, flat, "key", "response must be wrapped in data envelope")
	resp := envelope.Data
	assert.NotEmpty(t, resp.Key)
	assert.Equal(t, "TeamCentral sync", resp.Name)
	assert.Equal(t, []string{"assets:read", "locations:read"}, resp.Scopes)

	// Key must validate as an api-key JWT
	claims, err := jwt.ValidateAPIKey(resp.Key)
	require.NoError(t, err)
	assert.Equal(t, orgID, claims.OrgID)
```

- [ ] **Step 7.2: Run the test and confirm it fails**

Run: `just backend test-integration -run TestCreateAPIKey_Admin ./internal/handlers/orgs/...`
Expected: test FAILS. The assertion that `"key"` is absent at the top level fails because the current response is flat.

- [ ] **Step 7.3: Wrap the handler response**

Edit `backend/internal/handlers/orgs/api_keys.go:107`:

From:
```go
	httputil.WriteJSON(w, http.StatusCreated, resp)
```

To:
```go
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": resp})
```

- [ ] **Step 7.4: Update the swagger annotation**

Edit `backend/internal/handlers/orgs/api_keys.go:26`:

From:
```go
// @Success 201 {object} apikey.APIKeyCreateResponse
```

To:
```go
// @Success 201 {object} map[string]any "data: apikey.APIKeyCreateResponse"
```

Matches the style at line 116 (`// @Success 200 {object} map[string]any "data: []apikey.APIKeyListItem"`).

- [ ] **Step 7.5: Re-run the test and confirm it passes**

Run: `just backend test-integration -run TestCreateAPIKey_Admin ./internal/handlers/orgs/...`
Expected: PASS.

- [ ] **Step 7.6: Regenerate swagger output (if the repo commits generated docs)**

Run: `just backend swagger` (or whichever recipe regenerates swagger; check `just --list backend` for a recipe name like `swagger`, `docs`, or `openapi`).
Expected: `backend/docs/` or `backend/internal/docs/` updates reflecting the new `{object} map[string]any` for the POST. If no such recipe exists or generated output is not committed, skip this step — the annotation lives in source and gets generated by CI.

- [ ] **Step 7.7: Run the full orgs handler integration suite**

Run: `just backend test-integration ./internal/handlers/orgs/...`
Expected: all tests PASS. `TestCreateAPIKey_NonAdminForbidden` (403 path), `TestListAPIKeys_ExcludesRevoked`, and the new Task-6 tests still green.

- [ ] **Step 7.8: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys.go backend/internal/handlers/orgs/api_keys_integration_test.go
# plus any regenerated swagger files from step 7.6
git commit -m "fix(tra-454): wrap CreateAPIKey response in data envelope (C4)"
```

---

## Task 8: C4b — Frontend unwrap + fixture update

**Files:**
- Modify: `frontend/src/lib/api/apiKeys.ts:14-17`
- Modify: `frontend/src/lib/api/apiKeys.test.ts:24-36`

**Rationale:** Keep the module's public type unchanged (`Promise<APIKeyCreateResponse>`) so no downstream hook or component changes. Unwrap inside the module and update the test fixture accordingly.

- [ ] **Step 8.1: Read the existing test to identify the exact mock**

Run: `sed -n '1,60p' frontend/src/lib/api/apiKeys.test.ts`

Note the exact object that mocks the `POST /orgs/{id}/api-keys` response. It looks like a flat `APIKeyCreateResponse`-shaped object passed as the axios mock's `data`. Identify:
- the mock value (line numbers and the object literal),
- whether it is passed as `{data: <literal>}` (axios response) or just the literal (module-level fixture).

- [ ] **Step 8.2: Update the test fixture to match the wire format**

In `frontend/src/lib/api/apiKeys.test.ts`, change the mocked HTTP response body (i.e. what `apiClient.post` resolves to) so its `.data` is `{data: <existing flat fixture>}`. Do not change the assertion — the test still asserts against the unwrapped `APIKeyCreateResponse`.

Concretely, if the current mock looks like:
```ts
const fakeResponse = {
  key: "jwt-xyz",
  id: 1,
  name: "k",
  scopes: ["assets:read"],
  created_at: "2026-04-23T00:00:00Z",
  expires_at: null,
};
vi.mocked(apiClient.post).mockResolvedValueOnce({ data: fakeResponse } as any);
```

change it to:
```ts
const fakeResponse = {
  key: "jwt-xyz",
  id: 1,
  name: "k",
  scopes: ["assets:read"],
  created_at: "2026-04-23T00:00:00Z",
  expires_at: null,
};
vi.mocked(apiClient.post).mockResolvedValueOnce({ data: { data: fakeResponse } } as any);
```

Keep whatever `as` casts / mock helpers the file already uses; the only change is wrapping the payload in `{ data: ... }`.

- [ ] **Step 8.3: Run the test and confirm it fails**

Run: `just frontend test -- apiKeys.test.ts`
Expected: the create-test FAILS because the module still does `return resp.data` and `resp.data` is now `{data: APIKeyCreateResponse}` — the returned object no longer looks like an `APIKeyCreateResponse`.

- [ ] **Step 8.4: Update the module to unwrap**

Edit `frontend/src/lib/api/apiKeys.ts:14-17`:

From:
```ts
create: async (orgId: number, req: CreateAPIKeyRequest): Promise<APIKeyCreateResponse> => {
  const resp = await apiClient.post<APIKeyCreateResponse>(`/orgs/${orgId}/api-keys`, req);
  return resp.data;
},
```

To:
```ts
create: async (orgId: number, req: CreateAPIKeyRequest): Promise<APIKeyCreateResponse> => {
  const resp = await apiClient.post<{ data: APIKeyCreateResponse }>(`/orgs/${orgId}/api-keys`, req);
  return resp.data.data;
},
```

- [ ] **Step 8.5: Re-run the test and confirm it passes**

Run: `just frontend test -- apiKeys.test.ts`
Expected: PASS.

- [ ] **Step 8.6: Run the full frontend suite**

Run: `just frontend test`
Expected: all tests PASS. If any other test mocks `apiKeys.create` and passes a flat `APIKeyCreateResponse`, adjust it the same way.

- [ ] **Step 8.7: Commit**

```bash
git add frontend/src/lib/api/apiKeys.ts frontend/src/lib/api/apiKeys.test.ts
git commit -m "fix(tra-454): unwrap wrapped CreateAPIKey response in frontend (C4)"
```

---

## Task 9: Full verification pass

**Files:** none modified (verification only).

- [ ] **Step 9.1: Full validate**

Run: `just validate`
Expected: lint + tests across both workspaces green.

- [ ] **Step 9.2: Manual sanity — Bearer casing via curl (optional but recommended)**

Start the backend locally (or rely on preview deploy after push).

Run (adjust host as needed):
```bash
curl -i -H 'Authorization: bearer invalid-token' http://localhost:8080/api/v1/users/me
```
Expected: 401 with `error.detail = "Bearer token is invalid or expired"` — i.e. the scheme was accepted and only the token was rejected. Re-run with `Authorization: BEARER ...` and `Authorization: BeArEr ...` to confirm. `Authorization: Basic foo` must still return `detail = "Authorization header must be Bearer <token>"`.

- [ ] **Step 9.3: Push the branch and open a PR**

```bash
git push -u origin miks2u/tra-454-api-contract-polish
gh pr create --title "fix(tra-454): API contract polish (Bearer casing, key-create wrap, validator cleanup)" --body "$(cat <<'EOF'
## Summary
- Case-insensitive `Bearer` scheme in `Auth`, `APIKeyAuth`, and `EitherAuth` (C3)
- Wrapped `POST /api/v1/orgs/{id}/api-keys` response in `{"data": {...}}` + frontend unwrap (C4)
- Clean decoder + validator error messages on key creation, with JSON-named fields (C5)

Docs items (C1, C6, C7) handled separately in trakrf-docs.

## Test plan
- [ ] `just backend test ./internal/middleware/...` passes (Bearer casing unit tests)
- [ ] `just backend test-integration ./internal/middleware/...` passes (EitherAuth casing)
- [ ] `just backend test-integration ./internal/handlers/orgs/...` passes (empty body, validation envelope, wrapped response)
- [ ] `just frontend test` passes (apiKeys.create mock updated)
- [ ] `just validate` green
- [ ] Manual curl with `bearer` / `BEARER` / `BeArEr` authenticates as expected
EOF
)"
```

Expected: PR URL returned; preview deploy triggers automatically per the repo's workflow.

---

## Self-Review Notes

**Spec coverage:**

| Spec section | Task(s) |
|---|---|
| C3 middleware.go (Auth) | 1 |
| C3 apikey.go (APIKeyAuth) | 2 |
| C3 either_auth.go (EitherAuth) | 3 |
| C5a JSONTagNameFunc registration | 4 |
| C5b RespondValidationError | 5 |
| C5c decoder error strip | 5 |
| C5d tests | 6 |
| C4a response wrap | 7 |
| C4b swagger annotation | 7 |
| C4c frontend unwrap | 8 |
| C4d test fixtures | 8 |
| C4e backend test of wrapping | 7 |
| Full verification | 9 |

All spec items covered. Out-of-scope items (C1, C6, C7 docs) are explicitly not included per the scope decision in the spec.

**Placeholder scan:** No `TBD` / `TODO` / "handle edge cases" / etc. Step 3.2 and 6.2 call out potential fallback lookups with concrete grep commands; these are recovery paths, not placeholders.

**Type consistency:** `APIKeyCreateResponse` used identically in tests and module. `RespondValidationError` signature matches its definition at `httputil/validation.go:136`. Swagger annotation style copied verbatim from the sibling `ListAPIKeys` annotation.
