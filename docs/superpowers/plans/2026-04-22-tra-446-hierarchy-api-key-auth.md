# TRA-446 — Hierarchy endpoints accept API-key auth · Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-04-22-tra-446-hierarchy-api-key-auth-design.md`](../specs/2026-04-22-tra-446-hierarchy-api-key-auth-design.md)

**Goal:** Register the three `GET /api/v1/locations/{identifier}/{ancestors,children,descendants}` routes under the public `EitherAuth` group so API-key callers with `locations:read` succeed, matching the contract already declared in the OpenAPI annotations.

**Architecture:** Bug fix, not a feature. Three route-registration lines move from the session-only group (via `locations.Handler.RegisterRoutes`) into the existing `EitherAuth` read group in `router.go`. The now-empty `RegisterRoutes` method is deleted. Test routers in the two integration-test files are consolidated into a single `buildLocationsPublicReadRouter` mirroring post-fix production, to reduce future drift risk. New negative-auth tests (403 missing-scope, 401 no-auth) fill the existing coverage gap — happy-path API-key tests already exist under a different naming convention.

**Tech Stack:** Go, chi v5, chi middleware, `//go:build integration` tests, `testify`.

**Notes on TDD shape:** The hierarchy test helper (`buildLocationsHierarchyRouter`) was already authored under `EitherAuth` + `RequireScope`, so the auth enforcement the new tests assert is already in place against the helper. The production bug is a *wiring divergence* between test helper and production router. No lightweight automated test currently exercises the production router directly (would require standing up `setupRouter` with eleven handler deps). The consolidation step reduces drift risk; final correctness is validated by inspection + manual smoke via the running server.

---

## Task 1: Consolidate test router helpers

**Files:**
- Modify: `backend/internal/handlers/locations/public_integration_test.go` (rename + expand helper, around lines 62-72)
- Modify: `backend/internal/handlers/locations/public_write_integration_test.go` (delete helper around lines 45-56; update callers)

Pure refactor. Existing tests serve as the safety net. No production code changes yet.

- [ ] **Step 1: Replace `buildLocationsPublicRouter` with `buildLocationsPublicReadRouter`**

In `backend/internal/handlers/locations/public_integration_test.go`, replace the existing helper (around lines 62-72):

```go
func buildLocationsPublicReadRouter(store *storage.Storage) *chi.Mux {
	handler := locations.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations", handler.ListLocations)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}", handler.GetLocationByIdentifier)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/ancestors", handler.GetAncestors)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/children", handler.GetChildren)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/descendants", handler.GetDescendants)
	})
	return r
}
```

- [ ] **Step 2: Update `public_integration_test.go` callers**

Search for `buildLocationsPublicRouter(` in `public_integration_test.go` and rename every call site to `buildLocationsPublicReadRouter(`. These are the existing list/by-identifier happy-path tests.

Command:

```bash
grep -l buildLocationsPublicRouter backend/internal/handlers/locations/
```

Expected: only `public_integration_test.go` (after Step 1 removes the definition, grep returns call sites). Edit each call site.

- [ ] **Step 3: Delete `buildLocationsHierarchyRouter` from `public_write_integration_test.go`**

Remove the function (around lines 45-56). Do not touch `buildLocationsPublicWriteRouter` (write-path helper — out of scope).

- [ ] **Step 4: Re-point `buildLocationsHierarchyRouter` callers to `buildLocationsPublicReadRouter`**

In `public_write_integration_test.go`, rename every `buildLocationsHierarchyRouter(store)` call to `buildLocationsPublicReadRouter(store)`. There are multiple call sites — the existing hierarchy tests (`TestLocationsGetAncestors_ByIdentifier_Works`, `TestLocationsGetChildren_ByIdentifier_Works`, `TestLocationsGetDescendants_ByIdentifier_Works`, their `UnknownIdentifier_Returns404` siblings, and the `CrossOrg_NoLeak` tests).

- [ ] **Step 5: Verify build compiles**

```bash
just backend build
```

Expected: clean build, no unresolved symbols.

- [ ] **Step 6: Run existing locations integration tests**

```bash
just backend test-integration ./internal/handlers/locations/...
```

Expected: all pre-existing tests pass. The refactor is a no-op behavior-wise.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handlers/locations/public_integration_test.go backend/internal/handlers/locations/public_write_integration_test.go
git commit -m "refactor(tra-446): consolidate locations public-read test router helper"
```

---

## Task 2: Add missing-scope 403 test for hierarchy endpoints

**Files:**
- Modify: `backend/internal/handlers/locations/public_integration_test.go` (add test at end of file)

The existing `TestLocationsGet{Ancestors,Children,Descendants}_ByIdentifier_Works` tests already cover the API-key happy path (they mint an API key with `locations:read` and expect 200). The coverage gap is negative paths: missing scope (→ 403) and no auth (→ 401). This task adds the scope test.

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/handlers/locations/public_integration_test.go`:

