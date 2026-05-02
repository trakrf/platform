# TRA-571 Remove `scans:write` from Public Surface Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Drop `scans:write` from every public-facing surface — the API key minting UI scope picker, the frontend `Scope` union, and the backend `ValidScopes` validation map for the public POST `/api/v1/orgs/{id}/api-keys` endpoint. Resolves S6 + V5 from BB13 (mintable scope variant with no public endpoint that needs it).

**Architecture:** Three small changes that travel together.

1. **Frontend picker (`ScopeSelector.tsx`):** Flip the `scans` row from `hasWrite: true` to `hasWrite: false` so the dropdown only renders `None` and `Read`. The selector logic is data-driven from the `RESOURCES` table — no other code paths to touch.
2. **Frontend types (`types/apiKey.ts`):** Drop `'scans:write'` from the `Scope` union and from the `ALL_SCOPES` constant. `ALL_SCOPES` is exported but currently unused in app code, so this is a pure type-narrowing cleanup that prevents future code from accidentally re-introducing the value.
3. **Backend validation (`models/apikey/apikey.go`):** Remove `"scans:write"` from `apikey.ValidScopes`. The public `CreateAPIKey` handler reads this map (`api_keys.go:97`) to reject unknown scopes, so removing the entry causes any direct POST that bypasses the UI (curl, Postman, an external integrator) to get a `400 Invalid scope`. Internal handlers (`router.go:199` `RequireScope("scans:write")`, `inventory/save.go` `@Security APIKey[scans:write]`) keep referencing the literal scope string — `RequireScope` checks the JWT's scope claim against the literal, it does not look up `ValidScopes`. This is the "internal-only is fine — internal handlers may still reference it — but it should not appear on any public-facing surface" disposition the ticket calls out.

**Pre-launch context:** No production API keys exist yet (platform launch is still ahead — first prod tag tracked under TRA-485). That removes the entire "what about existing keys with `scans:write` already authorized?" question from this ticket. There is nothing in the wild to grandfather. Preview keys are throwaway dev artifacts and are treated the same.

No public OpenAPI spec change is required: `grep scans:write docs/api/openapi.public.yaml` already returns nothing today (the spec does not enumerate scopes, and the only operation that declared `scans:write` — `/inventory/save` — is `@Tags inventory,internal` and was partitioned out by TRA-547). No docs PR is required: trakrf-docs already references `scans:read` only (TRA-563 cleanup landed). Both facts are verified explicitly in Task 5.

**Tech Stack:** React 18 + TypeScript (Vitest + `@testing-library/react`), Go 1.22 (`net/http/httptest` for handler tests), `pnpm`, `just` task runner.

---

## File Structure

**Frontend — modify:**
- `frontend/src/components/apikeys/ScopeSelector.tsx:16` — flip `hasWrite: true` → `hasWrite: false` for the `scans` row entry. This is the only code change to the picker; the `levelFor` / `scopesFor` helpers are unchanged.
- `frontend/src/components/apikeys/ScopeSelector.test.tsx:35-45` — replace the `'emits scans:read + scans:write for "Read + Write" on Scans'` case with a new case asserting the **opposite** (no `Read + Write` option for Scans). Other tests are unchanged.
- `frontend/src/types/apiKey.ts:7` — drop `| 'scans:write'` from the `Scope` union.
- `frontend/src/types/apiKey.ts:44` — drop `'scans:write',` from `ALL_SCOPES`.

**Backend — modify:**
- `backend/internal/models/apikey/apikey.go:12` — delete the `"scans:write": true,` map entry from `ValidScopes`.
- `backend/internal/handlers/orgs/api_keys_integration_test.go` — add an integration test asserting `POST /api/v1/orgs/{id}/api-keys` with `scans:write` in the body returns `400 Invalid scope`. Pre-existing tests are unchanged.

