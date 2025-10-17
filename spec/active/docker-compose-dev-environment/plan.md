# Implementation Plan: Docker Compose Development Environment
Generated: 2025-10-17
Specification: spec.md

## Understanding

This plan establishes the foundation for local backend development by:
1. Setting up TimescaleDB in Docker with the complete trakrf schema
2. Migrating 12 database init scripts from trakrf-web (verbatim, no modifications)
3. Creating unified root `.env.local` configuration with security-first approach
4. Adding Just commands for docker orchestration
5. Updating documentation for easy onboarding

**Key architectural decisions:**
- External MQTT broker (EMQX Cloud) - not bundled in docker-compose
- Named Docker volumes for database persistence (not bind mounts)
- Consolidated to single `PG_URL` (eliminating DATABASE_URL confusion)
- `.env.local.example` contains placeholders only (never secrets)
- Copy SQL scripts as-is now, refactor via Go migrations later (Phase 3+)

## Relevant Files

**Reference Patterns** (existing code to follow):
- `/home/mike/trakrf-web/docker-compose.yaml` (lines 1-16) - TimescaleDB service configuration pattern
- `/home/mike/trakrf-web/.env.local` (lines 37-72) - Working environment variable structure
- `/home/mike/platform/justfile` (lines 8-17) - Existing backend command pattern
- `/home/mike/platform/README.md` (lines 40-67) - Section to replace with updated Quick Start

**Files to Create**:
- `docker-compose.yaml` (root) - TimescaleDB service definition
- `.env.local.example` (root) - Environment variable template with placeholders
- `database/init/*.sql` (12 files) - Database initialization scripts (copied verbatim)

**Files to Modify**:
- `justfile` (add docker orchestration commands after line 49)
- `README.md` (replace lines 40-67 "Quick Start" with "Local Development")
- `.gitignore` (add defensive entry for `timescale_data/` after line 154)
- `.envrc` (already correct - verify `dotenv_if_exists .env.local` is present)

## Architecture Impact
- **Subsystems affected**: Infrastructure (Docker), Database (TimescaleDB), Documentation
- **New dependencies**: None (uses existing Docker, direnv)
- **Breaking changes**: None (pure addition, no existing code affected)

## Task Breakdown

### Task 1: Create docker-compose.yaml
**File**: `docker-compose.yaml` (root)
**Action**: CREATE
**Pattern**: Reference `/home/mike/trakrf-web/docker-compose.yaml` lines 1-16

**Implementation**:
```yaml
volumes:
  timescale_data:

services:
  timescaledb:
    image: timescale/timescaledb:latest-pg17
    container_name: timescaledb
    ports:
      - "5432:5432"
    environment:
      POSTGRES_PASSWORD: ${DATABASE_PASSWORD}
      POSTGRES_DB: ${DATABASE_NAME:-postgres}
    volumes:
      - timescale_data:/var/lib/postgresql/data
      - ./database/init:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 10s
      timeout: 5s
      retries: 5
```

**Key differences from trakrf-web:**
- Service name: `timescaledb` (not `database`)
- Volume name: `timescale_data` (not `database_data`)
- Braces: `${DATABASE_PASSWORD}` (explicit braces)
- Health check: Added for robust `just db-up` waiting
- No ingester service (Phase 2)

**Validation**:
```bash
# Syntax check
docker-compose config

# Start service
docker-compose up -d timescaledb

# Verify health
docker-compose ps timescaledb
docker-compose exec timescaledb pg_isready -U postgres
```

### Task 2: Copy database init scripts
**File**: `database/init/*.sql` (12 files)
**Action**: CREATE (via copy)
**Pattern**: Direct copy from `/home/mike/trakrf-web/db/init/`

**Implementation**:
```bash
# Create directory
mkdir -p database/init

# Copy all 12 SQL files verbatim
cp /home/mike/trakrf-web/db/init/*.sql database/init/

# Verify all files copied
ls -1 database/init/
```

