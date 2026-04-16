# TRA-367 `server migrate` Subcommand Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the `server` binary into `serve` and `migrate` subcommands so the k8s migration Job can run against the backend image directly, while preserving today's bare `./server` behavior (migrate + serve) for local dev and existing deploys.

**Architecture:** Thin dispatcher in `backend/main.go` (package `main`) parses `os.Args[1]` and delegates to `internal/cmd/serve` or `internal/cmd/migrate` packages. Migrations live in a new thin `backend/migrations` package that exports `FS embed.FS`. The existing `//go:embed frontend/dist` stays in package `main` (can't reach into subpackages) and is passed into `serve.Run` as `fs.FS`.

**Tech Stack:** Go 1.25, `golang-migrate/migrate/v4`, `jackc/pgx/v5/pgxpool`, `go-chi/chi/v5`, `rs/zerolog`, embedded files via `embed`.

**Spec:** [docs/superpowers/specs/2026-04-16-tra-367-server-migrate-subcommand-design.md](../specs/2026-04-16-tra-367-server-migrate-subcommand-design.md)

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `backend/migrations/embed.go` | CREATE | Package `migrations`: exports `FS embed.FS` via `//go:embed *.sql`. Decouples migration file location from whoever consumes them. |
| `backend/internal/cmd/migrate/migrate.go` | CREATE | Package `migrate`: exports `Run(ctx, version) error`. Opens a `pgxpool`, wires golang-migrate against `migrations.FS`, runs `m.Up()`, logs result. |
| `backend/internal/cmd/migrate/migrate_test.go` | CREATE | Unit tests for `Run`'s error paths (missing `PG_URL`). Full integration covered by the end-to-end smoke step. |
| `backend/internal/cmd/serve/router.go` | CREATE | Package `serve`: `SetupRouter(...)` — moves today's `setupRouter` from `main.go` verbatim (imports change from `main` → `serve`). |
| `backend/internal/cmd/serve/serve.go` | CREATE | Package `serve`: exports `Run(ctx, version string, frontendFS fs.FS) error`. Moves today's `main()` body minus `runMigrations()`. |
| `backend/internal/cmd/serve/serve_test.go` | CREATE | Router-setup / route-registration / metrics tests moved from `backend/main_test.go`. |
| `backend/main.go` | REWRITE | Package `main`: thin dispatcher. Keeps the `//go:embed frontend/dist` directive. Parses args, wires logger + signal context, dispatches to `migrate.Run` and/or `serve.Run`. |
| `backend/main_test.go` | REWRITE | Table-driven test for dispatch arg parsing. |
| `backend/justfile` | MODIFY | Rewrite `migrate` recipe to invoke the new binary (`go run . migrate`) rather than the `migrate` CLI. Leave `migrate-down` / `migrate-status` / `migrate-force` / `migrate-create` untouched (dev ergonomics; TRA-85 cleanup). |

---

## Conventions used in this plan

- **Working directory:** run `go` / `just` commands from `backend/` (or via `just backend <recipe>` from repo root). Paths in file blocks are repo-relative.
- **Tests run with:** `cd backend && go test -v ./...` (or `just backend test`).
- **Commits:** conventional commit format (`feat(tra-367): ...`, `refactor(tra-367): ...`, `test(tra-367): ...`).

---

## Task 1: Create `backend/migrations` package that exports the embedded FS

**Files:**
- Create: `backend/migrations/embed.go`
- Create: `backend/migrations/embed_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/migrations/embed_test.go`:

```go
package migrations

import (
	"io/fs"
	"strings"
	"testing"
)

func TestFSContainsMigrations(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatalf("fs.ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one migration file, got 0")
	}

	var upCount int
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upCount++
		}
	}
	if upCount == 0 {
		t.Fatalf("expected at least one *.up.sql file among %d entries", len(entries))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd backend && go test ./migrations/...
```

Expected: FAIL — `backend/migrations` is not yet a Go package (`build failed: no Go files in ...`).

- [ ] **Step 3: Create the package file**

Create `backend/migrations/embed.go`:

```go
// Package migrations holds the versioned SQL migration files for the TrakRF
// platform backend. The files are embedded into the binary at build time so
// they travel with whichever binary consumes them (both server startup and
// the standalone `server migrate` subcommand).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 4: Run the test to verify it passes**

Run:
```bash
cd backend && go test ./migrations/...
```

Expected: PASS. Output includes `TestFSContainsMigrations` with `ok`.

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/embed.go backend/migrations/embed_test.go
git commit -m "feat(tra-367): add backend/migrations package exporting embedded FS

Decouples migration-file ownership from main.go so both the future
migrate subcommand and the transitional combined-default path can
consume the same embedded set without duplication."
```

---

## Task 2: Create `internal/cmd/migrate` package with `Run`

**Files:**
- Create: `backend/internal/cmd/migrate/migrate.go`
- Create: `backend/internal/cmd/migrate/migrate_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/cmd/migrate/migrate_test.go`:

```go
package migrate

import (
	"context"
	"strings"
	"testing"
)

func TestRun_MissingPGURL(t *testing.T) {
	t.Setenv("PG_URL", "")

	err := Run(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error when PG_URL is empty, got nil")
	}
	if !strings.Contains(err.Error(), "PG_URL") {
		t.Errorf("expected error mentioning PG_URL, got: %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd backend && go test ./internal/cmd/migrate/...
```

Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement `migrate.Run`**

Create `backend/internal/cmd/migrate/migrate.go`:

```go
// Package migrate runs embedded database migrations as a one-shot command.
// It opens its own pgxpool using PG_URL, applies pending migrations via
// golang-migrate, logs the result, and returns. It does not start an HTTP
// server or any long-running goroutines.
package migrate

import (
	"context"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/migrations"
)

// Run applies all pending embedded migrations to the database identified
// by the PG_URL environment variable, then returns. A nil return means
// success (including the "no pending migrations" case).
func Run(ctx context.Context, version string) error {
	log := logger.Get()

	pgURL := os.Getenv("PG_URL")
	if pgURL == "" {
		return fmt.Errorf("PG_URL environment variable not set")
	}

	config, err := pgxpool.ParseConfig(pgURL)
	if err != nil {
		return fmt.Errorf("failed to parse PG_URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	log.Info().Str("version", version).Msg("Starting migrations")

	err = m.Up()
	switch err {
	case nil:
		migrationVersion, dirty, _ := m.Version()
		log.Info().Uint("version", migrationVersion).Bool("dirty", dirty).Msg("Migrations complete")
		return nil
	case migrate.ErrNoChange:
		log.Info().Msg("No pending migrations")
		return nil
	default:
		return fmt.Errorf("migration failed: %w", err)
	}
}
```

Note the `iofs.New(migrations.FS, ".")` — because the exported FS is rooted at the `migrations` package directory, the source path is `.` (not `"migrations"` as in today's `main.go`).

- [ ] **Step 4: Run the test to verify it passes**

Run:
```bash
cd backend && go test ./internal/cmd/migrate/...
```

Expected: PASS. `TestRun_MissingPGURL` should pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/cmd/migrate/migrate.go backend/internal/cmd/migrate/migrate_test.go
git commit -m "feat(tra-367): add migrate subcommand package with Run

One-shot batch that opens its own pgxpool, wires golang-migrate
against the embedded migrations FS, and applies pending migrations.
Logs and returns cleanly on success, ErrNoChange, or failure."
```

---

## Task 3: Extract `setupRouter` into `internal/cmd/serve/router.go`

**Files:**
- Create: `backend/internal/cmd/serve/router.go`

This task is a pure mechanical move — no behavior change. Tests get added/moved in Task 5.

- [ ] **Step 1: Create `router.go` with the moved function**

Create `backend/internal/cmd/serve/router.go`. Copy the current `setupRouter` function from `backend/main.go` (lines 98–160) verbatim into this new file, then change:
- Package declaration from `main` to `serve`
- Function name from `setupRouter` (unexported) to `SetupRouter` (exported) so `main` can call it later if needed — actually we won't call it from main directly; `serve.Run` calls it internally. Keep it unexported.

```go
// Package serve runs the long-lived HTTP server process. It wires storage,
// services, handlers, middleware, and graceful shutdown. It does not perform
// schema migrations — those are the responsibility of the migrate subcommand
// (or the transitional combined default in main.go).
package serve

import (
	"net/http"
	"os"

	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	lookuphandler "github.com/trakrf/platform/backend/internal/handlers/lookup"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
	testhandler "github.com/trakrf/platform/backend/internal/handlers/testhandler"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/storage"
)

func setupRouter(
	authHandler *authhandler.Handler,
	orgsHandler *orgshandler.Handler,
	usersHandler *usershandler.Handler,
	assetsHandler *assetshandler.Handler,
	locationsHandler *locationshandler.Handler,
	inventoryHandler *inventoryhandler.Handler,
	reportsHandler *reportshandler.Handler,
	lookupHandler *lookuphandler.Handler,
	healthHandler *healthhandler.Handler,
	frontendHandler *frontendhandler.Handler,
	testHandler *testhandler.Handler,
	store *storage.Storage,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(logger.Middleware)
	r.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle)
	r.Use(middleware.Recovery)
	r.Use(middleware.CORS)
	r.Use(middleware.ContentType)

	r.Handle("/assets/*", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/favicon.ico", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/icon-*", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/logo.png", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/manifest.json", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/og-image.png", http.HandlerFunc(frontendHandler.ServeFrontend))

	r.Get("/swagger/*", httpSwagger.WrapHandler)

	r.Handle("/metrics", promhttp.Handler())

	healthHandler.RegisterRoutes(r)

	authHandler.RegisterRoutes(r, middleware.Auth)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.SentryContext)

		orgsHandler.RegisterRoutes(r, store)
		orgsHandler.RegisterMeRoutes(r)
		usersHandler.RegisterRoutes(r)
		assetsHandler.RegisterRoutes(r)
		locationsHandler.RegisterRoutes(r)
		inventoryHandler.RegisterRoutes(r)
		reportsHandler.RegisterRoutes(r)
		lookupHandler.RegisterRoutes(r)
	})

	if os.Getenv("APP_ENV") != "production" {
		testHandler.RegisterRoutes(r)
	}

	r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		frontendHandler.ServeSPA(w, r, "frontend/dist/index.html")
	})

	return r
}
```

**Do not delete `setupRouter` from `backend/main.go` yet** — that happens in Task 5 when the dispatcher lands. Having both temporarily is fine; the old `main.go` still compiles and `go build` still produces a working binary.

- [ ] **Step 2: Verify the new file compiles**

Run:
```bash
cd backend && go build ./internal/cmd/serve/...
```

Expected: no output, exit 0.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/cmd/serve/router.go
git commit -m "refactor(tra-367): copy setupRouter into new serve package

Mechanical copy, no behavior change. main.go still has its own copy
until Task 5 swaps in the dispatcher. Keeps each commit buildable."
```