**Leave alone (verified, not edited):**
- `backend/internal/cmd/serve/router.go:199` — `RequireScope("scans:write")` on `/api/v1/inventory/save`. Internal route, internal scope check; keeping the literal preserves any already-minted keys' ability to call the internal endpoint.
- `backend/internal/handlers/inventory/save.go:75,175` — `@Security APIKey[scans:write]` annotation and the inline comment. The handler is `@Tags inventory,internal`; the annotation only appears in the internal spec. No customer-facing surface is affected.
- `docs/api/openapi.public.yaml`, `backend/internal/handlers/swaggerspec/openapi.public.{yaml,json}` — no regen required; the public spec already does not reference `scans:write`. Verified in Task 5.
- `/home/mike/trakrf-docs` — already references only `scans:read` per TRA-563. Verified in Task 5.

---

## Inventory (read this before starting)

**`scans:write` references in the platform repo (as of plan write time):**

| File | Line | Disposition |
| ---- | ---- | ----------- |
| `frontend/src/types/apiKey.ts` | 7 | Remove (Task 3) |
| `frontend/src/types/apiKey.ts` | 44 | Remove (Task 3) |
| `frontend/src/components/apikeys/ScopeSelector.tsx` | 16 | Flip `hasWrite` (Task 1) |
| `frontend/src/components/apikeys/ScopeSelector.test.tsx` | 35-45 | Rewrite assertion (Task 2) |
| `backend/internal/models/apikey/apikey.go` | 12 | Remove (Task 4) |
| `backend/internal/handlers/inventory/save.go` | 75, 175 | Keep — internal handler |
| `backend/internal/cmd/serve/router.go` | 199 | Keep — internal route |
| `backend/docs/docs.go` | 1350 | Generated; will be regenerated by `just backend api-spec` in Task 6 |

**Branch and worktree:**
- Branch: `chore/tra-571-remove-scans-write-from-public-surface` (functional prefix per `feedback_branch_naming_functional`).
- Worktree: per `feedback_worktree_location`, create under `.worktrees/`. Linear's auto-generated `miks2u/tra-571-...` branch name is overridden in favor of the project convention.

**Existing-keys disposition:** moot — pre-launch, no production API keys exist (TRA-485 covers the first prod tag). Skip the ticket's "leave-as-is vs mass-revoke" debate and skip any disposition section in the PR body.

---

### Task 1: Set up the worktree

**Files:** none (worktree setup).

- [ ] **Step 1: Create worktree from main**

```bash
cd /home/mike/platform
git fetch origin main
git worktree add .worktrees/tra-571 -b chore/tra-571-remove-scans-write-from-public-surface origin/main
cd .worktrees/tra-571
```

- [ ] **Step 2: Confirm starting state**

```bash
git status
git log --oneline -3
```

Expected: clean worktree on `chore/tra-571-remove-scans-write-from-public-surface`, branched from the latest `origin/main`. From this point forward, all commands run from the worktree directory.

---

### Task 2: Add the failing frontend test for the picker change

**Files:**
- Modify: `frontend/src/components/apikeys/ScopeSelector.test.tsx` (lines 35-45)

- [ ] **Step 1: Replace the existing scans test with the new "no write option" assertion**

In `frontend/src/components/apikeys/ScopeSelector.test.tsx`, find the existing test (the one starting at the line that reads `it('emits scans:read + scans:write for "Read + Write" on Scans'`):

```tsx
  it('emits scans:read + scans:write for "Read + Write" on Scans', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={[]} onChange={onChange} />);
    const select = screen.getByLabelText(/scans/i);
    // Guard: the Read+Write option must actually be rendered for Scans — a prior
    // regression (hasWrite=false) hid this option but fireEvent.change would still
    // dispatch the onChange, masking the bug.
    expect(within(select).getByRole('option', { name: /read \+ write/i })).toBeInTheDocument();
    fireEvent.change(select, { target: { value: 'readwrite' } });
    expect(onChange).toHaveBeenCalledWith(['scans:read', 'scans:write']);
  });
```

Replace with:

```tsx
  it('does not offer "Read + Write" on Scans (TRA-571 — scans:write is internal-only)', () => {
    render(<ScopeSelector value={[]} onChange={() => {}} />);
    const select = screen.getByLabelText(/scans/i);
    expect(within(select).getByRole('option', { name: /^none$/i })).toBeInTheDocument();
    expect(within(select).getByRole('option', { name: /^read$/i })).toBeInTheDocument();
    expect(within(select).queryByRole('option', { name: /read \+ write/i })).not.toBeInTheDocument();
  });

  it('still offers "Read + Write" on Assets and Locations', () => {
    render(<ScopeSelector value={[]} onChange={() => {}} />);
    const assets = screen.getByLabelText(/assets/i);
    const locations = screen.getByLabelText(/locations/i);
    expect(within(assets).getByRole('option', { name: /read \+ write/i })).toBeInTheDocument();
    expect(within(locations).getByRole('option', { name: /read \+ write/i })).toBeInTheDocument();
  });
```

The first replacement test is the inverse of the original assertion. The second guards against a fat-finger regression where someone flips `hasWrite` on the wrong row(s).

- [ ] **Step 2: Run the test to verify it fails**

From the worktree root:

```bash
just frontend test src/components/apikeys/ScopeSelector.test.tsx
```

Expected: the new `does not offer "Read + Write" on Scans` test FAILS with something like:

```
TestingLibraryElementError: Found multiple elements with the role "option" and name `/read \+ write/i`
```

…or, more likely, the `not.toBeInTheDocument()` assertion fails because the option is still rendered. The "still offers Read + Write on Assets and Locations" test should PASS already.

If both pass: stop — the picker has already been changed; back-check the `ScopeSelector.tsx` source before continuing.

- [ ] **Step 3: Commit the test change**

```bash
git add frontend/src/components/apikeys/ScopeSelector.test.tsx
git commit -m "test(apikeys): assert scans picker has no Read+Write option (TRA-571)

Replaces the prior assertion that the scans row emitted scans:read +
scans:write with the inverse: the Read + Write option must not be
rendered for Scans, while Assets and Locations still offer it. Pairs
with the picker change in the next commit."
```

---

### Task 3: Make the frontend test pass — drop `hasWrite` from the scans row

**Files:**
- Modify: `frontend/src/components/apikeys/ScopeSelector.tsx:16`

- [ ] **Step 1: Edit the resources table**

In `frontend/src/components/apikeys/ScopeSelector.tsx`, find:

```tsx
const RESOURCES: { key: ResourceKey; label: string; hasWrite: boolean }[] = [
  { key: 'assets',    label: 'Assets',    hasWrite: true },
  { key: 'locations', label: 'Locations', hasWrite: true },
  { key: 'scans',     label: 'Scans',     hasWrite: true },
];
```

Change to:

```tsx
const RESOURCES: { key: ResourceKey; label: string; hasWrite: boolean }[] = [
  { key: 'assets',    label: 'Assets',    hasWrite: true },
  { key: 'locations', label: 'Locations', hasWrite: true },
  { key: 'scans',     label: 'Scans',     hasWrite: false },
];
```

That single-letter flip is the entire change. The conditional `{r.hasWrite && <option value="readwrite">Read + Write</option>}` already gates rendering on the flag, so the option simply stops appearing for Scans.

- [ ] **Step 2: Re-run the picker tests**

```bash
just frontend test src/components/apikeys/ScopeSelector.test.tsx
```

Expected: all 9 tests pass (the 7 untouched tests, plus the two replacement tests from Task 2).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/apikeys/ScopeSelector.tsx
git commit -m "feat(apikeys): remove Read+Write option from scans scope picker (TRA-571)

Flips hasWrite=false on the scans row in ScopeSelector. The picker now
offers None and Read for Scans — Read+Write is gone. Resolves S6 from
BB13: scans:write was mintable in the UI but no public endpoint required
it (the only handler that uses it, /api/v1/inventory/save, is internal-
only per TRA-547). Internal-only callers continue to use scans:write
through the chassis app's session JWT path."
```

---

### Task 4: Drop `scans:write` from the frontend `Scope` union and `ALL_SCOPES`

**Files:**
- Modify: `frontend/src/types/apiKey.ts:7,44`

- [ ] **Step 1: Edit the type and the constant**

In `frontend/src/types/apiKey.ts`, find:

```ts
export type Scope =
  | 'assets:read'
  | 'assets:write'
  | 'locations:read'
  | 'locations:write'
  | 'scans:read'
  | 'scans:write'
  | 'keys:admin';
