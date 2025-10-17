# Implementation Plan: Phase 2B - Docker Backend Integration

**Generated:** 2025-10-17
**Specification:** spec.md
**Phase:** 2B of Epic (TrakRF Platform Migration)

## Understanding

Containerize the Go backend HTTP server from Phase 2A into the existing docker-compose development environment. Enable hot-reload via Air for rapid iteration. This completes the "Docker Dev Environment" setup, providing full-stack local development (database + backend) with single-command startup.

**Key Decisions from Planning:**
- Skip `go.sum` copying (stdlib only, no dependencies yet)
- Skip backend healthcheck (no dependent services yet)
- Start services detached, separate logs command (`just dev-logs`)
- Add "Backend Service" section to .env.local.example
- Make Docker the primary workflow in README (Docker-first approach)

## Relevant Files

**Reference Patterns:**
- `docker-compose.yaml` (lines 5-20) - Existing timescaledb service with health check pattern
- `.env.local.example` (lines 7-26) - Database section formatting to mirror
- `justfile` (lines 52-84) - Existing db-* commands pattern to follow
- Phase 2A implementation - `backend/main.go`, `backend/health.go` (working server)

**Files to Create:**
- `backend/Dockerfile` - Multi-stage build (development with Air + production standalone)
- `backend/.air.toml` - Air hot-reload configuration (watch .go files, rebuild on change)
- `backend/.dockerignore` - Exclude binaries, build artifacts, IDE files from Docker context

**Files to Modify:**
- `docker-compose.yaml` - Add backend service with depends_on timescaledb
- `justfile` - Add backend-dev, backend-stop, backend-restart, backend-shell, dev commands
- `.env.local.example` - Add Backend Service section with PORT and LOG_LEVEL
- `README.md` - Add Docker Development section (replace existing backend quickstart)

## Architecture Impact

- **Subsystems affected:** Backend (Docker containerization), Development tooling (Just commands), Documentation
- **New dependencies:** Air (installed via go install in Dockerfile development stage)
- **Breaking changes:** None - Docker is additive, native `just backend-run` still works
- **Developer workflow change:** Primary path becomes `just dev` (Docker) vs `just backend-run` (native)

## Task Breakdown

### Task 1: Create Dockerfile
**File:** `backend/Dockerfile`
**Action:** CREATE
**Pattern:** Multi-stage build (development stage with Air, production stage with standalone binary)

**Implementation:**
```dockerfile
# Development stage (with Air hot-reload)
FROM golang:1.21-alpine AS development
WORKDIR /app

# Install Air for hot-reload
RUN go install github.com/cosmtrek/air@latest

# Copy go.mod only (no go.sum yet - stdlib only project)
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Run Air (reads .air.toml config)
CMD ["air", "-c", ".air.toml"]

# Production stage (standalone binary)
FROM golang:1.21-alpine AS builder
WORKDIR /app

# Copy go.mod and download dependencies
COPY go.mod ./
RUN go mod download

# Copy source and build
COPY . .
RUN go build -ldflags "-X main.version=0.1.0-dev" -o server .

# Final production image
FROM alpine:latest AS production
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
```

**Key Points:**
- Three stages: development (Air), builder (compile), production (runtime)
- Development stage is default for docker-compose (target: development)
- Production stage for Railway deployment (standalone binary)
- Only copy go.mod (no go.sum - Phase 3 will add when dependencies are introduced)

**Validation:**
```bash
# Build development image
docker build -f backend/Dockerfile --target development -t backend:dev ./backend

# Build production image
docker build -f backend/Dockerfile --target production -t backend:prod ./backend

# Verify image sizes
docker images | grep backend
# Expected: dev ~400MB (includes Go toolchain + Air), prod ~15MB (alpine + binary)
```

**Expected:** Both images build successfully, production image < 50MB

---

### Task 2: Create Air Configuration
**File:** `backend/.air.toml`
**Action:** CREATE
**Pattern:** Watch .go files, rebuild on change with 1 second delay

**Implementation:**
```toml
# Air hot-reload configuration for Go backend
root = "."
tmp_dir = "tmp"

[build]
  # Build command (creates binary in tmp/)
  cmd = "go build -o ./tmp/server ."

  # Binary to execute
  bin = "./tmp/server"

  # Delay before rebuilding (milliseconds)
  delay = 1000

  # Directories to exclude from watching
  exclude_dir = ["tmp", "vendor"]

  # File extensions to watch
  include_ext = ["go", "tpl", "tmpl", "html"]

  # Stop on build error (don't run broken binary)
  stop_on_error = true

[log]
  # Show timestamps in Air logs
  time = true

[color]
  # Color-coded log output
  main = "magenta"
  watcher = "cyan"
  build = "yellow"
  runner = "green"
```