---

## Task 4: Create `serve.Run` in `internal/cmd/serve/serve.go`

**Files:**
- Create: `backend/internal/cmd/serve/serve.go`

- [ ] **Step 1: Create `serve.go`**

Create `backend/internal/cmd/serve/serve.go`:

```go
package serve

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5/pgxpool"

	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	lookuphandler "github.com/trakrf/platform/backend/internal/handlers/lookup"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
	testhandler "github.com/trakrf/platform/backend/internal/handlers/testhandler"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	"github.com/trakrf/platform/backend/internal/logger"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/services/email"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/storage"
)

// Run starts the long-lived HTTP server process. It blocks until ctx is
// canceled (SIGINT / SIGTERM), then performs a graceful shutdown.
//
// frontendFS is the embedded React bundle. The dispatcher owns the go:embed
// directive because its path (frontend/dist) cannot be reached from this
// package's subtree.
func Run(ctx context.Context, version string, frontendFS fs.FS) error {
	startTime := time.Now()
	log := logger.Get()

	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:           dsn,
			Environment:   os.Getenv("APP_ENV"),
			Release:       version,
			EnableTracing: false,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Sentry initialization failed")
		} else {
			log.Info().Msg("Sentry initialized")
		}
	}
	defer sentry.Flush(2 * time.Second)

	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}

	store, err := storage.New(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize storage")
		return err
	}
	defer store.Close()
	log.Info().Msg("Storage initialized")

	emailClient := email.NewClient()
	authSvc := authservice.NewService(store.Pool().(*pgxpool.Pool), store, emailClient)
	orgsSvc := orgsservice.NewService(store.Pool().(*pgxpool.Pool), store, emailClient)
	log.Info().Msg("Services initialized")

	authHandler := authhandler.NewHandler(authSvc)
	orgsHandler := orgshandler.NewHandler(store, orgsSvc)
	usersHandler := usershandler.NewHandler(store)
	assetsHandler := assetshandler.NewHandler(store)
	locationsHandler := locationshandler.NewHandler(store)
	inventoryHandler := inventoryhandler.NewHandler(store)
	reportsHandler := reportshandler.NewHandler(store)
	lookupHandler := lookuphandler.NewHandler(store)
	healthHandler := healthhandler.NewHandler(store.Pool().(*pgxpool.Pool), version, startTime)
	frontendHandler := frontendhandler.NewHandler(frontendFS, "frontend/dist")
	testHandler := testhandler.NewHandler(store)
	log.Info().Msg("Handlers initialized")

	r := setupRouter(authHandler, orgsHandler, usersHandler, assetsHandler, locationsHandler, inventoryHandler, reportsHandler, lookupHandler, healthHandler, frontendHandler, testHandler, store)
	log.Info().Msg("Routes registered")

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info().Str("port", port).Str("version", version).Msg("Server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			log.Error().Err(err).Msg("Server failed")
			return err
		}
	case <-ctx.Done():
	}

	log.Info().Msg("Shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Shutdown error")
		return err
	}

	log.Info().Msg("Server stopped")
	return nil
}
```