```

Change to:

```ts
export type Scope =
  | 'assets:read'
  | 'assets:write'
  | 'locations:read'
  | 'locations:write'
  | 'scans:read'
  | 'keys:admin';
```

And find:

```ts
export const ALL_SCOPES: Scope[] = [
  'assets:read',
  'assets:write',
  'locations:read',
  'locations:write',
  'scans:read',
  'scans:write',
  'keys:admin',
];
```

Change to:

```ts
export const ALL_SCOPES: Scope[] = [
  'assets:read',
  'assets:write',
  'locations:read',
  'locations:write',
  'scans:read',
  'keys:admin',
];
```

- [ ] **Step 2: Type-check**

```bash
just frontend typecheck
```

Expected: clean. The `Scope` union narrowing is a removal, so any callsite that referenced the literal `'scans:write'` would now error — but a repo-wide grep before plan write confirmed no app-code callsites exist (only the rewritten test in Task 2 and this file). If typecheck reports an error, surface the offending file and add a follow-up step to the plan; do not silently broaden the type back.

- [ ] **Step 3: Run the full frontend test suite**

```bash
just frontend test
```

Expected: green. The `Scope` cast in `ScopeSelector.tsx` (`scopes.includes(\`${resource}:write\` as Scope)`) is a string concatenation cast and tolerates the union narrowing — `levelFor('scans', ...)` will always read `'scans:write'` as a missing key in the value array, which is the correct behavior for a scope that can no longer be present.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types/apiKey.ts
git commit -m "chore(types): drop scans:write from Scope union and ALL_SCOPES (TRA-571)

scans:write is internal-only after TRA-547 flipped /api/v1/inventory/save
to the internal-only spec. Removing the literal from the public Scope
union prevents future frontend code from accidentally re-introducing it
on minted keys. ALL_SCOPES is exported but currently unused in app code,
so this is a pure type-narrowing cleanup."
```

---

### Task 5: Verify the public spec and trakrf-docs are already clean (no edits)

**Files:** read-only verification.

- [ ] **Step 1: Confirm the public OpenAPI spec does not reference `scans:write`**

```bash
grep -n "scans:write" docs/api/openapi.public.yaml docs/api/openapi.public.json
```

Expected: no matches. The public partition strips `/inventory/save` (the only operation that declared `scans:write`), and the spec does not enumerate scopes anywhere else.

If matches appear: the public spec has drifted since plan write — stop and add a regen step. The most likely cause would be a new public-tagged handler that declared `@Security APIKey[scans:write]`; that would itself be a bug to investigate, not a spec to regenerate around.

- [ ] **Step 2: Confirm `scans:write` survives in the internal scope path**

```bash
grep -rn "scans:write" backend/internal/cmd/serve/router.go backend/internal/handlers/inventory/save.go
```

Expected: at least the three known references —
- `router.go` `RequireScope("scans:write")` line ~199
- `inventory/save.go` `@Security APIKey[scans:write]` line ~75
- `inventory/save.go` inline comment line ~175

These confirm internal callers still authorize and gate against the scope. `RequireScope` checks JWT claims directly, not `ValidScopes`, so the literal-string path remains intact for any future internal-only mint flow that needs it.

- [ ] **Step 3: Confirm trakrf-docs references only `scans:read`**

```bash
grep -rn "scans:write\|scans:read" /home/mike/trakrf-docs/docs/
```

Expected: every match is `scans:read`. If a `scans:write` reference appears, open a follow-up docs PR in `trakrf-docs` per `feedback_docs_prs_separate_checkout` (do not edit `/home/mike/trakrf-docs` directly). At plan write time this grep returned only `scans:read` references in `quickstart.mdx`, `getting-started/api.mdx`, and `authentication.md`.

If all three checks pass, no further docs/spec work is needed — proceed to Task 6.

---

### Task 6: Backend — write the failing validation test

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys_integration_test.go`