**Key Points:**
- Watches all .go files in backend/
- 1 second delay prevents rapid rebuilds on multi-file saves
- Excludes tmp/ directory (where Air builds the binary)
- Colored output for easy log scanning
- Stops on build errors (prevents running broken code)

**Validation:**
```bash
# Will validate in Task 5 when backend service is running
# Expected behavior: Edit main.go, see rebuild in logs within 1-2 seconds
```

---

### Task 3: Create .dockerignore
**File:** `backend/.dockerignore`
**Action:** CREATE
**Pattern:** Exclude files that shouldn't be in Docker build context

**Implementation:**
```
# Binaries (build artifacts)
server
backend
tmp/

# Test artifacts
*.test
*.out

# Development files
.air.toml.local
.env
.env.local

# IDE files
.vscode/
.idea/

# Git
.git/
.gitignore

# Documentation
README.md
```

**Key Points:**
- Excludes binaries that will be rebuilt in container
- Excludes IDE and git files (reduces context size)
- Excludes local env files (use docker-compose environment instead)
- Keeps .air.toml (needed for development stage)

**Validation:**
```bash
# Check Docker context size
cd backend && docker build --no-cache -f Dockerfile --target development -t backend:dev .
# Expected: Context should be < 100KB (just .go files and configs)
```

---

### Task 4: Update docker-compose.yaml
**File:** `docker-compose.yaml`
**Action:** MODIFY (add backend service after timescaledb)
**Pattern:** Reference timescaledb service (lines 5-20)

**Implementation:**

Add after the timescaledb service:

```yaml
  backend:
    build:
      context: ./backend
      target: development
    container_name: backend
    ports:
      - "8080:8080"
    environment:
      PORT: ${PORT:-8080}
      PG_URL: postgres://postgres:${DATABASE_PASSWORD}@timescaledb:5432/${DATABASE_NAME:-postgres}
    volumes:
      - ./backend:/app
    depends_on:
      timescaledb:
        condition: service_healthy
    restart: unless-stopped
```

**Key Points:**
- `target: development` - Uses Air hot-reload stage
- `volumes: ./backend:/app` - Mounts source for hot-reload
- `depends_on` with `condition: service_healthy` - Waits for database
- `PG_URL` uses `timescaledb` hostname (Docker network DNS)
- `restart: unless-stopped` - Auto-restart on crash (not on manual stop)
- No healthcheck yet (no dependent services)

**Validation:**
```bash
# Validate docker-compose syntax
docker-compose config

# Check backend service is defined
docker-compose config --services | grep backend

# Expected: No errors, backend service listed
```

---

### Task 5: Update justfile
**File:** `justfile`
**Action:** MODIFY (add Docker commands after existing backend section)
**Pattern:** Follow existing db-* commands pattern (lines 52-84)

**Implementation:**

Add after line 20 (after `backend: backend-lint backend-test backend-build`):

```makefile
# Docker development commands
backend-dev:
    docker-compose up -d backend
    @echo "🚀 Backend running at http://localhost:8080"
    @echo "📊 Health check: curl localhost:8080/health"
    @echo "📋 View logs: just dev-logs"

backend-stop:
    docker-compose stop backend

backend-restart:
    docker-compose restart backend

backend-shell:
    docker-compose exec backend sh

# Full stack development
dev: db-up backend-dev

dev-stop: backend-stop db-down

dev-logs:
    docker-compose logs -f
```

**Key Points:**
- `backend-dev` starts detached (no automatic log following per Q3 decision)
- Shows helpful hints for health check and viewing logs
- `backend-shell` for debugging inside container
- `dev` starts full stack (database + backend)
- `dev-logs` follows all services (database + backend)

**Validation:**
```bash
# Test commands exist
just --list | grep -E "backend-dev|dev|dev-stop"

# Test backend-dev (full validation)
just backend-dev

# Wait for startup
sleep 5

# Test health endpoints
curl -s localhost:8080/healthz  # Should return "ok"
curl -s localhost:8080/readyz   # Should return "ok"
curl -s localhost:8080/health | jq .  # Should return JSON

# Test hot-reload: Edit backend/main.go (change port log message)
# Expected: Logs show rebuild within 1-2 seconds

# Test logs
just dev-logs  # Should show both database and backend logs

# Cleanup
just dev-stop
```

---