Note the behavior change vs. today's `main()`:
- Uses the passed-in `ctx` for shutdown signaling instead of setting up its own `signal.Notify`. The dispatcher owns signal registration.
- Returns errors rather than `os.Exit`-ing. Dispatcher owns the exit code.
- Does not call `logger.Initialize` — dispatcher owns logger setup.
- `frontendHandler.NewHandler` now takes `fs.FS` interface instead of `embed.FS`. Verify in Step 2 below that this signature already matches; if not, either widen the handler signature or keep `embed.FS` throughout.

- [ ] **Step 2: Verify `frontendhandler.NewHandler` accepts `fs.FS`**

Run:
```bash
cd backend && grep -n "func NewHandler" internal/handlers/frontend/*.go
```

If the current signature is `NewHandler(fs embed.FS, distPath string)`, the compile in Step 3 will fail. Fix by updating `internal/handlers/frontend/frontend.go` to accept `fs.FS` (the interface) instead of `embed.FS` (the concrete type):

```go
// before
func NewHandler(fs embed.FS, distPath string) *Handler
// after
func NewHandler(fs fs.FS, distPath string) *Handler
```

Import `"io/fs"` and drop the `"embed"` import if no longer needed. Check every call site (likely just `main.go` and the tests) — `embed.FS` already implements `fs.FS`, so existing callers keep working.