```go
func TestLocationsHierarchy_MissingScope_Returns403(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra446-hierarchy-missing-scope")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"assets:read"})

	// Seed a location so the handler has something to resolve if auth were to pass.
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "scope-test",
		Name:       "Scope Test",
		Path:       "scope-test",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicReadRouter(store)

	paths := []string{
		"/api/v1/locations/scope-test/ancestors",
		"/api/v1/locations/scope-test/children",
		"/api/v1/locations/scope-test/descendants",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

```bash
just backend test-integration -run TestLocationsHierarchy_MissingScope_Returns403 ./internal/handlers/locations/...
```

Expected: PASS (the consolidated helper already has `RequireScope("locations:read")` middleware; this test validates that behavior is asserted for all three hierarchy paths).

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/locations/public_integration_test.go
git commit -m "test(tra-446): assert 403 on hierarchy endpoints when scope missing"
```

---

## Task 3: Add no-auth 401 test for hierarchy endpoints

**Files:**
- Modify: `backend/internal/handlers/locations/public_integration_test.go` (add test at end of file)

- [ ] **Step 1: Write the test**

Append to `backend/internal/handlers/locations/public_integration_test.go`:

```go
func TestLocationsHierarchy_NoAuth_Returns401(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra446-hierarchy-no-auth")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	r := buildLocationsPublicReadRouter(store)

	paths := []string{
		"/api/v1/locations/anything/ancestors",
		"/api/v1/locations/anything/children",
		"/api/v1/locations/anything/descendants",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			// No Authorization header.
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

```bash
just backend test-integration -run TestLocationsHierarchy_NoAuth_Returns401 ./internal/handlers/locations/...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/locations/public_integration_test.go
git commit -m "test(tra-446): assert 401 on hierarchy endpoints when auth header missing"
```

---

## Task 4: Move hierarchy routes to the public read group in production router

**Files:**
- Modify: `backend/internal/cmd/serve/router.go` (around lines 97 and 118-134)
- Modify: `backend/internal/handlers/locations/locations.go` (remove `RegisterRoutes` method at lines 849-856)

This is the actual bug fix. No new automated test proves the production wiring change directly (see "Notes on TDD shape" above). The new tests from Tasks 2 and 3 are green in this task too; the change's correctness is verified by:
1. The existing session-auth integration tests that cover hierarchy endpoints still pass (session auth still works via `EitherAuth`).
2. Manual smoke against a running server using an API key.

- [ ] **Step 1: Add the three hierarchy routes to the `EitherAuth` read group**

In `backend/internal/cmd/serve/router.go`, find the existing public read group (around lines 118-134) and add three lines next to `ListLocations` / `GetLocationByIdentifier`. The group should look like this after the edit:

```go
// TRA-396 public read surface — accepts API-key OR session auth via EitherAuth.
r.Group(func(r chi.Router) {
	r.Use(middleware.EitherAuth(store))
	r.Use(middleware.RateLimit(rl))
	r.Use(middleware.SentryContext)

	r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets", assetsHandler.ListAssets)
	r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets/{identifier}", assetsHandler.GetAssetByIdentifier)

	r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations", locationsHandler.ListLocations)
	r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}", locationsHandler.GetLocationByIdentifier)
	r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/ancestors", locationsHandler.GetAncestors)
	r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/children", locationsHandler.GetChildren)
	r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}/descendants", locationsHandler.GetDescendants)

	// Scan-class endpoints (logical scan events, current-locations snapshot, asset movement history)
	// require scans:read per TRA-392 — they moved under /locations/ and /assets/ for URL
	// aesthetics but are scan data, not asset/location CRUD data.
	r.With(middleware.RequireScope("scans:read")).Get("/api/v1/locations/current", reportsHandler.ListCurrentLocations)
	r.With(middleware.RequireScope("scans:read")).Get("/api/v1/assets/{identifier}/history", reportsHandler.GetAssetHistory)
})
```

- [ ] **Step 2: Remove `locationsHandler.RegisterRoutes(r)` from the session-only group**

In the same file, delete the `locationsHandler.RegisterRoutes(r)` call (currently around line 97 inside the session-auth group — the group starting with `r.Use(middleware.Auth)` around line 89).

- [ ] **Step 3: Verify there are no other callers of `RegisterRoutes` on the locations handler**

```bash
grep -rn 'locationsHandler.RegisterRoutes\|locationsHandler.RegisterRoutes(' backend/
```

Expected: zero hits. If hits remain, remove them (there should only have been the one call site).

- [ ] **Step 4: Delete the `RegisterRoutes` method and its doc comment from the locations handler**

In `backend/internal/handlers/locations/locations.go`, delete lines 849-856:

```go
// RegisterRoutes keeps only session-only surface (hierarchy by-identifier). Public write
// routes are registered in internal/cmd/serve/router.go under EitherAuth +
// WriteAudit + RequireScope. Public reads likewise (per TRA-396).
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/locations/{identifier}/ancestors", handler.GetAncestors)
	r.Get("/api/v1/locations/{identifier}/descendants", handler.GetDescendants)
	r.Get("/api/v1/locations/{identifier}/children", handler.GetChildren)
}
```

- [ ] **Step 5: Check whether `chi` import in `locations.go` is still needed**

After deleting `RegisterRoutes`, `chi.Router` is no longer referenced in that function. However, the file uses `chi.URLParam` throughout its handlers. Check:

```bash
grep -n 'chi\.' backend/internal/handlers/locations/locations.go | head
```

If only `chi.URLParam` remains (no `chi.Router`), the `github.com/go-chi/chi/v5` import stays (same package). No import change expected, but verify nothing became unused.

- [ ] **Step 6: Verify build compiles**

```bash
just backend build
```

Expected: clean build.

- [ ] **Step 7: Run the full locations integration suite**

```bash
just backend test-integration ./internal/handlers/locations/...
```

Expected: all tests pass, including every pre-existing hierarchy test (which continues to exercise the consolidated helper) and the two new negative-auth tests.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/cmd/serve/router.go backend/internal/handlers/locations/locations.go
git commit -m "fix(tra-446): register hierarchy endpoints under EitherAuth public-read group"
```