### Task 6: Update .env.local.example
**File:** `.env.local.example`
**Action:** MODIFY (add Backend Service section)
**Pattern:** Follow existing section formatting (lines 7-26 Database section)

**Implementation:**

Add after line 60 (after Cloud DB section), before Frontend section:

```bash
# -----------------------------------------------------------------------------
# Backend Service
# -----------------------------------------------------------------------------
# HTTP server port
PORT=8080

# Log level (debug, info, warn, error)
LOG_LEVEL=info
```

**Key Points:**
- New section header follows existing format
- PORT with default 8080 (matches docker-compose default)
- LOG_LEVEL for future use (not consumed by Phase 2A yet)
- Positioned logically between backend DB config and frontend config

**Validation:**
```bash
# Verify format consistency
grep -A 3 "# Backend Service" .env.local.example

# Expected: Section header with PORT and LOG_LEVEL vars
```

---

### Task 7: Update README.md
**File:** `README.md`
**Action:** MODIFY (add Docker Development section, make Docker-first)
**Pattern:** Replace existing backend quickstart (lines 29-40) with Docker workflow

**Implementation:**

Replace the "Development" section (lines 29-149) with:

```markdown
## Development

### Prerequisites
- Docker & Docker Compose
- Just (task runner) - https://just.systems/
- direnv (optional but recommended - auto-loads `.env.local`)

**Note:** Go and Node.js are NOT required for Docker-based development. Install them only if you want to run services natively.

### Quick Start (Docker-First)

**1. Configure environment**
```bash
# Copy template
cp .env.local.example .env.local

# Edit .env.local and set:
#   - DATABASE_PASSWORD (and URL-encode it in PG_URL)
#   - MQTT credentials from EMQX Cloud
#   - Other backend/frontend vars as needed

# Enable direnv (auto-loads .env.local)
direnv allow
```

**2. Start full stack**
```bash
# Start database + backend with hot-reload
just dev

# Backend will be available at http://localhost:8080
# Logs are streaming to terminal

# In another terminal, test endpoints:
curl localhost:8080/healthz   # Liveness check
curl localhost:8080/readyz    # Readiness check
curl localhost:8080/health    # Detailed health (JSON)
```

**3. Develop with hot-reload**
```bash
# Edit backend/main.go or backend/health.go
# Air automatically rebuilds and restarts (< 5 seconds)

# View logs
just dev-logs

# Access container shell for debugging
just backend-shell
```

**4. Stop services**
```bash
# Stop all services
just dev-stop

# Or stop individual services
just backend-stop
just db-down
```

### Docker Commands

**Backend:**
```bash
just backend-dev       # Start backend (requires db)
just backend-stop      # Stop backend
just backend-restart   # Restart backend
just backend-shell     # Shell into backend container
```

**Full Stack:**
```bash
just dev          # Start database + backend
just dev-stop     # Stop all services
just dev-logs     # Follow logs (all services)
```

**Database:**
```bash
just db-up        # Start TimescaleDB
just db-down      # Stop TimescaleDB
just db-logs      # View database logs
just db-shell     # Connect to psql
just db-status    # Check database health
just db-reset     # ⚠️  Reset database (deletes all data)
```

### Native Development (Optional)

If you have Go 1.21+ installed, you can run backend natively:

```bash
# Run backend natively (outside Docker)
just backend-run      # Starts at localhost:8080

# Run validation
just backend-lint     # Format + lint
just backend-test     # Run tests
just backend-build    # Build binary
just backend          # All checks
```

**Note:** Docker is the recommended workflow. Native commands are available for those who prefer it.
```

**Key Points:**
- Docker is now the primary, default workflow (as per Q5 decision)
- Quick Start focuses on Docker path
- Native workflow moved to "Optional" section
- Clear commands for full stack vs individual services
- Hot-reload behavior explained
- Health check examples included

**Validation:**
```bash
# Verify README renders correctly
# (Manual check - ensure markdown formatting is clean)

# Follow Quick Start steps to validate instructions work
just dev
curl localhost:8080/health
just dev-stop
```

---

## Risk Assessment

**LOW RISKS** ✅
- **Air installation in Dockerfile** - Well-tested, stable tool
  - Mitigation: N/A - Air is production-ready

- **Port 8080 conflicts** - Might be in use locally
  - Mitigation: Configurable via .env.local PORT variable

**MEDIUM RISKS** ⚠️
- **First Docker build slow** - Downloads Go base image (~400MB)
  - Mitigation: Document expected wait time, Docker caches layers