**Expected files (826 total lines)**:
- `01-prereqs.sql` (68 lines) - TimescaleDB extension, trakrf schema, utility functions
- `02-accounts.sql` (41 lines) - Account management
- `03-users.sql` (41 lines) - User management
- `04-account_users.sql` (44 lines) - Multi-tenancy junction
- `05-locations.sql` (51 lines) - Location tracking
- `06-devices.sql` (54 lines) - Device registry
- `07-antennas.sql` (55 lines) - Antenna metadata
- `08-assets.sql` (54 lines) - Asset tracking
- `09-tags.sql` (54 lines) - RFID tag data
- `10-events.sql` (130 lines) - Event logging
- `11-messages.sql` (150 lines) - MQTT message storage
- `99-sample-data.sql` (84 lines) - Sample/seed data

**Validation**:
```bash
# Verify file count
ls database/init/*.sql | wc -l  # Should show 12

# Verify line counts match
wc -l database/init/*.sql

# Start database and check schema creation (after Task 1)
docker-compose up -d timescaledb
sleep 10
docker-compose exec timescaledb psql -U postgres -c "\dn"  # Should show trakrf schema
docker-compose exec timescaledb psql -U postgres -c "\dt trakrf.*"  # Should show tables
```

### Task 3: Create .env.local.example
**File**: `.env.local.example` (root)
**Action**: CREATE
**Pattern**: Reference `/home/mike/trakrf-web/.env.local` lines 37-72, but with placeholders only

**Implementation**:
```bash
# =============================================================================
# ⚠️  SECURITY: This is a TEMPLATE - Replace all placeholder values
# Copy actual credentials from ../trakrf-web/.env.local
# NEVER commit .env.local to git (it's in .gitignore)
# =============================================================================

# -----------------------------------------------------------------------------
# Backend: Database (local docker)
# -----------------------------------------------------------------------------
# Password for PostgreSQL superuser
DATABASE_PASSWORD=your_secure_password_here

# Database name (default: postgres)
DATABASE_NAME=postgres

# Database connection URL (used by backend when running in Docker)
# Format: postgresql://user:password@host:port/database?options
# Note: Password must be URL-encoded (# → %23, ! → %21, ^ → %5E)
PG_URL=postgresql://postgres:your_url_encoded_password@timescaledb:5432/postgres?sslmode=disable&options=-c%20search_path%3Dtrakrf,public

# PostgreSQL host (for backend running in Docker)
PG_HOST=timescaledb

# PostgreSQL port
PG_PORT=5432

# -----------------------------------------------------------------------------
# Backend: MQTT (EMQX Cloud - copy from ../trakrf-web/.env.local)
# -----------------------------------------------------------------------------
# MQTT protocol (mqtts for TLS)
MQTT_PROTO=mqtts

# EMQX Cloud hostname
MQTT_HOST=your-emqx-cloud-host.emqxsl.com

# EMQX Cloud port (8883 for TLS)
MQTT_PORT=8883

# MQTT username
MQTT_USER=your_mqtt_username

# MQTT password
MQTT_PASS=your_mqtt_password

# MQTT topic filter (# = wildcard)
MQTT_TOPIC=your_topic/#

# Full MQTT connection URL (used by Redpanda Connect ingester)
# Note: Password must be URL-encoded
MQTT_URL=mqtts://your_user:your_url_encoded_pass@your-host.emqxsl.com:8883

# MQTT client ID (unique identifier for this connection)
MQTT_CLIENT_ID=trakrf-platform-local

# -----------------------------------------------------------------------------
# Backend: Cloud DB (optional - for production cloud connection)
# -----------------------------------------------------------------------------
# Timescale Cloud connection URL (copy from ../trakrf-web/.env.local if using)
CLOUD_PG_URL=postgres://user:password@your-cloud-host.tsdb.cloud.timescale.com:12345/tsdb?sslmode=require&options=-c%20search_path%3Dtrakrf,public

# -----------------------------------------------------------------------------
# Frontend: VITE_* vars (exposed to browser)
# -----------------------------------------------------------------------------
# Backend API endpoint
VITE_API_URL=http://localhost:8080/api

# Application name
VITE_APP_NAME="TrakRF Platform"

# Log level (debug, info, warn, error)
VITE_LOG_LEVEL=debug

# Enable BLE mock mode (true = no hardware needed)
VITE_BLE_MOCK_ENABLED=true

# Default antenna power (dBm)
VITE_DEFAULT_ANTENNA_POWER=25

# Default inventory session (0-3)
VITE_DEFAULT_INVENTORY_SESSION=1

# BLE command timeout (milliseconds)
VITE_BLE_COMMAND_TIMEOUT=10000

# -----------------------------------------------------------------------------
# Frontend: BLE Bridge (E2E tests - adjust to your bridge server IP)
# -----------------------------------------------------------------------------
# IP address of BLE MCP bridge server
BLE_MCP_HOST=192.168.x.x

# WebSocket port
BLE_MCP_WS_PORT=8080

# HTTP port
BLE_MCP_HTTP_PORT=8081

# Authentication token for bridge HTTP API
BLE_MCP_HTTP_TOKEN=your_bridge_token
```