- [ ] **Step 3: Verify the package compiles**

Run:
```bash
cd backend && go build ./internal/cmd/serve/...
```

Expected: no output, exit 0. If `NewHandler` signature needed a tweak in Step 2, also verify:

```bash
cd backend && go build ./...
```

Expected: no output, exit 0 (existing `main.go` still compiles because `embed.FS` satisfies `fs.FS`).

- [ ] **Step 4: Commit**

```bash
git add backend/internal/cmd/serve/serve.go backend/internal/handlers/frontend/
git commit -m "feat(tra-367): add serve.Run in new serve package

Extracts today's main() body (minus runMigrations) into serve.Run,
which takes ctx + version + frontendFS and returns error. Dispatcher
owns signal handling, logger init, and exit codes in the next task.
If NewHandler's signature needed widening to fs.FS, include that."
```

---

## Task 5: Replace `main.go` with dispatcher and move router tests

**Files:**
- Rewrite: `backend/main.go`
- Rewrite: `backend/main_test.go`
- Create: `backend/internal/cmd/serve/serve_test.go`

- [ ] **Step 1: Write the failing dispatcher test**

Rewrite `backend/main_test.go` to remove router tests (they move to serve_test.go in Step 4) and add a table-driven test for argument parsing.

```go
package main

import (
	"testing"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    command
		wantErr bool
	}{
		{"no args -> combined default", []string{}, cmdCombined, false},
		{"serve explicit", []string{"serve"}, cmdServe, false},
		{"migrate explicit", []string{"migrate"}, cmdMigrate, false},
		{"-h prints usage", []string{"-h"}, cmdHelp, false},
		{"--help prints usage", []string{"--help"}, cmdHelp, false},
		{"unknown subcommand is an error", []string{"bogus"}, cmdUnknown, true},
		{"extra args after serve is an error", []string{"serve", "extra"}, cmdUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCommand(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseCommand(%v) err = %v, wantErr = %v", tt.args, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseCommand(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd backend && go test -run TestParseCommand .
```