The existing integration test file already exercises `CreateAPIKey` end-to-end (router + storage). Add a new subtest that mints with `scans:write` and asserts `400 Invalid scope`.

- [ ] **Step 1: Append the new test function**

In `backend/internal/handlers/orgs/api_keys_integration_test.go`, append a new top-level test function below the existing `TestCreateAPIKey_ValidationFailed_JSONFieldNames` case (line ~416). It follows the same harness shape as the existing validation tests in this file (`testutil.SetupTestDB`, `seedAdminUser`, `newAdminRouter`, `httptest.NewRequest` / `httptest.NewRecorder`):

```go
// TestCreateAPIKey_RejectsScansWrite pins the TRA-571 contract: the public
// CreateAPIKey handler must reject scans:write because the scope is no
// longer in apikey.ValidScopes. Internal handlers still reference the
// literal "scans:write" string against JWT claims (router.go RequireScope
// and inventory/save.go @Security annotation), but no new key may be
// minted with it via the public surface.
func TestCreateAPIKey_RejectsScansWrite(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-tra571")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, sessionToken := seedAdminUser(t, pool, orgID)

	r := newAdminRouter(t, store)

	body := []byte(`{"name":"tra-571-guard","scopes":["scans:write"]}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)
	assert.Equal(t, "Invalid scope", resp.Error.Title)
	assert.Contains(t, resp.Error.Detail, "scans:write",
		"detail must name the offending scope so the integrator knows what to remove")
}