**Security notes:**
- All values are placeholders
- Real credentials copied manually from `../trakrf-web/.env.local`
- File is committed to git (safe because no secrets)
- Developer copies to `.env.local` and fills in real values

**Validation**:
```bash
# Verify file exists
test -f .env.local.example && echo "✓ Template exists"

# Verify no real secrets
grep -q "your_" .env.local.example && echo "✓ Uses placeholders"

# Verify security warning present
grep -q "SECURITY" .env.local.example && echo "✓ Has security warning"
```

### Task 4: Update justfile with docker commands
**File**: `justfile`
**Action**: MODIFY (add commands after line 49)
**Pattern**: Follow existing recipe format from lines 8-17

**Implementation**:
Add after `check: validate` (line 49):

```just
# Docker Compose orchestration
db-up:
    docker-compose up -d timescaledb
    @echo "⏳ Waiting for database to be ready..."
    @for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
        if docker-compose exec timescaledb pg_isready -U postgres > /dev/null 2>&1; then \
            echo "✅ Database is ready"; \
            exit 0; \
        fi; \
        sleep 2; \
    done; \
    echo "⚠️  Database is starting but not ready yet. Run 'just db-status' to check."

db-down:
    docker-compose down

db-logs:
    docker-compose logs -f timescaledb

db-shell:
    docker-compose exec timescaledb psql -U postgres -d postgres

db-reset:
    @echo "⚠️  This will delete all data. Press Ctrl+C to cancel."
    @sleep 3
    docker-compose down -v
    docker-compose up -d timescaledb
    @echo "⏳ Waiting for database to initialize..."
    @sleep 10
    @echo "✅ Database reset complete"

db-status:
    @docker-compose ps timescaledb
    @docker-compose exec timescaledb pg_isready -U postgres && echo "✅ Database is ready" || echo "❌ Database not ready"
```

**Key design choices:**
- Retry loop: 15 attempts × 2s = 30s max wait
- Silent pg_isready checks during loop (cleaner output)
- Informative messages for all states
- `db-reset` has 3-second warning for safety
- `db-status` shows both container state and database readiness

**Validation**:
```bash
# Syntax check
just --list | grep -E "^  db-"

# Test each command
just db-up
just db-status
just db-logs  # Ctrl+C to exit
just db-shell  # \q to exit
```

### Task 5: Update README.md
**File**: `README.md`
**Action**: MODIFY (replace lines 40-67 "Quick Start")
**Pattern**: Maintain existing markdown structure and tone

**Implementation**:
Replace the "Quick Start" section (lines 40-67) with:

```markdown
## Local Development

### Prerequisites
- Docker & Docker Compose
- direnv (optional but recommended - auto-loads `.env.local`)
- Just (task runner) - https://just.systems/

### Setup

**1. Configure environment variables**
```bash
# Copy template
cp .env.local.example .env.local

# Edit .env.local and fill in real values from ../trakrf-web/.env.local:
#   - DATABASE_PASSWORD (and URL-encode it in PG_URL)
#   - MQTT_HOST, MQTT_USER, MQTT_PASS, MQTT_TOPIC (from EMQX Cloud)
#   - CLOUD_PG_URL (if using Timescale Cloud)

# Enable direnv (auto-loads .env.local when cd'ing into directory)
direnv allow
```

**2. Start database**
```bash
# Start TimescaleDB
just db-up

# Verify database is ready
just db-status

# View logs (optional)
just db-logs

# Connect to database (optional)
just db-shell
```

**3. Verify schema**
```bash
# Inside psql (from 'just db-shell'):
\dn                    # List schemas - should show 'trakrf'
\dt trakrf.*           # List tables in trakrf schema
SELECT * FROM trakrf.accounts;  # Should show sample data
\q                     # Quit
```

### Database Management

```bash
# Start database
just db-up

