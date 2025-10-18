# Implementation Plan: Phase 6A - Justfile Monorepo Structure (Delegation Pattern)
Generated: 2025-10-18
Specification: spec.md

## Understanding

Refactor the flat justfile structure (42 recipes, 145 lines) to leverage Just's delegation pattern with `set fallback := true`. This achieves:
- **Smaller root justfile** (~60 lines, down from 145) - orchestration only
- **Workspace justfiles** - single source of truth for workspace commands
- **Delegation pattern** - `just <workspace> <command>` from root
- **Context-aware** - `just <command>` from workspace directories

**Key Pattern**: Workspace justfiles use `set fallback := true` and define unqualified commands (`dev`, `lint`, `test`, `build`). Root justfile delegates to workspaces via `<workspace> *args: cd <workspace> && just {{args}}`. Combined commands use delegation: `lint: (frontend "lint") (backend "lint")`.

## Relevant Files

**Reference Patterns**:
- **Stuart Ellis article**: https://www.stuartellis.name/articles/just-task-runner/#multiple-justfiles-in-a-directory-structure
- **Just Manual - Fallback**: https://just.systems/man/en/chapter_46.html
- **Just Manual - Recipe Parameters**: https://just.systems/man/en/chapter_21.html (for `*args` pattern)
- Current `justfile` (145 lines, 42 recipes) - will shrink to ~60 lines

**Files to Create**:
- `backend/justfile` (~35 lines) - Backend commands with fallback
- `frontend/justfile` (~40 lines) - Frontend commands with fallback

**Files to Modify**:
- `justfile` (SHRINK from 145 to ~60 lines) - Remove qualified commands, add delegation
- `README.md` - Update Quick Start with delegation examples
- `backend/README.md` - Show workspace-specific and delegation patterns
- `CLAUDE.md` - Document delegation + fallback pattern

## Architecture Impact

- **Subsystems affected**: Developer tooling only (no production code changes)
- **New dependencies**: None (Just already installed)
- **Breaking changes**: Command syntax changes from `just frontend-dev` to `just frontend dev` (no impact - only user running CSW)

## Task Breakdown

### Task 1: Create backend/justfile with fallback
**File**: `backend/justfile`
**Action**: CREATE
**Pattern**: Unqualified command names + fallback

**Implementation**:
```just
# Backend Task Runner (TrakRF Platform)
# Uses fallback to inherit shared recipes from root justfile
set fallback := true

# Start Go development server
dev:
    go run .

# Alias for consistency
run: dev

# Lint Go code (formatting + static analysis)
lint:
    go fmt ./...
    go vet ./...

# Run backend tests with verbose output
test:
    go test -v ./...

# Run tests with race detection
test-race:
    go test -race ./...

# Run tests with coverage report
test-coverage:
    go test -cover ./...

# Build backend binary with version injection
build:
    go build -ldflags "-X main.version=0.1.0-dev" -o bin/trakrf .

# Run all backend validation checks
validate: lint test build

# Shell access
shell:
    docker compose exec backend sh
```

**Validation**:
```bash
cd backend
just --list        # Should show backend recipes + root recipes (fallback)
just dev           # Should run: go run .
just lint          # Should run: go fmt + go vet
```

---

### Task 2: Create frontend/justfile with fallback
**File**: `frontend/justfile`
**Action**: CREATE
**Pattern**: Unqualified command names + fallback

**Implementation**:
```just
# Frontend Task Runner (TrakRF Platform)
# Uses fallback to inherit shared recipes from root justfile
set fallback := true

# Start Vite development server
dev:
    pnpm dev

# Lint frontend code (auto-fix enabled)
lint:
    pnpm run lint --fix

# Run TypeScript type checking
typecheck:
    pnpm run typecheck

# Run frontend unit tests
test:
    pnpm test

# Run E2E tests (headless only)
test-e2e:
    pnpm test:e2e

# Build frontend for production
build:
    pnpm run build

# Run all frontend validation checks
validate: lint typecheck test build

# Install dependencies
install:
    pnpm install

# Clean build artifacts
clean:
    rm -rf dist node_modules/.vite
```

**Validation**:
```bash
cd frontend
just --list        # Should show frontend recipes + root recipes (fallback)
just dev           # Should run: pnpm dev
just typecheck     # Should run: pnpm run typecheck
```

---

### Task 3: Refactor root justfile with delegation pattern
**File**: `justfile`
**Action**: MODIFY (major refactor - shrink from 145 to ~60 lines)
**Pattern**: Delegation + orchestration

**Implementation** - Replace entire file with:

```just
# TrakRF Platform - Task Runner
# https://just.systems/

# List all available recipes
default:
    @just --list

# ============================================================================
# Workspace Delegation
# ============================================================================
# Delegate commands to workspace justfiles
# Usage: just <workspace> <command> [args...]
# Example: just frontend dev, just backend test

frontend *args:
    cd frontend && just {{args}}

backend *args:
    cd backend && just {{args}}

# ============================================================================
# Combined Validation Commands
# ============================================================================
# Run checks across all workspaces

lint: (frontend "lint") (backend "lint")

test: (frontend "test") (backend "test")

build: (frontend "build") (backend "build")

validate: lint test build

# Alias for CSW integration
check: validate

# ============================================================================
# Full Stack Development
# ============================================================================

# Docker-based development (database + backend container)
dev: db-up
    @echo "⏳ Waiting for database to be ready..."
    @sleep 3
    @echo "🔄 Running migrations..."
    @just db-migrate-up
    @echo "🚀 Starting backend..."
    @docker compose up -d backend
    @echo "✅ Development environment ready"

# Local development (parallel frontend + backend)
dev-local:
    @echo "🚀 Starting local development servers..."
    @echo "📱 Frontend: http://localhost:5173"
    @echo "🔧 Backend: http://localhost:8080"
    @echo ""
    @echo "Press Ctrl+C to stop both servers"
    @just frontend dev & just backend dev & wait

dev-stop:
    docker compose stop backend
    docker compose down

dev-logs:
    docker compose logs -f

# ============================================================================
# Docker Compose Orchestration
# ============================================================================

db-up:
    docker compose up -d timescaledb
    @echo "⏳ Waiting for database to be ready..."
    @for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
        if docker compose exec timescaledb pg_isready -U postgres > /dev/null 2>&1; then \
            echo "✅ Database is ready"; \
            exit 0; \
        fi; \
        sleep 2; \
    done; \
    echo "⚠️  Database is starting but not ready yet. Run 'just db-status' to check."

db-down:
    docker compose down

db-logs:
    docker compose logs -f timescaledb

db-shell:
    docker compose exec timescaledb psql -U postgres -d postgres

db-reset:
    @echo "⚠️  This will delete all data. Press Ctrl+C to cancel."
    @sleep 3
    docker compose down -v
    docker compose up -d timescaledb
    @echo "⏳ Waiting for database to initialize..."
    @sleep 10
    @echo "✅ Database reset complete"

db-status:
    @docker compose ps timescaledb
    @docker compose exec timescaledb pg_isready -U postgres && echo "✅ Database is ready" || echo "❌ Database not ready"

# ============================================================================
# Database Migrations
# ============================================================================

db-migrate-up:
    @echo "🔄 Running database migrations..."
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" up'
    @echo "✅ Migrations complete"

db-migrate-down:
    @echo "⏪ Rolling back last migration..."
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" down 1'
    @echo "✅ Rollback complete"

db-migrate-status:
    @echo "📊 Migration status:"
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" version'

db-migrate-create name:
    @echo "📝 Creating new migration: {{name}}"
    docker compose exec backend sh -c 'migrate create -ext sql -dir /app/database/migrations -seq {{name}}'

db-migrate-force version:
    @echo "⚠️  Forcing migration version to {{version}}"
    docker compose exec backend sh -c 'migrate -path /app/database/migrations -database "$PG_URL" force {{version}}'
```

**What Changed**:
- ❌ Removed: All `backend-*` commands (9 recipes, ~30 lines)
- ❌ Removed: All `frontend-*` commands (5 recipes, ~15 lines)
- ✅ Added: `frontend *args:` delegation (1 line)
- ✅ Added: `backend *args:` delegation (1 line)
- ✅ Changed: Combined commands use delegation syntax
- ✅ Added: `dev-local` for parallel development
- ✅ Kept: All Docker/db commands unchanged (orchestration)

**Net Result**: 145 lines → ~60 lines (85 lines removed!)

**Validation**:
```bash
# From root - delegation syntax
just frontend dev          # Should run: cd frontend && just dev
just backend lint          # Should run: cd backend && just lint
just lint                  # Should run both frontend and backend lint
just validate              # Should run full validation

# Old syntax should fail (expected)
just frontend-dev          # Should error: recipe not found
just backend-test          # Should error: recipe not found
```

---

### Task 4: Test command matrix from all contexts
**File**: N/A (testing task)
**Action**: VALIDATE
**Pattern**: Verify delegation + fallback works

**Test Cases**:

From **project root** (delegation):
```bash
just --list              # Should show ~20 recipes (orchestration + delegation)
just frontend dev        # Should start Vite dev server
just backend dev         # Should start Go server
just backend lint        # Should lint Go code
just frontend typecheck  # Should run tsc
just lint                # Should lint both workspaces
just validate            # Should validate both workspaces
just dev                 # Docker orchestration (unchanged)
just dev-local           # Parallel local servers
just db-up               # Database up (unchanged)
```

From **backend/** directory (fallback):
```bash
cd backend
just --list              # Should show backend recipes + root recipes
just dev                 # Should run: go run .
just lint                # Should run: go fmt + go vet
just test                # Should run: go test -v ./...
just build               # Should run: go build
just validate            # Should run: lint test build (backend only)
just db-up               # Should work (fallback to root)
just db-migrate-up       # Should work (fallback to root)
```

From **frontend/** directory (fallback):
```bash
cd frontend
just --list              # Should show frontend recipes + root recipes
just dev                 # Should run: pnpm dev
just lint                # Should run: pnpm run lint --fix
just typecheck           # Should run: pnpm run typecheck
just test                # Should run: pnpm test
just build               # Should run: pnpm run build
just validate            # Should run: lint typecheck test build
just db-up               # Should work (fallback to root)
```

**Validation**: All commands should work as described. If any fail, debug before proceeding.

---

### Task 5: Update README.md with delegation pattern
**File**: `README.md`
**Action**: MODIFY
**Pattern**: Show delegation syntax and workspace context

**Find** the Quick Start section and **replace** with:

```markdown
### Development Workflow

**From project root (delegation syntax):**
```bash
# Full stack
just dev           # Docker-based (db + backend container + migrations)
just dev-local     # Local parallel (frontend + backend dev servers)

# Workspace-specific
just frontend dev        # Start Vite dev server
just backend dev         # Start Go server
just frontend typecheck  # TypeScript type checking
just backend test        # Run Go tests

# Combined validation
just lint        # Lint both workspaces
just test        # Test both workspaces
just build       # Build both workspaces
just validate    # Full validation (lint + test + build)
```

**From workspace directories (direct commands):**
```bash
# Backend development
cd backend
just dev           # Start Go server
just test          # Run backend tests
just validate      # Backend-only validation
just db-up         # Database commands work via fallback

# Frontend development
cd frontend
just dev           # Start Vite dev server
just test          # Run frontend tests
just typecheck     # TypeScript checking
just validate      # Frontend-only validation
```

**How it works:**
- **Delegation**: `just <workspace> <command>` from root → `cd <workspace> && just <command>`
- **Fallback**: Workspace justfiles can call root recipes (db commands, etc.)
- **Context-aware**: `just dev` does the right thing based on current directory
```

**Validation**: Read updated README.md to verify examples are clear

---

### Task 6: Update backend/README.md with delegation examples
**File**: `backend/README.md`
**Action**: MODIFY
**Pattern**: Show workspace-first, then delegation

**Find** the "Quick Start" section (around line 64) and **replace** with:

```markdown
### Quick Start

#### Development Mode (with hot reload)

**Option 1: From workspace directory (recommended)**
```bash
# Terminal 1: Frontend
cd frontend && just dev  # http://localhost:5173

# Terminal 2: Backend
cd backend && just dev   # http://localhost:8080
# CORS enabled automatically for frontend dev
```

**Option 2: From root with delegation**
```bash
# Terminal 1: Frontend
just frontend dev        # http://localhost:5173

# Terminal 2: Backend
just backend dev         # http://localhost:8080
```

**Option 3: Parallel local development**
```bash
just dev-local           # Starts both in parallel
# Frontend: http://localhost:5173
# Backend: http://localhost:8080
# Press Ctrl+C to stop both
```

**Option 4: Docker orchestration**
```bash
just dev                 # Full stack with database
```

#### Production Mode (integrated)

```bash
# Build everything (frontend + backend)
./scripts/build.sh

# Or via Just
just build              # Builds both workspaces

# Run integrated server
cd backend && ./bin/trakrf
# Full app on http://localhost:8080
```
```

**Validation**: Read updated backend/README.md to verify clarity

---

### Task 7: Document delegation + fallback in CLAUDE.md
**File**: `CLAUDE.md`
**Action**: MODIFY
**Pattern**: Add comprehensive Just section

**Find** the package manager rules section (around line 25) and **add** this section after:

```markdown
## 🔧 CRITICAL: Justfile Structure (Delegation + Fallback Pattern)

**Just Task Runner with Delegation Pattern**

This project uses Just's delegation pattern for monorepo task management:

### Structure
```
platform/
├── justfile                 # Root orchestration (~60 lines)
│                           # - Delegation: frontend *args, backend *args
│                           # - Orchestration: dev, db-*, docker commands
│                           # - Combined: lint, test, build, validate
├── backend/justfile         # Backend-specific (~35 lines)
│                           # - set fallback := true
│                           # - Unqualified: dev, lint, test, build
└── frontend/justfile        # Frontend-specific (~40 lines)
                            # - set fallback := true
                            # - Unqualified: dev, lint, typecheck, test, build
```

### How It Works: Delegation + Fallback

**Delegation** (root → workspace):
```just
# Root justfile
frontend *args:
    cd frontend && just {{args}}

backend *args:
    cd backend && just {{args}}
```

**Fallback** (workspace → root):
```just
# frontend/justfile
set fallback := true

# This enables calling root recipes like:
# just db-up (falls back to root justfile)
```

### Command Patterns

**From project root (delegation syntax):**
```bash
just frontend dev        # Delegates to: cd frontend && just dev
just backend lint        # Delegates to: cd backend && just lint
just backend test        # Delegates to: cd backend && just test
```

**From workspace directory (direct + fallback):**
```bash
cd backend
just dev                 # Runs local backend/justfile recipe
just db-up               # Falls back to root justfile recipe
just validate            # Runs local backend/justfile recipe
```

**Combined commands (orchestration):**
```bash
just lint                # Runs: just frontend lint && just backend lint
just test                # Runs: just frontend test && just backend test
just validate            # Runs: lint + test + build for both workspaces
```

### Mental Model

**Root justfile** = Orchestra conductor
- Delegates to workspaces: `just <workspace> <command>`
- Orchestrates Docker/database: `just dev`, `just db-up`
- Combines workspace commands: `just lint`, `just validate`

**Workspace justfiles** = Musicians
- Define unqualified commands: `dev`, `lint`, `test`, `build`
- Can call root recipes via fallback: `db-up`, `validate`
- Override root recipes if same name exists locally

### When Modifying Justfiles

**Adding workspace-specific command**:
1. Add to `backend/justfile` or `frontend/justfile` with unqualified name
2. Test from workspace: `cd backend && just new-command`
3. Test from root: `just backend new-command`

**Adding orchestration command**:
1. Add to root `justfile`
2. Automatically available from all directories
3. If needs workspace commands, use delegation: `(frontend "cmd")`

**Adding combined command**:
1. Add to root `justfile` using delegation syntax
2. Example: `my-check: (frontend "typecheck") (backend "test")`

### Command Syntax Summary

| Location | Syntax | Result |
|----------|--------|--------|
| Root | `just frontend dev` | Delegates to frontend workspace |
| Root | `just backend lint` | Delegates to backend workspace |
| Root | `just lint` | Runs both frontend and backend lint |
| Root | `just dev` | Docker orchestration |
| Root | `just dev-local` | Parallel local dev servers |
| Workspace | `just dev` | Runs local dev command |
| Workspace | `just db-up` | Falls back to root recipe |
| Workspace | `just validate` | Runs local validate command |

### Breaking Changes from Old Pattern

**Old syntax** (removed):
```bash
just frontend-dev        # ❌ No longer exists
just backend-lint        # ❌ No longer exists
just backend-test        # ❌ No longer exists
```

**New syntax** (delegation):
```bash
just frontend dev        # ✅ Space instead of hyphen
just backend lint        # ✅ Space instead of hyphen
just backend test        # ✅ Space instead of hyphen
```

**Same character count, just s/-/ /**

### Why This Pattern?

1. **Single Source of Truth**: Workspace commands only in workspace justfiles
2. **Smaller Root**: ~60 lines (down from 145) - orchestration only
3. **Clearer Separation**: Root = conductor, workspaces = musicians
4. **More Maintainable**: Add workspace command = edit workspace file only
5. **Scalable**: Add third workspace = one delegation line in root

**References**:
- Stuart Ellis - Just Monorepo: https://www.stuartellis.name/articles/just-task-runner/#multiple-justfiles-in-a-directory-structure
- Just Manual - Fallback: https://just.systems/man/en/chapter_46.html
- Just Manual - Parameters: https://just.systems/man/en/chapter_21.html
```

**Validation**: Read updated CLAUDE.md to verify documentation is comprehensive

---

### Task 8: Final validation - all workflows regression-free
**File**: N/A (comprehensive testing)
**Action**: VALIDATE
**Pattern**: Ensure no regressions, verify new patterns work

**Critical Workflows**:

1. **Docker workflow** (unchanged):
   ```bash
   just dev               # Docker orchestration
   just db-migrate-up     # Database migrations
   just dev-stop          # Stop services
   ```

2. **Validation workflow** (changed syntax):
   ```bash
   just validate          # Combined validation
   just lint              # Combined lint
   just test              # Combined test
   just build             # Combined build
   ```

3. **Workspace workflow from root** (new delegation syntax):
   ```bash
   just frontend dev      # Start Vite
   just backend dev       # Start Go server
   just frontend typecheck
   just backend test
   ```

4. **Workspace workflow from directory** (new):
   ```bash
   cd backend && just validate    # Backend only
   cd frontend && just validate   # Frontend only
   cd backend && just dev         # Direct command
   cd frontend && just dev        # Direct command
   ```

5. **Local dev workflow** (new):
   ```bash
   just dev-local         # Parallel servers
   # Verify both start
   # Verify Ctrl+C stops both
   ```

6. **Fallback workflow** (new):
   ```bash
   cd backend && just db-up       # Should work (fallback)
   cd frontend && just db-status  # Should work (fallback)
   cd backend && just db-migrate-up
   ```

**Validation**: Every workflow above should work. Document any issues before marking complete.

---

## Risk Assessment

**Risk**: Users confused by syntax change (`frontend-dev` → `frontend dev`)
- **Mitigation**: Only one user (you), no impact. Documentation updated everywhere.

**Risk**: Delegation pattern unclear to future developers
- **Mitigation**: Comprehensive CLAUDE.md documentation with examples and mental model

**Risk**: Fallback doesn't work as expected
- **Mitigation**: Stuart Ellis pattern is well-tested, Just manual documents it clearly

**Risk**: Docker commands break from subdirectories
- **Mitigation**: `docker compose` uses project root by default (no path issues)

**Risk**: Combined commands become harder to read
- **Mitigation**: Delegation syntax `(frontend "lint")` is explicit and clear

## Integration Points

- **Root justfile**: Major refactor (145 → ~60 lines), add delegation, remove qualified commands
- **Workspace justfiles**: New files with fallback, unqualified command names
- **Documentation**: README.md, backend/README.md, CLAUDE.md all updated with new patterns
- **No code changes**: Purely developer tooling

## VALIDATION GATES (MANDATORY)

**For this refactor, validation is behavioral testing:**

After EVERY task:
1. **Syntax check**: Run `just --list` from relevant directory
2. **Command execution**: Test the specific recipes created/modified
3. **Delegation test**: Verify `just <workspace> <command>` works from root
4. **Fallback test**: Verify root recipes work from workspace directories

**After all tasks complete:**
1. Run complete command matrix (Task 4)
2. Test all critical workflows (Task 8)
3. Verify documentation accuracy (Tasks 5, 6, 7)

**No code validation gates** (lint/test/build) apply since this is tooling only.

## Validation Sequence

1. **After Task 1**: `cd backend && just --list && just dev`
2. **After Task 2**: `cd frontend && just --list && just dev`
3. **After Task 3**: Test delegation from root + verify old commands fail
4. **After Task 4**: Complete command matrix passes
5. **After Tasks 5-7**: Documentation is accurate
6. **After Task 8**: All workflows work, no regressions

## Plan Quality Assessment

**Complexity Score**: 6/10 (MEDIUM-HIGH but acceptable)
- At threshold but low-risk (tooling refactor, not production code)
- Delegation pattern superior to original flat structure
- Zero new dependencies

**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear spec with concrete examples
- ✅ Reference implementation (Stuart Ellis + Just manual)
- ✅ User has prior experience with fallback pattern
- ✅ Delegation pattern is cleaner than import
- ✅ Zero new dependencies (Just already installed)
- ✅ No production code changes (tooling only)
- ✅ No breaking changes impact (only user is you)
- ✅ Root justfile SHRINKS (better architecture)
- ✅ Clear testing strategy (command matrix)
- ⚠️ Pattern novelty for this codebase (first use)

**Assessment**: High confidence. Delegation pattern results in cleaner, more maintainable structure. Root justfile shrinks from 145 to ~60 lines. Single source of truth for workspace commands. Better separation of concerns.

**Estimated one-pass success probability**: 90%

**Reasoning**: Simple refactor with clear reference implementation and superior architecture. The 10% risk comes from potential edge cases in delegation or fallback, but Just's documentation is clear and user has prior experience. Delegation pattern is architecturally superior to flat structure, resulting in more maintainable code.
