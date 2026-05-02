# TRA-578 Public-Spec Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Drop the `/api/v1/orgs/{id}/api-keys*` endpoints from the public OpenAPI spec, and rename the `scans:read` scope to `history:read` across spec, code, DB, and SPA.

**Architecture:** Two coupled cleanups landed in one PR. (1) Flip `@Tags api-keys,public` → `@Tags api-keys,internal` on all four api-keys handlers so the partition tool routes them to the internal spec only. (2) Hard-cut rename `scans:read` → `history:read` in `ValidScopes`, route `RequireScope` calls, swag `@Security` annotations, frontend `Scope` type/UI, and migrate `api_keys.scopes` rows. Pre-launch (no prod keys) means no compatibility alias is needed.

**Tech Stack:** Go (backend, swag for OpenAPI gen, custom `apispec` partition tool), TypeScript/React (frontend), TimescaleDB (PostgreSQL migrations via `golang-migrate`), pnpm/Vitest, just task runner.

---

## Working directory

All work happens in `/home/mike/platform` on branch `feat/tra-578-public-spec-cleanup-history-read`. The branch already exists. Run `just` recipes from the repo root or workspace root per CLAUDE.md.

Preview DB access: `psql "$PG_URL_PREVIEW"` is available if needed.

---

## Task 1: Update `ValidScopes` in the apikey model

**Files:**
- Modify: `backend/internal/models/apikey/apikey.go:7-19`

- [ ] **Step 1: Edit `ValidScopes` map**

Change line 17 from `"scans:read":      true,` to `"history:read":    true,`. Update the surrounding doc comment to reference the new name. Also update the comment on line 9 — replace `scans:write` reference if needed; current comment is accurate, just confirm.

Final state of the map (lines 13-20):
```go
var ValidScopes = map[string]bool{
	"assets:read":     true,
	"assets:write":   true,
	"locations:read":  true,
	"locations:write": true,
	"history:read":    true,
	"keys:admin":      true,
}
```