# Stop database
just db-down

# View logs
just db-logs

# Connect to psql
just db-shell

# Check database status
just db-status

# Reset database (⚠️  DELETES ALL DATA)
just db-reset
```

### External Services

**MQTT Broker (EMQX Cloud):**
- Readers publish tag data to cloud broker
- Backend subscribes from cloud (configured in `.env.local`)
- Enables remote developer access to live tag stream
- Cost: ~$1-2/month
- Alternative: Run local EMQX on Portainer for isolated testing
```

**Validation**:
```bash
# Verify README renders correctly
grep -A 5 "## Local Development" README.md

# Check for key sections
grep -q "Prerequisites" README.md && echo "✓ Prerequisites section"
grep -q "Database Management" README.md && echo "✓ Database commands documented"
grep -q "EMQX Cloud" README.md && echo "✓ External MQTT documented"
```

### Task 6: Update .gitignore
**File**: `.gitignore`
**Action**: MODIFY (add defensive entry)
**Pattern**: Add to Docker volumes section around line 154

**Implementation**:
Add after line 154 (`timescaledb-data/`):

```gitignore
# Docker volumes (defensive - named volumes don't create local directories,
# but this protects against someone switching to bind mounts)
timescale_data/
```

**Validation**:
```bash
# Verify .gitignore includes necessary patterns
grep -q "\.env\.local" .gitignore && echo "✓ .env.local ignored"
grep -q "timescale_data" .gitignore && echo "✓ timescale_data/ ignored"
grep -q "spec/csw" .gitignore && echo "✓ spec/csw ignored"

# Verify .env.local.example is NOT ignored (should be committed)
git check-ignore .env.local.example && echo "❌ ERROR: .env.local.example should not be ignored" || echo "✓ .env.local.example will be committed"
```

### Task 7: Verify .envrc configuration
**File**: `.envrc`
**Action**: VERIFY (no changes needed)
**Pattern**: Ensure `dotenv_if_exists .env.local` is present

**Implementation**:
Read `.envrc` and verify line 3 contains:
```bash
dotenv_if_exists .env.local
```

This is already correct (from previous read). No changes needed.

**Validation**:
```bash
# Verify direnv configuration
grep -q "dotenv_if_exists .env.local" .envrc && echo "✓ .envrc configured correctly"

# Test direnv loading (if direnv installed)
if command -v direnv &> /dev/null; then
    direnv allow
    echo "✓ direnv configuration allowed"
fi
```

### Task 8: Integration validation
**File**: Multiple files
**Action**: VALIDATE (final end-to-end test)
**Pattern**: Complete workflow from clean state to working database

**Implementation**:
```bash
# Clean slate
docker-compose down -v

# Start fresh
just db-up

# Wait for ready (already handled by just db-up retry loop)

# Verify database schema
docker-compose exec timescaledb psql -U postgres -c "\dn" | grep trakrf
docker-compose exec timescaledb psql -U postgres -c "\dt trakrf.*" | grep accounts

# Verify sample data loaded
docker-compose exec timescaledb psql -U postgres -c "SELECT COUNT(*) FROM trakrf.accounts;"

# Verify health check
docker inspect timescaledb --format='{{json .State.Health.Status}}' | grep healthy

# Test db-shell access
echo "\q" | just db-shell

# Verify logs accessible
just db-logs --tail 50 | grep "database system is ready to accept connections"
```

**Success criteria**:
- ✅ `trakrf` schema exists
- ✅ All tables created (accounts, users, locations, devices, antennas, assets, tags, events, messages)
- ✅ Sample data loaded (at least 1 account)
- ✅ Health check passing
- ✅ `just db-shell` connects successfully
- ✅ Logs show "ready to accept connections"

**Validation**:
All integration tests above must pass.

## Risk Assessment

**Risk: Database fails to start due to environment variable issues**
- **Mitigation**: Health check in docker-compose + retry loop in `just db-up` provides clear feedback
- **Recovery**: `just db-logs` shows exact error, `just db-status` confirms state