Expected: FAIL with "undefined: command / cmdCombined / parseCommand".

- [ ] **Step 3: Rewrite `backend/main.go` as the dispatcher**

Replace the entire contents of `backend/main.go` with:

```go
package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/trakrf/platform/backend/internal/cmd/migrate"
	"github.com/trakrf/platform/backend/internal/cmd/serve"
	"github.com/trakrf/platform/backend/internal/logger"
)

//go:embed frontend/dist
var frontendFS embed.FS

var version = "dev"

type command int

const (
	cmdCombined command = iota // no arg: migrate then serve
	cmdServe
	cmdMigrate
	cmdHelp
	cmdUnknown
)

const usage = "usage: server [serve|migrate]"

func parseCommand(args []string) (command, error) {
	if len(args) == 0 {
		return cmdCombined, nil
	}
	if len(args) > 1 {
		return cmdUnknown, fmt.Errorf("unexpected extra arguments: %v", args[1:])
	}
	switch args[0] {
	case "serve":
		return cmdServe, nil
	case "migrate":
		return cmdMigrate, nil
	case "-h", "--help":
		return cmdHelp, nil
	default:
		return cmdUnknown, fmt.Errorf("unknown subcommand: %q", args[0])
	}
}

func main() {
	cmd, err := parseCommand(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if cmd == cmdHelp {
		fmt.Println(usage)
		os.Exit(0)
	}

	loggerCfg := logger.NewConfig(version)
	logger.Initialize(loggerCfg)
	log := logger.Get()
	log.Info().Msg("Logger initialized")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runErr := run(ctx, cmd)
	if runErr != nil {
		log.Error().Err(runErr).Msg("Command failed")
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd command) error {
	switch cmd {
	case cmdMigrate:
		return migrate.Run(ctx, version)
	case cmdServe:
		return serve.Run(ctx, version, frontendFS)
	case cmdCombined:
		if err := migrate.Run(ctx, version); err != nil {
			return err
		}
		return serve.Run(ctx, version, frontendFS)
	}
	return fmt.Errorf("unreachable command: %v", cmd)
}
```