(Preserve the project's existing alignment; gofmt will fix any drift.)

- [ ] **Step 2: Run gofmt to normalize**

```bash
just backend lint
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/models/apikey/apikey.go
git commit -m "refactor(apikey): TRA-578 rename scans:read scope to history:read in ValidScopes"
```

---

## Task 2: Rename `RequireScope` calls and route comment

**Files:**
- Modify: `backend/internal/cmd/serve/router.go:166-169`

- [ ] **Step 1: Update both `RequireScope` calls and the explanatory comment**

Change:

```go
		// require scans:read per TRA-392 — they moved under /locations/ and /assets/ for URL
		// shape but remain scan-derived data.
		r.With(middleware.RequireScope("scans:read")).Get("/api/v1/locations/current", reportsHandler.ListCurrentLocations)
		r.With(middleware.RequireScope("scans:read")).Get("/api/v1/assets/{id}/history", reportsHandler.GetAssetHistory)
```

to:

```go
		// require history:read per TRA-578 — these endpoints expose scan-derived
		// history data; the scope name aligns with the /history endpoint vocabulary.
		r.With(middleware.RequireScope("history:read")).Get("/api/v1/locations/current", reportsHandler.ListCurrentLocations)
		r.With(middleware.RequireScope("history:read")).Get("/api/v1/assets/{id}/history", reportsHandler.GetAssetHistory)
```

(Preserve any second comment line that exists at line 167 verbatim; only the scope literal and lead comment change.)

- [ ] **Step 2: Verify build**

```bash
just backend build
```
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/cmd/serve/router.go
git commit -m "refactor(router): TRA-578 require history:read on /locations/current and /assets/{id}/history"
```

---

## Task 3: Update `@Security` annotations on report handlers

**Files:**
- Modify: `backend/internal/handlers/reports/asset_history.go:64`
- Modify: `backend/internal/handlers/reports/current_locations.go:56`

- [ ] **Step 1: Edit asset_history.go**

Change line 64 from `// @Security APIKey[scans:read]` to `// @Security APIKey[history:read]`.

- [ ] **Step 2: Edit current_locations.go**

Change line 56 from `// @Security APIKey[scans:read]` to `// @Security APIKey[history:read]`.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/reports/asset_history.go backend/internal/handlers/reports/current_locations.go
git commit -m "docs(reports): TRA-578 update @Security annotations to history:read"
```

---

## Task 4: Flip api-keys handlers to internal-only

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys.go:37,146,214,261`

- [ ] **Step 1: Replace all four `@Tags api-keys,public` with `@Tags api-keys,internal`**

Each of the four handler doc comments has `// @Tags api-keys,public`. Change each to `// @Tags api-keys,internal`.

Use a single `sed -i` from the repo root (verify the count first):

```bash
grep -c "// @Tags api-keys,public" backend/internal/handlers/orgs/api_keys.go
# Expected: 4
sed -i 's|// @Tags api-keys,public|// @Tags api-keys,internal|g' backend/internal/handlers/orgs/api_keys.go
grep -c "// @Tags api-keys,public" backend/internal/handlers/orgs/api_keys.go
# Expected: 0
grep -c "// @Tags api-keys,internal" backend/internal/handlers/orgs/api_keys.go
# Expected: 4
```

- [ ] **Step 2: Verify build**

```bash
just backend build
```
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys.go
git commit -m "refactor(api-keys): TRA-578 mark all /orgs/{id}/api-keys handlers internal-only"
```

---

## Task 5: Add migration 000039 for `scans:read` → `history:read`

**Files:**
- Create: `backend/migrations/000039_rename_scans_read_scope.up.sql`
- Create: `backend/migrations/000039_rename_scans_read_scope.down.sql`

- [ ] **Step 1: Confirm next migration number**

```bash
ls backend/migrations/ | grep -E '^[0-9]+' | sort | tail -5
```
Expected: latest is `000038_location_path_backfill_and_cascade.{up,down}.sql`. Next is 000039.

- [ ] **Step 2: Write the up migration**

Create `backend/migrations/000039_rename_scans_read_scope.up.sql`:

```sql
SET search_path=trakrf,public;

-- TRA-578: rename scans:read → history:read so the scope vocabulary aligns
-- with /assets/{id}/history and /locations/current rather than a non-existent
-- /scans resource. Hard cut: pre-launch, no production keys exist.
UPDATE api_keys
   SET scopes = array_replace(scopes, 'scans:read', 'history:read')
 WHERE 'scans:read' = ANY(scopes);

COMMENT ON COLUMN api_keys.scopes IS
    'Subset of ValidScopes in models/apikey: assets:read, assets:write, locations:read, locations:write, history:read, scans:write, keys:admin';
```

- [ ] **Step 3: Write the down migration**

Create `backend/migrations/000039_rename_scans_read_scope.down.sql`:

```sql
SET search_path=trakrf,public;

UPDATE api_keys
   SET scopes = array_replace(scopes, 'history:read', 'scans:read')
 WHERE 'history:read' = ANY(scopes);

COMMENT ON COLUMN api_keys.scopes IS
    'Subset of ValidScopes in models/apikey: assets:read, assets:write, locations:read, locations:write, scans:read, scans:write, keys:admin';
```

- [ ] **Step 4: Verify embed test still passes**

```bash
just backend test backend/migrations/embed_test.go ./backend/migrations/...
```
Expected: pass — the embed test loads all migration files.

If the test path above doesn't run cleanly, fall back to:
```bash
cd backend && go test ./migrations/...
```

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/000039_rename_scans_read_scope.up.sql backend/migrations/000039_rename_scans_read_scope.down.sql
git commit -m "feat(migrations): TRA-578 rename scans:read → history:read on api_keys.scopes"
```

---

## Task 6: Update integration test for the new scope name

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys_integration_test.go:459-471`

- [ ] **Step 1: Rename test and update body**

Change the test name from `TestCreateAPIKey_AcceptsScansRead` to `TestCreateAPIKey_AcceptsHistoryRead`. Update the leading comment and the JSON body. Specifically:

```go
// TestCreateAPIKey_AcceptsHistoryRead is the positive companion: history:read
// remains a valid scope after TRA-578 (renamed from scans:read).
func TestCreateAPIKey_AcceptsHistoryRead(t *testing.T) {
```

and the body literal:

```go
	body := []byte(`{"name":"tra-578-positive","scopes":["history:read"]}`)
```

(Update the `name` field to `tra-578-positive` so the audit trail in test output points at the right ticket.)

- [ ] **Step 2: Audit for any other `scans:read` literals in backend tests**

```bash
grep -rn "scans:read" backend/ --include="*.go"
```
Expected: no matches outside migration comments / generated swagger artifacts (those are regenerated in Task 9).

If matches are found, replace each with `history:read`.

- [ ] **Step 3: Run the integration test (skip if PG_URL_LOCAL not configured; rely on CI)**

```bash
just backend test-integration -run TestCreateAPIKey_AcceptsHistoryRead ./internal/handlers/orgs/...
```
Expected: pass against local Postgres. If no local Postgres, document that CI will run it and proceed.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys_integration_test.go
git commit -m "test(api-keys): TRA-578 rename TestCreateAPIKey_AcceptsScansRead to AcceptsHistoryRead"
```

---

## Task 7: Frontend type + UI rename

**Files:**
- Modify: `frontend/src/types/apiKey.ts:6,42`
- Modify: `frontend/src/components/apikeys/ScopeSelector.tsx:11,16`
- Modify: `frontend/src/components/apikeys/ScopeSelector.test.tsx`

- [ ] **Step 1: Update `Scope` union and `ALL_SCOPES`**

In `frontend/src/types/apiKey.ts`:
- Change line 6: `  | 'scans:read'` → `  | 'history:read'`
- Change line 42: `  'scans:read',` → `  'history:read',`

- [ ] **Step 2: Rename ResourceKey + RESOURCES entry**

In `frontend/src/components/apikeys/ScopeSelector.tsx`:
- Change line 11: `type ResourceKey = 'assets' | 'locations' | 'scans';` → `type ResourceKey = 'assets' | 'locations' | 'history';`
- Change line 16: `  { key: 'scans',     label: 'Scans',     hasWrite: false },` → `  { key: 'history',   label: 'History',   hasWrite: false },`

- [ ] **Step 3: Update tests**

In `frontend/src/components/apikeys/ScopeSelector.test.tsx`:

Find the test on line ~51 starting `it('emits scans:read for "Read" on Scans'`. Replace the entire test with:

```tsx
  it('emits history:read for "Read" on History', () => {
    const onChange = vi.fn();
    render(<ScopeSelector value={[]} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/history/i), { target: { value: 'read' } });
    expect(onChange).toHaveBeenCalledWith(['history:read']);
  });
```

Find the negative-write test (the one starting `it('does not offer "Read + Write" on Scans (TRA-571 — scans:write is internal-only)'`). Update its label match and comment but keep the assertion intact:

```tsx
  it('does not offer "Read + Write" on History (TRA-571 + TRA-578 — scans:write is internal-only)', () => {
    render(<ScopeSelector value={[]} onChange={() => {}} />);
    const select = screen.getByLabelText(/history/i);
    expect(within(select).getByRole('option', { name: /^none$/i })).toBeInTheDocument();
    expect(within(select).getByRole('option', { name: /^read$/i })).toBeInTheDocument();
    expect(within(select).queryByRole('option', { name: /read \+ write/i })).not.toBeInTheDocument();
  });
```

Search for any other `scans:read` / `scans` references in the test file and update consistently. Specifically, check the `preserves data scopes when toggling key management` test and any value arrays — if they pass `scans:read` literally, update to `history:read`.

```bash
grep -n "scans" frontend/src/components/apikeys/ScopeSelector.test.tsx
```

For each remaining match, decide: if it references the old scope or the old "Scans" label, update to history; if it's a comment about TRA-571 history that's no longer accurate, update or remove.

- [ ] **Step 4: Run frontend tests**

```bash
just frontend test ScopeSelector
```
Expected: all tests pass with the new history:read assertions.

- [ ] **Step 5: Audit frontend for any other scans:read literals**

```bash
grep -rn "scans:read\|'scans'" frontend/src --include="*.ts" --include="*.tsx"
```
Expected: no remaining matches. If any are found, evaluate whether they should be `history:read` / `'history'` and update.

- [ ] **Step 6: Run typecheck and lint**

```bash
just frontend lint
```
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/types/apiKey.ts frontend/src/components/apikeys/ScopeSelector.tsx frontend/src/components/apikeys/ScopeSelector.test.tsx
git commit -m "refactor(frontend): TRA-578 rename Scans→History UI label and scans:read→history:read"
```

---

## Task 8: Regenerate OpenAPI spec and verify

**Files:**
- Modify (regenerated): `backend/docs/swagger.json`, `backend/docs/swagger.yaml`, `backend/docs/docs.go`
- Modify (regenerated): `docs/api/openapi.public.json`, `docs/api/openapi.public.yaml`

- [ ] **Step 1: Regenerate**

```bash
just backend api-spec
```
Expected: success messages — public spec at `docs/api/openapi.public.{json,yaml}`, internal spec embedded.

- [ ] **Step 2: Verify api-keys removed from public spec**

```bash
grep -c '/orgs/{id}/api-keys' docs/api/openapi.public.json
```
Expected: 0.

```bash
grep -c '/orgs/{id}/api-keys' docs/api/openapi.public.yaml
```
Expected: 0.

- [ ] **Step 3: Verify scope rename in public spec**

```bash
grep -c '"scans:read"' docs/api/openapi.public.json
grep -c 'scans:read' docs/api/openapi.public.yaml
```
Expected: both 0.

```bash
grep -c '"history:read"' docs/api/openapi.public.json
grep -c 'history:read' docs/api/openapi.public.yaml
```
Expected: ≥ 2 each (current_locations + asset_history endpoints).

- [ ] **Step 4: Verify api-keys still in internal spec (embedded)**

```bash
grep -c '/orgs/{id}/api-keys' backend/internal/handlers/swaggerspec/openapi.internal.json 2>/dev/null || echo "internal spec not yet built"
```
Expected: ≥ 1 (note: this file is gitignored — generated locally only).

- [ ] **Step 5: Lint the public spec**

```bash
just backend api-lint
```
Expected: clean (no new violations beyond preexisting baseline).

- [ ] **Step 6: Commit regenerated artifacts**

```bash
git add backend/docs/swagger.json backend/docs/swagger.yaml backend/docs/docs.go docs/api/openapi.public.json docs/api/openapi.public.yaml
git commit -m "chore(api-spec): TRA-578 regenerate public OpenAPI after api-keys → internal + history:read rename"
```

---

## Task 9: Changelog entry

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Read current CHANGELOG to identify the unreleased section**

```bash
head -40 CHANGELOG.md
```
Identify the format (likely a `## Unreleased` block at the top).

- [ ] **Step 2: Add entry**

Insert under the unreleased / latest section:

```markdown
- TRA-578 Public API surface cleanup:
  - `POST/GET/DELETE /api/v1/orgs/{id}/api-keys*` removed from the public OpenAPI spec. Key minting remains browser-mediated by design (see Authentication docs). The endpoints are still implemented and used by the SPA.
  - Renamed scope `scans:read` → `history:read` to match the `/assets/{id}/history` and `/locations/current` endpoint vocabulary. Existing preview keys with `scans:read` are migrated; pre-launch hard cut (no production API keys exist).
```

Match the surrounding bullet style (the file may use `- ` or `* `; mirror what's already there).

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): TRA-578 note api-keys internal-flip and history:read rename"
```

---

## Task 10: Final verification + push + PR

- [ ] **Step 1: Re-run full validation**

```bash
just lint
```
Expected: clean across both workspaces.

```bash
just backend test
```
Expected: pass.

```bash
just frontend test
```
Expected: pass.

- [ ] **Step 2: Apply migration to preview DB and verify rewrite**

If the migration runs as part of CI/preview deploy: skip this manual step and rely on the post-deploy verification.

If running manually:

```bash
psql "$PG_URL_PREVIEW" -c "SELECT count(*) FROM trakrf.api_keys WHERE 'scans:read' = ANY(scopes);"
# Take a 'before' count.
```

After deploy / preview migration:

```bash
psql "$PG_URL_PREVIEW" -c "SELECT count(*) FROM trakrf.api_keys WHERE 'scans:read' = ANY(scopes);"
# Expected: 0
psql "$PG_URL_PREVIEW" -c "SELECT count(*) FROM trakrf.api_keys WHERE 'history:read' = ANY(scopes);"
# Expected: equal to the 'before' count.
```

- [ ] **Step 3: Push branch**

```bash
git push -u origin feat/tra-578-public-spec-cleanup-history-read
```

- [ ] **Step 4: Open PR**

Use `gh pr create` with a HEREDOC body. Title: `feat: TRA-578 drop programmatic mint from public spec, rename scans:read → history:read`. Body summarizes O-1 and C-5, lists migration / breaking-on-preview behavior, notes the docs PR is a follow-up in `trakrf/docs`.

- [ ] **Step 5: Verify preview deploy**

Wait for the auto preview deploy. Then:

1. Open `https://app.preview.trakrf.id/api` — confirm no `/orgs/{id}/api-keys` paths in the rendered Redoc, and the scope enums show `history:read`.
2. Mint a fresh preview key with `History → Read` only via the SPA avatar menu.
3. `curl -H "Authorization: Bearer $KEY" "https://app.preview.trakrf.id/api/v1/locations/current?limit=1"` → expect 200.
4. `curl -H "Authorization: Bearer $KEY" "https://app.preview.trakrf.id/api/v1/assets/<some-id>/history?limit=1"` → expect 200.

If any of these fail, do not merge — investigate.

---

## Out of scope

- `trakrf-docs` Authentication / Quickstart / private-endpoints updates — separate PR after this one merges (documented in spec § "Files touched — trakrf-docs repo").
- `keys:admin` removal from `ValidScopes` — internal mint endpoint still recognizes it for self-rotation.
- `scans:read` middleware alias — pre-launch decision is hard cut.