**Risk: SQL init scripts fail to execute**
- **Mitigation**: Docker logs capture all init script errors, numbered scripts ensure correct execution order
- **Recovery**: `just db-reset` wipes state and reruns init scripts, `just db-logs` shows exact SQL error

**Risk: Port 5432 already in use**
- **Mitigation**: Clear error from Docker, user can stop conflicting service or change port in docker-compose.yaml
- **Recovery**: `docker ps | grep 5432` identifies conflict, `just db-down` stops our service

**Risk: Volume persistence causes unexpected state**
- **Mitigation**: `just db-reset` explicitly documented with warning, named volume makes cleanup obvious
- **Recovery**: `docker volume rm platform_timescale_data` (if needed) or `just db-reset`

**Risk: Credentials accidentally committed to git**
- **Mitigation**: `.env.local` already in .gitignore (line 29), `.env.local.example` uses placeholders only
- **Recovery**: If committed, immediate `git rm --cached .env.local` + rotate credentials

**Risk: direnv not installed, environment variables not loaded**
- **Mitigation**: README explicitly documents direnv as optional, docker-compose reads from .env.local directly
- **Recovery**: Manual `export $(cat .env.local | xargs)` or docker-compose handles it automatically

## Integration Points

**Docker Compose:**
- Service name `timescaledb` used in PG_URL: `postgresql://...@timescaledb:5432/...`
- Volume `timescale_data` for persistence
- Health check enables robust waiting in `just db-up`

**Environment Variables:**
- `.envrc` loads `.env.local` automatically with direnv
- `docker-compose` reads `.env.local` for variable substitution
- Backend (Phase 2) will read from environment

**Just Commands:**
- New `db-*` commands integrate with existing validation commands
- `just db-up` prerequisite for future `just backend` workflow
- Consistent naming pattern with existing `frontend-*` and `backend-*` commands

**Database Schema:**
- `trakrf` schema namespace isolation
- `search_path=trakrf,public` in connection URL
- Sample data provides immediate validation capability

**Documentation:**
- README.md "Local Development" section replaces outdated Quick Start
- Clear workflow: Setup → Start → Verify → Manage
- External MQTT pattern documented for onboarding

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are blocking gates. Do not proceed past a failed gate.

After EVERY code change:

**Gate 1: Syntax & Configuration**
```bash
# Docker Compose syntax
docker-compose config

# Verify no syntax errors, service definition correct
```

**Gate 2: Service Health**
```bash
# Start database
just db-up

# Check status
just db-status

# Verify health
docker inspect timescaledb --format='{{.State.Health.Status}}'
# Must show: "healthy"
```

**Gate 3: Schema Validation**
```bash
# Connect and verify schema
just db-shell

# Inside psql:
# \dn                    -- Should show 'trakrf' schema
# \dt trakrf.*           -- Should list all tables
# SELECT COUNT(*) FROM trakrf.accounts;  -- Should return > 0
# \q
```

**Enforcement Rules:**
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Report error and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

**Per-task validation**: After each task, run relevant commands from task validation section

**Final validation**: After Task 8, run complete integration validation:
```bash
# Full validation
just db-reset  # Clean slate
just db-up
just db-status
just db-shell  # Manual verification
```

**Documentation validation**:
```bash
# Verify README is accurate
grep -A 10 "## Local Development" README.md

# Verify .env.local.example is safe to commit
git diff --cached .env.local.example | grep -i "password\|secret" | grep -v "your_"
# Should return nothing (no real credentials)
```

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Existing pattern in trakrf-web to follow exactly
✅ All clarifying questions answered (6/6 decisions locked in)
✅ Simple file operations (copy, create config files)
✅ No new dependencies or packages
✅ Infrastructure-only (no business logic)
✅ Validation is straightforward (database starts = success)
✅ Existing justfile pattern to extend
✅ Docker health checks provide clear success/failure signals

**No significant uncertainty factors**

**Assessment**: High confidence implementation. This is a straightforward infrastructure setup following proven patterns from trakrf-web. The only complexity is ensuring proper variable substitution and file paths, which are easily validated.

**Estimated one-pass success probability**: 95%

**Reasoning**: Simple configuration files with clear validation gates. The 5% uncertainty comes from potential environment-specific issues (port conflicts, Docker version differences), not from code complexity. The retry loop in `just db-up` and comprehensive health checks mitigate most failure modes.
