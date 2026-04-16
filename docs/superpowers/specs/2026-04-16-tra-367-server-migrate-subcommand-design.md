# TRA-367: `server migrate` subcommand

**Linear:** [TRA-367](https://linear.app/trakrf/issue/TRA-367)
**Parent epic:** [TRA-373](https://linear.app/trakrf/issue/TRA-373) (M1.5 platform hardening)
**Status:** Spec — pending plan
**Date:** 2026-04-16

## Context

`helm/trakrf-backend` currently runs schema migrations as a Kubernetes Job using the `migrate/migrate:v4.17.0` image and a ConfigMap built from files synced via `just sync-migrations`. That works but forces migrations to live in two places (platform repo + chart `migrations/` dir) and requires manual sync before any chart change that picks up new migrations.

The backend image ships only a single `server` binary. Migrations are already `//go:embed`'d into that binary via `//go:embed migrations/*.sql` in `backend/main.go` and applied inline during `server` startup. There is no `migrate` CLI in the runtime image and no on-disk migration files.

TRA-367 adds a `migrate` subcommand to the `server` binary so the Job can run `./server migrate` against the backend image. That eliminates the dual-source-of-truth for migration files and makes the embedded set the single authoritative source.

The decision to use a subcommand rather than shipping the upstream `migrate` CLI in the image was captured on the TRA-367 Linear issue (2026-04-16): embedded files are already the source of truth, single-binary-with-subcommands is the modern Go service idiom, and there is no version-skew risk between a vendored CLI and the embedded set.

## Goals

1. Ship a `server migrate` subcommand that applies embedded migrations and exits.
2. Ship a `server serve` subcommand that runs the HTTP server without performing any migrations — enabling a DB role with no DDL grants (infosec improvement; enables TRA-85).
3. Preserve today's behavior for bare `./server` invocation (runs migrations inline, then serves) — zero breakage for existing deploys, Dockerfiles, compose, Air, or tests that rely on the combined behavior.
4. Keep the change small and focused. Defer `migrate status`, `down`, `force`, and `goto` to a follow-up if a real need surfaces.

## Non-goals

- Railway preview environment migration split → covered by [TRA-85](https://linear.app/trakrf/issue/TRA-85).
- Full `golang-migrate` CLI surface (status / force / down / goto) → follow-up if needed.
- Structured CLI framework (cobra, urfave/cli) → two subcommands don't justify it.
- Schema-naming refactor → [TRA-278](https://linear.app/trakrf/issue/TRA-278).
- Infra repo follow-up (flip `migrate.image`, delete the `migrations/` ConfigMap, remove `just sync-migrations`) → separate PR in infra repo after this ships.

## Design

### 1. Binary structure & dispatch

**Package layout**

```
backend/
  main.go                         # ~30 line dispatcher
  internal/cmd/
    serve/serve.go                # Run(ctx, version) - HTTP server startup (no migrations)
    serve/router.go               # setupRouter
    migrate/migrate.go            # Run(ctx, version) - applies embedded migrations, exits
```

**Dispatcher (`main.go`)**

- Parses `os.Args[1]`. Recognized explicit subcommands: `serve`, `migrate`.
- **No arg** → runs `migrate.Run` and then, if it returns nil, runs `serve.Run` in the same process. Preserves today's bare `./server` behavior for local dev, compose, Air, legacy Dockerfiles, and any test relying on inline-migrate-on-startup.
- Unknown subcommand → prints `usage: server [serve|migrate]` to stderr, exits 2.
- `-h` / `--help` → same usage line, exits 0.
- Initializes logger once (shared across subcommands).
- Creates root context via `signal.NotifyContext(ctx, SIGINT, SIGTERM)`.
- Each subcommand exports `Run(ctx context.Context, version string) error`. Dispatcher logs any returned error and calls `os.Exit(1)`. If migrate fails during the bare default path, serve does not start.

The combined-default behavior is a transitional convenience. TRA-85 retires it once every deployment target explicitly invokes `serve` or `migrate` with a role-matched `DB_URL`.

**Testability**

The dispatch decision is extracted into a pure function (takes `[]string`, returns subcommand enum + err) so `main_test.go` can cover it without spawning processes. `main()` handles the exit.

### 2. Subcommand behaviors

**`server` (no arg — combined default, transitional)**

- Runs `migrate.Run` then `serve.Run` in the same process. Matches today's `./server` behavior exactly.
- Requires a DB role with DDL grants (same as today).
- Intended for local dev, compose, Air, and any existing deployment target that hasn't yet been migrated to the explicit-subcommand model.
- Retired by TRA-85.

**`server serve`**

- Current `main()` body minus the `runMigrations()` call.
- Initializes Sentry, storage, services, handlers, router, HTTP server, signal-driven graceful shutdown — unchanged.
- Assumes schema is current. A stale-schema query fails at first DB access and surfaces via existing Sentry + health check.
- Does not import `embed`, `golang-migrate`, or `iofs`.
- Connects using the existing `DB_URL` env var. Operator points that at the non-DDL runtime role.

**`server migrate`**

- Creates its own minimal `pgxpool.Pool` (no storage wrapper, no services, no handlers).
- Connects using the same `DB_URL` env var. Operator points that at the DDL migration role for this invocation.
- Runs `m.Up()` on embedded migrations.
- On success: logs `{version, dirty: false}`, exits 0.
- On `migrate.ErrNoChange`: logs "no pending migrations", exits 0.
- On error: logs wrapped error, returns err (dispatcher exits 1).
- Single-shot batch: no server, no long-lived signal loop. Does respect context cancellation between phases (pool open, migrator create, `m.Up()`).

**Exit codes**

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Runtime error (migration failure, DB unavailable, serve crash) |
| 2 | Usage error (unknown subcommand) |

### 3. Logging, error handling, observability

- Logger init (`logger.NewConfig(version)` + `logger.Initialize`) moves to the dispatcher — both subcommands get consistent structured logging.
- Sentry init moves to serve only. Migrate is short-lived; failures surface via exit code + stderr + Job backoff. Skipping Sentry init saves a network roundtrip per migrate invocation.
- Subcommands return errors; dispatcher is the single exit point.
- No panics at the top level. Recovery middleware stays in serve's router (unchanged).
- `/metrics` endpoint (TRA-370) stays in serve, carried through `setupRouter` move intact. Migrate exposes no endpoints.

### 4. Ripple changes outside the binary

The combined-default behavior preserves today's contract for bare `./server`, so most of the surrounding system keeps working untouched.

**`justfile`**

- Add a `backend: migrate` convenience recipe that invokes `go run . migrate` with the local `DB_URL`. Useful for running migrate standalone during dev (e.g. after pulling new migrations without restarting the server).
- Root `just migrate` delegates to `just backend migrate`.

**Tests**

- `backend/main_test.go` — coverage for dispatch (table-driven: `[]` → combined, `["serve"]` → serve, `["migrate"]` → migrate, `["-h"]` / `["--help"]` → usage, `["bogus"]` → error).
- `internal/cmd/migrate/migrate_test.go` — integration: run migrate against a test DB, assert `schema_migrations.version` matches highest embedded migration, second run returns nil (idempotent).
- `internal/cmd/serve/serve_test.go` — port whatever exists in today's `backend/main_test.go` that tests router/handler wiring. No new serve coverage added in this PR.
- No ripple test changes expected — the combined-default path preserves inline-migrate-on-startup for any test that depends on it.

**What stays unchanged**

- Root `Dockerfile` — `CMD ["/server"]` still works (bare invocation = combined default = today's behavior).
- `backend/Dockerfile` production + development stages — bare invocation and Air hot-reload both keep working unchanged.
- `docker-compose.yaml` — backend service runs bare `./server`, which still migrates and serves. No new `backend-migrate` service needed.
- `railway.json` — untouched. Railway start command runs the combined default until TRA-85 flips it.
- `go.mod` — no new deps. `embed`, `golang-migrate`, `iofs` move packages, nothing added.

### 5. Migration embedding

Go's `//go:embed` directive cannot reference paths outside the package's own subtree (no `..`). Two placement options:

- **A.** Move the migrations directory from `backend/migrations/` into the migrate package: `backend/internal/cmd/migrate/migrations/*.sql`. Minimal glue code; migrations become less discoverable at the top level.
- **B.** Create a thin `backend/migrations/embed.go` that does the embed and exports `FS embed.FS`. The migrate package imports it: `import "github.com/trakrf/platform/backend/migrations"` and uses `migrations.FS`. Migration files stay at `backend/migrations/` (current discoverable location).

This spec picks **B**. Rationale: the `backend/migrations/` directory is the conventional location humans look for when adding new migrations, and it's the path referenced in docs and in the infra repo's `just sync-migrations`. Option A would require updating those references; B doesn't. Cost is a one-file package with a single `//go:embed *.sql\nvar FS embed.FS` declaration.

## Risks and rollback

- Rollback is clean — schema is unchanged by this PR (no new migrations). Redeploy the previous backend image to revert.
- First prod rollout: verify the k8s migration Job completes before the backend Deployment rolls. Existing chart ordering handles this; confirm during implementation.
- Railway preview is **not** affected by this PR — bare `./server` still runs migrate + serve. Railway's start command split moves under TRA-85.

## Success criteria

- `./server migrate` applies embedded migrations and exits 0.
- `./server migrate` against a current schema logs "no pending" and exits 0.
- `./server serve` starts successfully against a DB role with only CRUD grants (no DDL).
- `./server` (no args) runs migrate + serve exactly as today — zero breakage for existing deploys, Dockerfiles, compose, Air, or tests.
- `./server bogus` prints usage and exits 2.
- `just test` and `docker compose up` work end-to-end with no new setup steps.
- Infra team can flip `migrate.image` in `helm/trakrf-backend` to the backend image with command `["./server", "migrate"]` after this ships.

## Sequencing

- TRA-370 (/metrics endpoint) — **merged** to main as of 2026-04-16 (commit `512d177`). This spec is cut from that state.
- TRA-367 implementation — this PR.
- [TRA-85](https://linear.app/trakrf/issue/TRA-85) follow-up — DB user split + flip every deployment target (k8s prod, Railway preview, compose, Air) to explicit `migrate` / `serve` invocations, then retire the combined default. Not required for TRA-367 to ship.
- Infra repo follow-up — after TRA-367 merges and the new backend image is published.