- **Volume mount permissions** - Linux/Mac/Windows differences
  - Mitigation: Air runs as root in container, should work across platforms

**NO HIGH RISKS** - Straightforward Docker configuration

## Integration Points

- **docker-compose:** Backend service integrated with existing timescaledb service
- **justfile:** New Docker commands alongside existing native commands
- **.env.local:** Backend vars added, consumed by docker-compose
- **README:** Docker workflow documented as primary path

## VALIDATION GATES (MANDATORY)

**CRITICAL:** These are BLOCKING gates, not suggestions.

After Task 4 (docker-compose updated):
```bash
docker-compose config  # Syntax validation
```

After Task 5 (justfile updated):
```bash
just --list | grep backend-dev  # Command exists
```

**Full Validation (After Task 7):**
```bash
# Start full stack
just dev

# Wait for startup
sleep 5

# Test all three health endpoints
curl -s localhost:8080/healthz | grep "ok"
curl -s localhost:8080/readyz | grep "ok"
curl -s localhost:8080/health | jq .status | grep "ok"

# Test hot-reload (make a code change)
# Edit backend/main.go, verify logs show rebuild

# Test logs command
just dev-logs  # Should show backend + db logs

# Cleanup
just dev-stop
```

**If ANY validation fails:**
- Fix immediately
- Re-run validation
- Do not proceed until all tests pass

## Validation Sequence

**After each task:**
- Syntax validation (Dockerfile, docker-compose.yaml, justfile)
- No Go code validation needed (no .go files changed)

**Final validation (Task 7 complete):**
- Full end-to-end workflow test (see VALIDATION GATES above)
- Hot-reload cycle test
- All three health endpoints responding

## Plan Quality Assessment

**Complexity Score:** 7/10 (MEDIUM-HIGH)
- Mitigated by: Complete code examples in spec + configuration-only changes

**Confidence Score:** 9/10 (HIGH)

**Confidence Factors:**
- ✅ All code provided in spec (no creative implementation needed)
- ✅ Docker and Air are well-documented, stable technologies
- ✅ Existing docker-compose pattern to follow (timescaledb service)
- ✅ Clear validation strategy with specific curl commands
- ✅ No Go code changes (pure configuration)
- ✅ User answers clarified ambiguous decisions
- ⚠️ New development workflow (Air hot-reload) - minor learning curve

**Assessment:** High confidence. This is primarily configuration work with all code provided. The only novelty is Air hot-reload, which is well-documented and the config is provided in spec. Validation is straightforward (test endpoints, verify reload).

**Estimated one-pass success probability:** 85%

**Reasoning:**
- All configuration files are fully specified in spec (copy-paste ready)
- Docker multi-stage builds are standard practice
- Air is mature and stable
- Validation is concrete (curl commands with expected responses)
- 15% risk accounts for: potential Docker quirks, Air setup variations, or docker-compose networking issues
- All risks have clear mitigation (restart services, check logs, verify network)

## Phase 2B Definition of Done

### Functional Requirements
- ✅ `just dev` starts full stack (database + backend) with hot-reload
- ✅ `just backend-dev` starts backend only (requires db running)
- ✅ Backend container connects to TimescaleDB container
- ✅ Health endpoints accessible at http://localhost:8080
- ✅ Code changes trigger automatic reload (< 5 second cycle)
- ✅ `just dev-logs` shows logs for all services
- ✅ `just dev-stop` stops all containers cleanly

### Quality Requirements
- ✅ Dockerfile follows best practices (multi-stage, minimal layers)
- ✅ Development image includes Air and source mounting
- ✅ Production image is standalone (< 50MB)
- ✅ .dockerignore excludes unnecessary files

### Integration Requirements
- ✅ Backend service defined in docker-compose.yaml
- ✅ Backend depends_on TimescaleDB with health check condition
- ✅ Backend uses PG_URL from .env.local
- ✅ Backend and TimescaleDB on same Docker network
- ✅ Port 8080 exposed and accessible

### Documentation Requirements
- ✅ README documents Docker-first workflow
- ✅ .env.local.example has Backend Service section
- ✅ Just commands listed and explained

## Next Steps After Phase 2B

1. **Test full workflow:**
   - Verify hot-reload cycle
   - Test all Just commands
   - Validate documentation

2. **Phase 3:** Database Migrations (go-migrate)
   - Add db.Ping() to /readyz endpoint
   - Wire up DATABASE_URL environment variable
   - Port 12 SQL init scripts to migrations

3. **Optional:** Test production build
   - `docker build --target production`
   - Deploy to Railway for validation