---

## Task 5: Full validation & manual smoke

**Files:** None modified.

- [ ] **Step 1: Run the full validate suite**

```bash
just validate
```

Expected: lint + test green across both workspaces.

- [ ] **Step 2: Start local server for manual smoke**

(Skip if you will rely on the preview deployment. The preview URL is `https://app.preview.trakrf.id` after the PR is opened — see `.github/workflows/sync-preview.yml`.)

```bash
just backend serve
```

- [ ] **Step 3: Mint a test API key with `locations:read` scope**

Use the existing API-key management flow (UI or CLI per TRA-393). Record the bearer token as `$API_KEY`.

- [ ] **Step 4: Seed a location hierarchy and smoke each endpoint**

(Either via the UI or by pointing at seed data that already has a hierarchy.)

```bash
curl -i -H "Authorization: Bearer $API_KEY" http://localhost:8080/api/v1/locations/<known-identifier>/ancestors
curl -i -H "Authorization: Bearer $API_KEY" http://localhost:8080/api/v1/locations/<known-identifier>/children
curl -i -H "Authorization: Bearer $API_KEY" http://localhost:8080/api/v1/locations/<known-identifier>/descendants
```

Expected: each returns `HTTP/1.1 200` with `{"data":[...]}`. Each response carries rate-limit headers (`X-RateLimit-*`) since the routes are now in the rate-limited group.

- [ ] **Step 5: Negative-path smoke**

```bash
# No Authorization header → 401
curl -i http://localhost:8080/api/v1/locations/<known-identifier>/ancestors

# Token with non-locations scope → 403 (use an assets-only key)
curl -i -H "Authorization: Bearer $ASSETS_ONLY_KEY" http://localhost:8080/api/v1/locations/<known-identifier>/ancestors
```

Expected: 401 and 403 respectively.

- [ ] **Step 6: Push and open the PR**

```bash
git push -u origin miks2u/tra-446-hierarchy-endpoints-reject-api-key-auth-despite-docs
gh pr create --title "fix(tra-446): hierarchy endpoints accept API-key auth" --body "$(cat <<'EOF'
## Summary
- Moves `GET /api/v1/locations/{identifier}/{ancestors,children,descendants}` from the session-only router group into the `EitherAuth` public-read group so API keys with `locations:read` now succeed — matches the `@Security APIKey[locations:read]` annotations already in the OpenAPI spec.
- Deletes `locations.Handler.RegisterRoutes` (held only the three stragglers) and consolidates the two public-read test router helpers into one to reduce future drift.
- Adds missing negative-auth coverage (403 missing-scope, 401 no-auth) for the three hierarchy paths.

## Test plan
- [ ] `just validate` green
- [ ] Preview: API key with `locations:read` → 200 on `/ancestors`, `/children`, `/descendants`
- [ ] Preview: key without scope → 403 on same paths
- [ ] Preview: no auth → 401 on same paths
- [ ] Preview: session JWT continues to return 200 (no regression)

Closes TRA-446.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Out of scope (explicit)

- **401 error message taxonomy rework.** The ticket's wild-goose-chase path is cured by the route move. General "unsupported auth method vs invalid credential" work spans `middleware/apikey.go`, `either_auth.go`, and `httputil/auth_error.go` and deserves its own design pass.
- **Full-production-router integration test.** `setupRouter` takes 11 handler dependencies; standing it up in tests is a broader harness concern, not this ticket.
- **Moving existing hierarchy tests** out of `public_write_integration_test.go` (where they currently live despite being read tests) into a hierarchy-specific file. Pure churn for this ticket.
