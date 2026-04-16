# TRA-370 Backend `/metrics` Endpoint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose a Prometheus `/metrics` endpoint on the TrakRF Go backend, unauthenticated, alongside `/healthz` and `/readyz`, returning the default Go runtime + process collector metrics.

**Architecture:** Register the `prometheus/client_golang` `promhttp.Handler()` on the chi router *outside* the auth middleware group, after the swagger route. v1 ships only the default Go runtime + process collectors (GC, goroutines, memory, file descriptors). Custom request-scoped histograms are explicitly out of scope per the Linear ticket.

**Tech Stack:** Go 1.25, chi v5, `github.com/prometheus/client_golang` (new dep).

**Linear:** [TRA-370](https://linear.app/trakrf/issue/TRA-370) — backend portion only. Infra ServiceMonitor + Grafana dashboard live in `trakrf/infra` and will be planned/executed separately from that repo.

**Branch:** `feature/tra-370-metrics-endpoint`

**Out of scope (deferred):**
- ServiceMonitor YAML and Helm values (separate `trakrf/infra` plan)
- Grafana dashboard JSON (separate `trakrf/infra` plan)
- Custom HTTP request histograms / per-route metrics (follow-up ticket per Linear notes)

---

## File Structure

| File | Change | Responsibility |
|---|---|---|
| `backend/go.mod` | modify | Add `github.com/prometheus/client_golang` require |
| `backend/go.sum` | modify | Lock new dep + transitive deps |
| `backend/main.go` | modify (~2 lines) | Import promhttp; register `/metrics` route in `setupRouter` |
| `backend/main_test.go` | modify | Add `/metrics` to `TestRouterRegistration` table; add `TestMetricsEndpoint` for content + auth-free assertion |
| `backend/README.md` | modify | Document `/metrics` in "Endpoints" section + endpoints table |

No new files. All changes confined to `backend/` workspace.

---

## Task 1: Create branch and verify clean baseline

**Files:** none modified

- [ ] **Step 1: Verify `main` is current**

```bash
git -C /home/mike/platform fetch origin main
git -C /home/mike/platform status
```

Expected: working tree clean of staged changes (untracked spec/inventory files in repo root are unrelated and may remain). Current branch `main`.

- [ ] **Step 2: Create feature branch**

```bash
git -C /home/mike/platform checkout -b feature/tra-370-metrics-endpoint
```

Expected: `Switched to a new branch 'feature/tra-370-metrics-endpoint'`.

- [ ] **Step 3: Confirm `/metrics` does not yet exist**

```bash
just backend test -run TestRouterRegistration
```

Expected: PASS. (Sanity check that the test harness runs before we touch anything.)

---

## Task 2: Add `prometheus/client_golang` dependency

**Files:**
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`

- [ ] **Step 1: Add the dependency**

Run from project root:

```bash
just backend get github.com/prometheus/client_golang
```

If `just backend get` is not a defined recipe, fall back to:

```bash
cd backend && go get github.com/prometheus/client_golang && cd ..
```

Expected: `go: added github.com/prometheus/client_golang vX.Y.Z` (and several transitive `go: added` lines for `prometheus/common`, `prometheus/procfs`, etc.).

- [ ] **Step 2: Tidy modules**

```bash
cd backend && go mod tidy && cd ..
```

Expected: no errors. `go.mod` now contains a `github.com/prometheus/client_golang` line.

- [ ] **Step 3: Verify build still passes**

```bash
just backend build
```

If `just backend build` is not defined:

```bash
cd backend && go build ./... && cd ..
```

Expected: builds with no errors.

- [ ] **Step 4: Commit**

```bash
git add backend/go.mod backend/go.sum
git commit -m "chore(tra-370): add prometheus/client_golang dependency"
```

---

## Task 3: Write failing test for `/metrics` route registration

**Files:**
- Modify: `backend/main_test.go`

- [ ] **Step 1: Add `/metrics` to the `TestRouterRegistration` table**

Edit `backend/main_test.go`. In the `tests` table inside `TestRouterRegistration` (currently lines ~63-90), add a new entry alongside the other unauth'd endpoints (`/healthz`, `/readyz`, `/health`):

```go
{"GET", "/healthz"},
{"GET", "/readyz"},
{"GET", "/health"},
{"GET", "/metrics"},
```

Place the `{"GET", "/metrics"}` line immediately after `{"GET", "/health"},`.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd backend && go test -run TestRouterRegistration -v ./...
```

Expected: FAIL with `Route not found: GET /metrics`.

---

## Task 4: Register the `/metrics` route in `setupRouter`

**Files:**
- Modify: `backend/main.go`

- [ ] **Step 1: Add the import**

In `backend/main.go`, add the promhttp import alongside the existing imports. Place it grouped with other third-party imports (e.g., near `httpSwagger "github.com/swaggo/http-swagger"` on line 21):

```go
"github.com/prometheus/client_golang/prometheus/promhttp"
```

(Plain import, no alias. `goimports` / `gofmt` will resolve grouping.)

- [ ] **Step 2: Register the route**

In `setupRouter`, immediately after the swagger route on line 127 (`r.Get("/swagger/*", httpSwagger.WrapHandler)`) and before `healthHandler.RegisterRoutes(r)`, add:

```go
r.Handle("/metrics", promhttp.Handler())
```

The full surrounding context after the edit:

```go
r.Get("/swagger/*", httpSwagger.WrapHandler)

r.Handle("/metrics", promhttp.Handler())

healthHandler.RegisterRoutes(r)
```

This places `/metrics` outside the `r.Group(... middleware.Auth ...)` block on line 133, so it is reachable without authentication — matching the spec acceptance criterion.

- [ ] **Step 3: Run gofmt / goimports**

```bash
cd backend && gofmt -w main.go
```

- [ ] **Step 4: Run the route test to verify it now passes**

```bash
cd backend && go test -run TestRouterRegistration -v ./...
```

Expected: PASS for all entries including `GET /metrics`.

- [ ] **Step 5: Run the full router test suite**

```bash
cd backend && go test -run TestRouter -v ./...
```

Expected: `TestRouterSetup` and `TestRouterRegistration` both PASS.

---

## Task 5: Add content + auth-bypass test for `/metrics`

This test goes beyond route registration — it actually serves a request through the router and asserts (a) HTTP 200, (b) response contains Prometheus exposition-format markers, (c) no `Authorization` header is required.

**Files:**
- Modify: `backend/main_test.go`

- [ ] **Step 1: Add the test**

Append the following to `backend/main_test.go`:

```go
func TestMetricsEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics: got status %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "# HELP ") {
		t.Errorf("GET /metrics: response missing Prometheus '# HELP' marker; got first 200 bytes: %q", body[:min(200, len(body))])
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Errorf("GET /metrics: response missing default Go runtime metric 'go_goroutines'")
	}
}
```

- [ ] **Step 2: Add the required imports to `main_test.go`**

Add to the `import` block at the top of `backend/main_test.go`:

```go
"net/http"
"net/http/httptest"
"strings"
```

`min` is a Go 1.21+ builtin and the module is on Go 1.25, so no helper needed.

- [ ] **Step 3: Run the new test**

```bash
cd backend && go test -run TestMetricsEndpoint -v ./...
```

Expected: PASS. Output should mention `--- PASS: TestMetricsEndpoint`.

If FAIL with status != 200, the route was registered inside the auth group — verify Step 2 of Task 4 placed the line *before* `r.Group(...)` on line 133.

- [ ] **Step 4: Run full backend test suite**

```bash
just backend test
```

Expected: all tests pass. No regressions from the dependency addition.

- [ ] **Step 5: Commit**

```bash
git add backend/main.go backend/main_test.go
git commit -m "feat(tra-370): expose unauthenticated /metrics endpoint

Registers prometheus/client_golang promhttp.Handler at /metrics,
outside the auth middleware group. Default Go runtime + process
collectors only — request-scoped histograms are a follow-up."
```

---

## Task 6: Document `/metrics` in backend README

**Files:**
- Modify: `backend/README.md`

- [ ] **Step 1: Add `/metrics` to the directory-listing comment**

In `backend/README.md` on line 14, the file map currently reads:

```
├── health.go       # Health check handlers (/healthz, /readyz, /health)
```

There is no separate metrics file (the handler is registered inline in `main.go`), so no change to the directory listing is required. Skip this step if the structure block doesn't make sense to amend; proceed to Step 2.

- [ ] **Step 2: Add `/metrics` to the unauthenticated-endpoints prose section**

Find the section that currently lists `/healthz` and `/readyz` in prose (near line 30):

```
- **GET /healthz** - Liveness probe (returns "ok" if process alive)
- **GET /readyz** - Readiness probe (returns "ok" if ready for traffic)
```

Add immediately below:

```
- **GET /metrics** - Prometheus exposition (Go runtime + process collectors; no auth)
```

- [ ] **Step 3: Add `/metrics` to the endpoints table**

Find the Markdown table near line 50 with rows for `/healthz` and `/readyz`:

```
| GET | `/healthz` | Liveness probe | `200 OK` - Plain text "ok" |
| GET | `/readyz` | Readiness probe | `200 OK` - Plain text "ok" |
```

Add immediately below:

```
| GET | `/metrics` | Prometheus metrics | `200 OK` - Prometheus exposition format |
```

- [ ] **Step 4: Add `/metrics` to the unauthenticated-paths list**

If a list near line 379 enumerates unauthenticated paths (`Health checks (/healthz, /readyz, /health)`), extend it:

```
1. Health checks (`/healthz`, `/readyz`, `/health`)
2. Prometheus metrics (`/metrics`)
```

(Renumber subsequent items if the list is numbered; otherwise append as a bullet.)

- [ ] **Step 5: Commit**

```bash
git add backend/README.md
git commit -m "docs(tra-370): document /metrics endpoint"
```

---

## Task 7: Manual smoke test against running backend

**Files:** none modified

- [ ] **Step 1: Start the backend**

```bash
just backend run
```

Wait for `Server starting` log line.

- [ ] **Step 2: Curl the endpoint**

In a second terminal:

```bash
curl -s -i http://localhost:8080/metrics | head -40
```

Expected:
- HTTP/1.1 200 OK
- `Content-Type: text/plain; version=0.0.4; charset=utf-8` (or similar Prometheus exposition content type)
- Body contains lines like:
  - `# HELP go_goroutines Number of goroutines that currently exist.`
  - `# TYPE go_goroutines gauge`
  - `go_goroutines <number>`
  - `# HELP process_resident_memory_bytes ...`

- [ ] **Step 3: Confirm no auth required**

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/metrics
```

Expected: `200` (no Authorization header sent, no redirect, no 401).

- [ ] **Step 4: Stop the backend**

Ctrl-C in the `just backend run` terminal.

---

## Task 8: Lint and final validation

**Files:** none modified

- [ ] **Step 1: Run combined lint**

```bash
just lint
```

Expected: no errors. (If `golangci-lint` complains about the new import grouping, run `goimports -w backend/main.go backend/main_test.go` and re-stage.)

- [ ] **Step 2: Run combined test suite**

```bash
just test
```

Expected: all backend + frontend tests pass.

- [ ] **Step 3: Push branch**

```bash
git push -u origin feature/tra-370-metrics-endpoint
```

- [ ] **Step 4: Open PR**

```bash
gh pr create --title "feat(tra-370): expose Prometheus /metrics endpoint" --body "$(cat <<'EOF'
## Summary
- Registers `promhttp.Handler()` at `/metrics`, outside the auth middleware group
- Adds `github.com/prometheus/client_golang` dependency
- Default Go runtime + process collectors (GC, goroutines, memory, FDs); request-scoped histograms are a follow-up
- Documents the new endpoint in `backend/README.md`

Backend portion of [TRA-370](https://linear.app/trakrf/issue/TRA-370). The infra ServiceMonitor + Grafana dashboard land in a separate PR against `trakrf/infra`.

## Test plan
- [x] `just backend test` — `TestRouterRegistration` includes `/metrics`; new `TestMetricsEndpoint` asserts 200 + Prometheus format + auth-free
- [x] `just lint` clean
- [x] Manual: `curl http://localhost:8080/metrics` returns Prometheus exposition with `go_goroutines`, `process_resident_memory_bytes`
- [ ] Preview deploy: `curl https://app.preview.trakrf.id/metrics` returns 200 (verify after PR opens)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR URL printed. Preview deploy will trigger automatically per `.github/workflows/sync-preview.yml`.

- [ ] **Step 5: Verify preview deploy `/metrics`**

After the preview workflow finishes (check `gh pr checks <pr-number>` until green):

```bash
curl -s -o /dev/null -w "%{http_code}\n" https://app.preview.trakrf.id/metrics
```

Expected: `200`. If `404`, check that the deploy is running the new image (preview may be cached on prior SHA).

---

## Acceptance Criteria (from Linear)

| Criterion | Where verified |
|---|---|
| `curl backend:8080/metrics` returns Prometheus exposition format | Task 7 Step 2 (manual) + Task 5 (test) |
| `/metrics` is reachable without authentication | Task 5 (test) + Task 7 Step 3 (manual) |
| Prometheus targets page shows `trakrf-backend` as `UP` | Out of scope (infra plan) |
| Grafana dashboard renders with live Go runtime data | Out of scope (infra plan) |

---

## Rollback

If `/metrics` causes issues in production:

```bash
git revert <commit-sha-of-route-registration>
```

The `prometheus/client_golang` import is otherwise inert — leaving the dep in `go.mod` is harmless if the route is removed.