// TestCreateAPIKey_AcceptsScansRead is the positive companion: scans:read
// is still a valid public scope and must continue to mint cleanly.
func TestCreateAPIKey_AcceptsScansRead(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-tra571-read")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	_, sessionToken := seedAdminUser(t, pool, orgID)

	r := newAdminRouter(t, store)

	body := []byte(`{"name":"tra-571-positive","scopes":["scans:read"]}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%d/api-keys", orgID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}
```

The two helper symbols this test relies on (`seedAdminUser`, `newAdminRouter`, `testutil.SetupTestDB`, `testutil.CreateTestAccount`) are all already in scope in this file — they are used by the surrounding tests (lines 77, 119, 154, 336, 372, etc.). The imports needed (`bytes`, `encoding/json`, `fmt`, `net/http`, `net/http/httptest`, `testing`, `github.com/jackc/pgx/v5/pgxpool`, `github.com/stretchr/testify/assert`, `github.com/stretchr/testify/require`, `testutil`) are all already imported in the file.

The negative test asserts the exact title `"Invalid scope"` and type `"validation_error"` because that is what the handler emits today (`backend/internal/handlers/orgs/api_keys.go:97-101`):

```go
for _, s := range req.Scopes {
    if !apikey.ValidScopes[s] {
        httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
            "Invalid scope", "Unknown scope: "+s, reqID)
        return
    }
}
```

If the assertions above ever drift from the handler's actual envelope (because of an unrelated error-shape refactor), update the test to match — do not weaken it.

- [ ] **Step 2: Run the new tests to verify the negative case fails**

```bash
just backend test ./internal/handlers/orgs/...
```

Expected: `TestCreateAPIKey_RejectsScansWrite` FAILS — the response is 201 (created), not 400, because `scans:write` is still in `ValidScopes` at this point. `TestCreateAPIKey_AcceptsScansRead` should already PASS (`scans:read` is unchanged).

If the negative test passes against an unmodified `ValidScopes`: stop and re-read `models/apikey/apikey.go` — `scans:write` may have already been removed.

- [ ] **Step 3: Commit the tests**

```bash
git add backend/internal/handlers/orgs/api_keys_integration_test.go
git commit -m "test(api-keys): pin the scans:write rejection on public CreateAPIKey (TRA-571)

Failing test that exercises the TRA-571 contract: POST
/api/v1/orgs/{id}/api-keys with scans:write in the scope list must
return 400. Pairs with the apikey.ValidScopes change in the next
commit."
```

---

### Task 7: Backend — drop `scans:write` from `apikey.ValidScopes`

**Files:**
- Modify: `backend/internal/models/apikey/apikey.go:12`

- [ ] **Step 1: Edit the map**

In `backend/internal/models/apikey/apikey.go`, find:

```go
// ValidScopes is the canonical set of scope strings accepted by the public API.
var ValidScopes = map[string]bool{
	"assets:read":     true,
	"assets:write":    true,
	"locations:read":  true,
	"locations:write": true,
	"scans:read":      true,
	"scans:write":     true,
	"keys:admin":      true,
}
```

Change to:

```go
// ValidScopes is the canonical set of scope strings accepted on key minting via
// the public POST /api/v1/orgs/{id}/api-keys endpoint. scans:write is
// intentionally absent — it is an internal-only scope (the only handler that
// references it, /api/v1/inventory/save, is @Tags inventory,internal per
// TRA-547). Already-minted keys with scans:write continue to authenticate
// against the internal endpoint because middleware.RequireScope checks the
// JWT's scope claim against the literal string, not against ValidScopes.
var ValidScopes = map[string]bool{
	"assets:read":     true,
	"assets:write":    true,
	"locations:read":  true,
	"locations:write": true,
	"scans:read":      true,
	"keys:admin":      true,
}
```

The expanded comment is the durable record of *why* the map omits a scope that still appears as a literal in the codebase — it heads off a future "this looks inconsistent, let me re-add it" cleanup.

- [ ] **Step 2: Re-run the test that was failing in Task 6 Step 2**

```bash
just backend test ./internal/handlers/orgs/...
```

Expected: green. The new `TestCreateAPIKey_RejectsScansWrite` now passes; existing tests remain green (none of them mint with `scans:write` — confirmed by `grep -rn 'scans:write' backend/internal/handlers/orgs/` which returns nothing in test files at plan write time).

- [ ] **Step 3: Run the model unit tests**

```bash
just backend test ./internal/models/apikey/...
```

Expected: green. `apikey_wire_test.go` and other model tests do not iterate `ValidScopes` (verified via `grep ValidScopes backend/internal/models/apikey/...` which only matches the declaration), so removing one entry should not break wire-level tests.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/models/apikey/apikey.go
git commit -m "fix(api-keys): drop scans:write from public ValidScopes (TRA-571)

scans:write is internal-only after TRA-547 — the only endpoint that
gates on it (/api/v1/inventory/save) is @Tags inventory,internal.
Removing it from ValidScopes makes the public CreateAPIKey handler
reject mints that include the scope (400 Invalid scope), closing the
direct-curl bypass that the UI removal in TRA-571's frontend tasks
alone could not.

Internal RequireScope(\"scans:write\") in router.go is unaffected — it
checks JWT claims against the literal string, not ValidScopes. Pre-
launch, no production keys exist, so there is no grandfathering
question to resolve."
```

---

### Task 8: Regenerate the swagger artifacts (mechanical)

The frontend changes do not touch the Go API spec, but the backend `ValidScopes` change technically does not regenerate either. So this step exists only to keep the committed `backend/docs/docs.go` and the public/internal yaml/json artifacts in sync with `HEAD`. Running it is cheap and surfaces unrelated drift early.

**Files:**
- Possibly modify (regenerated): `backend/docs/docs.go`, `docs/api/openapi.public.{yaml,json}`, `backend/internal/handlers/swaggerspec/openapi.{public,internal}.{yaml,json}`

- [ ] **Step 1: Regenerate**

```bash
just backend api-spec
```

Expected output ends with the two `✅ Public spec` / `✅ Internal spec` lines.

- [ ] **Step 2: Inspect the diff**

```bash
git diff -- backend/docs/docs.go docs/api/openapi.public.yaml docs/api/openapi.public.json backend/internal/handlers/swaggerspec/
```

Expected: empty diff, OR a small diff that only reflects environment drift (timestamps, generator version bump). Do **not** commit any change that touches scope enumerations or path additions/removals — those would indicate this task surfaced an unrelated issue that needs investigation, not a quick mechanical commit.

- [ ] **Step 3 (only if Step 2 is empty): skip the commit**

If `git diff` is empty, there is nothing to commit. Move on to Task 9.

- [ ] **Step 4 (only if Step 2 shows benign drift): commit**

```bash
git add backend/docs/docs.go docs/api/openapi.public.yaml docs/api/openapi.public.json backend/internal/handlers/swaggerspec/
git commit -m "chore(api): regenerate swagger artifacts (TRA-571 — no semantic change)

Mechanical regeneration after the ValidScopes change. The diff is
limited to generator-side drift; no scope or path content changes
because scans:write was already absent from the public spec partition
(it lived only on the @Tags internal /inventory/save handler)."
```

---

### Task 9: Run the full backend + frontend validate sweep

**Files:** read-only verification.

- [ ] **Step 1: Backend tests**

```bash
just backend test
```

Expected: all packages pass.

- [ ] **Step 2: Backend lint + api-lint**

```bash
just backend lint
just backend api-lint
```

Expected: clean. `api-lint` (Redocly) should report 0 errors; pre-existing warnings unrelated to security schemes are acceptable.

- [ ] **Step 3: Repo-wide validate**

```bash
just validate
```

Expected: lint + tests pass on both workspaces. If frontend type-check fails because some component imports `Scope` and references the removed literal, surface the file path — that would be a real breakage that needs investigation rather than a quick narrow-the-type rollback.

---

### Task 10: Push the branch and open the PR

**Files:** none (git/CI).

- [ ] **Step 1: Push**

```bash
git push -u origin chore/tra-571-remove-scans-write-from-public-surface
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "chore(api-keys): drop scans:write from public scope picker + ValidScopes (TRA-571)" --body "$(cat <<'EOF'
## Summary

- Removes the **Read + Write** option from the **Scans** row of the API key minting UI. The picker now offers **None** and **Read** for Scans only — Assets and Locations still offer Read + Write.
- Drops \`'scans:write'\` from the frontend \`Scope\` union and the \`ALL_SCOPES\` constant so future frontend code cannot accidentally re-introduce the value.
- Removes \`"scans:write"\` from \`apikey.ValidScopes\` in the backend, so direct-curl mints (UI bypass) get \`400 Invalid scope\`.
- Resolves S6 + V5 from BB13: \`scans:write\` was mintable but no public endpoint required it (the only handler that uses it, \`/api/v1/inventory/save\`, was flipped \`@Tags inventory,internal\` in TRA-547). Pairs with the public-surface cleanup theme of TRA-568.

## What is intentionally **not** changed

- Internal references to the literal \`scans:write\` string remain — \`backend/internal/cmd/serve/router.go\` \`RequireScope(\"scans:write\")\` and \`backend/internal/handlers/inventory/save.go\` \`@Security APIKey[scans:write]\`. Internal handlers may still gate on the scope; \`middleware.RequireScope\` checks the JWT's scope claim against the literal string and does not consult \`ValidScopes\`. This is the disposition the ticket calls out.
- The public OpenAPI spec is unchanged — \`grep scans:write docs/api/openapi.public.yaml\` already returned no matches before this PR (\`/inventory/save\` is partitioned out of the public spec).
- \`trakrf-docs\` is unchanged — the Authentication scopes table already only mentions \`scans:read\` (TRA-563 cleanup).

## Existing keys

Pre-launch — no production keys exist. The ticket's "leave-as-is vs mass-revoke" debate is moot.

## Test plan

- [ ] \`just frontend test\` — new picker-shape assertion passes; the scans Read+Write option no longer renders
- [ ] \`just backend test\` — new \`TestCreateAPIKey_RejectsScansWrite\` integration test passes
- [ ] \`just backend api-lint\` — Redocly 0 errors
- [ ] \`grep scans:write docs/api/openapi.public.yaml\` — empty (regression guard)
- [ ] Manual smoke on preview at \`https://app.preview.trakrf.id\`: open API Keys → New key, confirm the Scans dropdown only shows None / Read; mint a test key with \`scans:read\`, confirm it works against \`GET /api/v1/locations/current\`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Capture PR URL** for use in the manual smoke step.

---

### Task 11: Manual smoke test on the preview deploy

**Files:** none (manual verification on `https://app.preview.trakrf.id`).

- [ ] **Step 1: Wait for preview deploy**

`gh pr checks` should show the preview job green. The preview URL is `https://app.preview.trakrf.id` (sync-preview workflow re-uses the single shared preview app).

- [ ] **Step 2: Sign in to preview**

Log in with the test admin account. No console errors expected.

- [ ] **Step 3: Open API Keys → New key**

Avatar menu → API Keys → **New key**. Locate the **Permissions** fieldset.

Expected dropdown options per row:
- Assets: None / Read / Read + Write
- Locations: None / Read / Read + Write
- **Scans: None / Read** (no Read + Write)
- Key management: None / Admin

If the Scans row still shows Read + Write, the build did not pick up the change — recheck the preview-deploy commit SHA against the PR head SHA before assuming a regression.

- [ ] **Step 4: Mint a throwaway key with scans:read only**

Name it `tra-571-smoke`, pick **Scans → Read** and nothing else. Submit, copy the JWT.

- [ ] **Step 5: Verify the key works for a scans:read endpoint**

```bash
curl -sS -H "Authorization: Bearer <jwt>" https://app.preview.trakrf.id/api/v1/locations/current?limit=1 | jq
```

Expected: 200 with a `data` array (possibly empty depending on preview state); `total_count` field present per the response envelope.

- [ ] **Step 6: Verify a scans:write attempt would have been rejected (sanity, optional)**

Try to mint a second throwaway key with `scans:write` via curl, bypassing the UI:

```bash
curl -sS -X POST \
  -H "Authorization: Bearer <admin-session-jwt>" \
  -H "Content-Type: application/json" \
  -d '{"name":"tra-571-bypass-test","scopes":["scans:write"]}' \
  https://app.preview.trakrf.id/api/v1/orgs/<org-id>/api-keys
```

Expected: 400 with an error body containing `"Unknown scope: scans:write"` (exact wording per the model errors envelope). This is the curl-bypass close that the backend `ValidScopes` change buys.

If you do not have a convenient admin-session JWT in hand, skip Step 6 — the integration test in Task 6 covers the same contract at the unit level.

- [ ] **Step 7: Revoke the throwaway test key**

From the API Keys list, revoke `tra-571-smoke`. Confirms the existing revoke flow is unaffected.

- [ ] **Step 8: Comment the result on the PR**

A short note like:

> Preview smoke: scans dropdown shows None/Read only at preview deploy `<sha>`; minted key with `scans:read` returned 200 against `/api/v1/locations/current`; bypass-curl with `scans:write` returned 400 (`Unknown scope: scans:write`). Revoked the test key.

This is the BB14 acceptance evidence.

- [ ] **Step 9: Merge**

Per project convention, **merge commit, never squash** (`feedback_no_squash_merges`). Wait for the merge to land on `main`.

---

## Acceptance criteria — final mapping

| Ticket AC | Where covered |
| --------- | ------------- |
| `read/write` option removed from scans scope picker in the API key minting UI | Tasks 2 + 3 (test + impl) |
| `none` and `read` options remain in the scans scope picker | Task 2 (positive assertions on the remaining options) + Task 3 |
| No public OpenAPI spec content references `scans:write` as a public scope | Task 5 Step 1 (verification — already true; no change needed) |
| `scans:read` remains in the public OpenAPI spec scope enumeration | Task 5 Step 1 (the unrelated `scans:read` matches in `openapi.public.yaml` are left in place) |
| Manual smoke test: mint a new API key in preview, verify scans picker shows only `none` and `read` | Task 11 Steps 3-4 |
| Existing keys with `scans:write` authorized documented in PR description | N/A — pre-launch, no production keys exist; covered by a one-liner in the PR body |
| PR merged before BB14 verification | Task 11 Step 9 |
