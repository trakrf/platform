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
2. Remove inline `runMigrations()` from server startup so `server serve` can run under a DB role that has no DDL grants (infosec improvement — enables TRA-85).
3. Keep bare `./server` invocation backward compatible — it defaults to `serve`.
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

- Parses `os.Args[1]`. Recognized: `serve`, `migrate`.
- No arg → defaults to `serve` (backward compat — today's Dockerfile `CMD ["/server"]` keeps working).
- Unknown subcommand → prints `usage: server [serve|migrate]` to stderr, exits 2.
- `-h` / `--help` → same usage line, exits 0.
- Initializes logger once (shared by both subcommands).
- Creates root context via `signal.NotifyContext(ctx, SIGINT, SIGTERM)`.
- Each subcommand exports `Run(ctx context.Context, version string) error`. Dispatcher logs any returned error and calls `os.Exit(1)`.

**Testability**

The dispatch decision is extracted into a pure function (takes `[]string`, returns subcommand enum + err) so `main_test.go` can cover it without spawning processes. `main()` handles the exit.

### 2. Subcommand behaviors

**`server serve` (default)**

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

**`docker-compose.yaml`**

- Add `backend-migrate` service using the same backend image/build, running `./server migrate` with the same `DB_URL`, `depends_on` the db service.
- Backend service gains `depends_on: { backend-migrate: { condition: service_completed_successfully } }`.
- TimescaleDB service unchanged.

**Dev (Air hot-reload)**

- Compose overrides the dev backend service `command:` to run migrate-then-air (single shell step or small inline sequence). Dockerfile unchanged.

**`justfile`**

- Add `backend: migrate` recipe invoking `go run . migrate` (or the built binary) with local `DB_URL`.
- Root `just migrate` delegates to `just backend migrate`.
- Existing `just backend dev` unchanged — compose handles the chain.

**Tests**

- `backend/main_test.go` — coverage for dispatch (table-driven: `[]`, `[serve]`, `[migrate]`, `[-h]`, `[--help]`, `[bogus]`).
- `internal/cmd/migrate/migrate_test.go` — integration: run migrate against a test DB, assert `schema_migrations.version` matches highest embedded migration, second run returns nil (idempotent).
- `internal/cmd/serve/serve_test.go` — port whatever exists in today's `backend/main_test.go` that tests router/handler wiring. No new serve coverage added in this PR.
- Any test relying on the old inline-migrate-on-serve-startup behavior gets a `TestMain` that runs `migrate.Run` against the test DB first. Grep during implementation.

**What stays unchanged**

- Root `Dockerfile` — `CMD ["/server"]` still works (bare invocation defaults to serve).
- `backend/Dockerfile` production stage — `CMD ["./server"]` still works.
- `railway.json` — untouched (Railway split handled under TRA-85).
- `go.mod` — no new deps. `embed`, `golang-migrate`, `iofs` move packages, nothing added.

### 5. Migration embedding

Go's `//go:embed` directive cannot reference paths outside the package's own subtree (no `..`). Two placement options:

- **A.** Move the migrations directory from `backend/migrations/` into the migrate package: `backend/internal/cmd/migrate/migrations/*.sql`. Minimal glue code; migrations become less discoverable at the top level.
- **B.** Create a thin `backend/migrations/embed.go` that does the embed and exports `FS embed.FS`. The migrate package imports it: `import "github.com/trakrf/platform/backend/migrations"` and uses `migrations.FS`. Migration files stay at `backend/migrations/` (current discoverable location).

This spec picks **B**. Rationale: the `backend/migrations/` directory is the conventional location humans look for when adding new migrations, and it's the path referenced in docs and in the infra repo's `just sync-migrations`. Option A would require updating those references; B doesn't. Cost is a one-file package with a single `//go:embed *.sql\nvar FS embed.FS` declaration.

## Risks and rollback

- Rollback is clean — schema is unchanged by this PR (no new migrations). Redeploy the previous backend image to revert.
- First prod rollout: verify the k8s migration Job completes before the backend Deployment rolls. Existing chart ordering handles this; confirm during implementation.
- Railway preview will break after merge if the preview start command isn't updated. TRA-85 must land concurrently with or before this PR reaches Railway.

## Success criteria

- `./server migrate` applies embedded migrations and exits 0.
- `./server migrate` against a current schema logs "no pending" and exits 0.
- `./server serve` starts successfully with a DB role that has only CRUD grants.
- `./server` (no args) still runs serve — zero breakage for current deploys.
- `./server bogus` prints usage and exits 2.
- Compose + `just test` flows run end-to-end without manual migration steps.
- Infra team can flip `migrate.image` in `helm/trakrf-backend` to the backend image with command `["./server", "migrate"]` after this ships.

## Sequencing

- TRA-370 (/metrics endpoint) — **merged** to main as of 2026-04-16 (commit `512d177`). This spec is cut from that state.
- TRA-367 implementation — this PR.
- TRA-85 follow-up — DB user split + Railway preview start-command split. Must land concurrently with or before TRA-367 reaches Railway.
- Infra repo follow-up — after TRA-367 merges and the new backend image is published.