**Remove** the old `runMigrations`, `setupRouter`, and `migrationsFS` declarations — they live in their new packages now. Keep `//go:embed frontend/dist` and `frontendFS` here (they can't move).

- [ ] **Step 4: Move router tests to `serve_test.go`**

Create `backend/internal/cmd/serve/serve_test.go` by moving the body of today's `backend/main_test.go` (the three tests: `TestRouterSetup`, `TestRouterRegistration`, `TestMetricsEndpoint`, plus `setupTestRouter`). Adjust:

- Package declaration: `package serve`.
- Remove the `frontendFS` reference; the tests don't actually need a real FS — the frontend handler just serves whatever FS it was given. Pass `nil` or an empty `fstest.MapFS{}` from `"testing/fstest"`.
- Call `setupRouter(...)` directly (same package) rather than through any wrapper.

```go
package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"
	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	lookuphandler "github.com/trakrf/platform/backend/internal/handlers/lookup"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
	testhandler "github.com/trakrf/platform/backend/internal/handlers/testhandler"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/storage"
)

func setupTestRouter(t *testing.T) *chi.Mux {
	t.Helper()

	store := &storage.Storage{}
	authSvc := authservice.NewService(nil, store, nil)
	orgsSvc := orgsservice.NewService(nil, store, nil)

	authHandler := authhandler.NewHandler(authSvc)
	orgsHandler := orgshandler.NewHandler(store, orgsSvc)
	usersHandler := usershandler.NewHandler(store)
	assetsHandler := assetshandler.NewHandler(store)
	locationsHandler := locationshandler.NewHandler(store)
	inventoryHandler := inventoryhandler.NewHandler(store)
	reportsHandler := reportshandler.NewHandler(store)
	lookupHandler := lookuphandler.NewHandler(store)
	healthHandler := healthhandler.NewHandler(nil, "test", time.Now())
	frontendHandler := frontendhandler.NewHandler(fstest.MapFS{}, "frontend/dist")
	testHandler := testhandler.NewHandler(store)

	return setupRouter(authHandler, orgsHandler, usersHandler, assetsHandler, locationsHandler, inventoryHandler, reportsHandler, lookupHandler, healthHandler, frontendHandler, testHandler, store)
}

func TestRouterSetup(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("setupRouter panicked: %v", r)
		}
	}()

	r := setupTestRouter(t)

	if r == nil {
		t.Fatal("setupRouter returned nil")
	}
}

func TestRouterRegistration(t *testing.T) {
	r := setupTestRouter(t)

	tests := []struct {
		method string
		path   string
	}{
		{"GET", "/healthz"},
		{"GET", "/readyz"},
		{"GET", "/health"},
		{"GET", "/metrics"},
		{"POST", "/api/v1/auth/signup"},
		{"POST", "/api/v1/auth/login"},
		{"POST", "/api/v1/auth/forgot-password"},
		{"POST", "/api/v1/auth/reset-password"},
		{"POST", "/api/v1/auth/accept-invite"},
		{"GET", "/api/v1/orgs"},
		{"POST", "/api/v1/orgs"},
		{"GET", "/api/v1/orgs/1/members"},
		{"PUT", "/api/v1/orgs/1/members/2"},
		{"DELETE", "/api/v1/orgs/1/members/2"},
		{"GET", "/api/v1/orgs/1/invitations"},
		{"POST", "/api/v1/orgs/1/invitations"},
		{"DELETE", "/api/v1/orgs/1/invitations/5"},
		{"POST", "/api/v1/orgs/1/invitations/5/resend"},
		{"GET", "/api/v1/users/me"},
		{"POST", "/api/v1/users/me/current-org"},
		{"GET", "/api/v1/users"},
		{"GET", "/assets/index.js"},
		{"GET", "/favicon.ico"},
		{"GET", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rctx := chi.NewRouteContext()

			if !r.Match(rctx, tt.method, tt.path) {
				t.Errorf("Route not found: %s %s", tt.method, tt.path)
			}
		})
	}
}

func TestMetricsEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
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

If `frontendhandler.NewHandler` signature still insists on `embed.FS`, either fix that in this step (widen to `fs.FS`) or use a package-local `var emptyFS embed.FS` in the test and pass that.

- [ ] **Step 5: Run the full test suite**

Run:
```bash
cd backend && go test -v ./...
```

Expected: PASS. New tests to watch for: `TestParseCommand` (in `backend`), `TestRouterSetup`, `TestRouterRegistration`, `TestMetricsEndpoint` (in `internal/cmd/serve`), `TestRun_MissingPGURL` (in `internal/cmd/migrate`), `TestFSContainsMigrations` (in `migrations`). Everything else in the project must continue to pass.

- [ ] **Step 6: Commit**

```bash
git add backend/main.go backend/main_test.go backend/internal/cmd/serve/serve_test.go
git commit -m "feat(tra-367): swap main.go to dispatcher; move router tests

main.go becomes a ~90-line dispatcher that parses os.Args, wires the
logger and signal context once, and delegates to migrate.Run and/or
serve.Run. Bare ./server invocation still runs migrate + serve for
backward compat. Router tests move to internal/cmd/serve alongside
the router they exercise."
```

---

## Task 6: Update `backend/justfile` migrate recipe

**Files:**
- Modify: `backend/justfile`

- [ ] **Step 1: Edit the `migrate` recipe**

Replace the `migrate:` recipe (lines ~96–99 of `backend/justfile`) so it invokes the new subcommand. Leave all other `migrate-*` recipes untouched.

Old:
```just
# Run database migrations
migrate:
    @echo "🔄 Running database migrations..."
    docker compose exec backend sh -c 'migrate -path /app/migrations -database "$PG_URL" up'
    @echo "✅ Migrations complete"
```

New:
```just
# Run database migrations using the embedded set (./server migrate)
migrate:
    @echo "🔄 Running database migrations..."
    @env PG_URL="{{pg_url_local}}" go run . migrate
    @echo "✅ Migrations complete"
```

- [ ] **Step 2: Verify the recipe runs**

Prerequisite: local Postgres is up (`just database up`).

Run:
```bash
just backend migrate
```

Expected: logs "Starting migrations" then either "Migrations complete" (fresh DB) or "No pending migrations" (already current). Exit 0.

If you get "PG_URL environment variable not set", confirm `.env.local` exports `PG_URL_LOCAL` and direnv is loaded.

- [ ] **Step 3: Commit**

```bash
git add backend/justfile
git commit -m "chore(tra-367): flip just backend migrate to ./server migrate

Uses the new embedded-migrations subcommand instead of the migrate
CLI inside the backend container. Other migrate-* recipes (down,
status, force, create) keep their CLI-based dev-ergonomics forms
until TRA-85 replaces them."
```

---

## Task 7: End-to-end verification

**Files:**
- No file changes expected (verification only).

- [ ] **Step 1: Build the binary**

Run:
```bash
cd backend && just build
```

Expected: `backend/bin/trakrf` exists, exit 0.

- [ ] **Step 2: Verify `./server` (combined default) still migrates + serves**

Prerequisite: local Postgres is up. In a separate terminal, tail logs; or observe stdout.

Run:
```bash
cd backend && env PG_URL="$PG_URL_LOCAL" ./bin/trakrf &
sleep 3
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/metrics | head -5
kill %1 2>/dev/null || true
```

Expected:
- Logs show `"Starting migrations"` → `"No pending migrations"` (or `"Migrations complete"`) → `"Server starting"`.
- `/healthz` returns 200.
- `/metrics` returns Prometheus text.

- [ ] **Step 3: Verify `./server migrate`**

Run:
```bash
cd backend && env PG_URL="$PG_URL_LOCAL" ./bin/trakrf migrate
echo "exit: $?"
```

Expected: logs end with "Migrations complete" or "No pending migrations". Exit code 0. No HTTP server starts.

- [ ] **Step 4: Verify `./server serve` (no inline migrations)**

Run:
```bash
cd backend && env PG_URL="$PG_URL_LOCAL" ./bin/trakrf serve &
sleep 3
curl -fsS http://localhost:8080/healthz
kill %1 2>/dev/null || true
```

Expected:
- Logs **do not** mention migrations.
- `/healthz` returns 200.

- [ ] **Step 5: Verify unknown subcommand and help**

Run:
```bash
cd backend && ./bin/trakrf bogus; echo "exit: $?"
cd backend && ./bin/trakrf --help; echo "exit: $?"
```

Expected:
- `bogus` → stderr shows `unknown subcommand: "bogus"` and `usage: server [serve|migrate]`, exit 2.
- `--help` → stdout shows `usage: server [serve|migrate]`, exit 0.

- [ ] **Step 6: Run the full validation suite**

Run:
```bash
just validate
```

Expected: PASS. Both frontend and backend lint / test / build / smoke-test green.

- [ ] **Step 7: If every check passed, no further commit needed.**

If a cleanup emerged (e.g. forgot to delete a stale import), commit it:

```bash
git add -A
git commit -m "chore(tra-367): cleanup from e2e verification"
```

---

## Out of plan (follow-ups, not in this PR)

- Infra repo change: flip `migrate.image` in `helm/trakrf-backend` to the backend image with command `["./server", "migrate"]`; delete the `migrations/` ConfigMap and the `just sync-migrations` recipe. Separate PR in the infra repo after this ships.
- [TRA-85](https://linear.app/trakrf/issue/TRA-85): DB user split + flip every deployment target to explicit `migrate` / `serve` invocations, then retire the combined default.
- Migration status / force / down / goto subcommands: if a real need surfaces.
