# Feature: Docker Compose Development Environment

## Origin
Setting up local development infrastructure for the TrakRF platform monorepo. This provides the foundation for backend development by establishing TimescaleDB with the existing schema from trakrf-web.

## Outcome
A working local development environment where `docker-compose up` starts TimescaleDB with the complete trakrf schema, ready for backend development.

## User Story
As a platform developer
I want a local TimescaleDB instance with the trakrf schema
So that I can develop the Go backend against a real database with live reader data from EMQX Cloud

## Context

**Current State:**
- Frontend fully integrated in monorepo (Phase 1 & 2 complete)
- No backend infrastructure yet
- Working trakrf-web setup with 12 database init scripts
- Readers sending live data to EMQX Cloud broker ($1-2/month)
- Redpanda Connect ingester running separately

**Desired State:**
- TimescaleDB running in Docker with complete schema
- Database accessible at localhost:5432
- All 12 init scripts from trakrf-web migrated as-is
- Unified .env.local for all environment configuration
- Just commands for docker orchestration
- Ready for Go backend development (Phase 2)

**Why This Matters:**
- Establishes infrastructure foundation for backend
- Enables development with real database and live data
- Maintains working pattern: readers → EMQX Cloud → local backend → local DB
- Single .env.local simplifies onboarding

## Technical Requirements

### 1. Docker Compose Configuration

**File:** `docker-compose.yaml`

**Services:**
- **timescaledb**: TimescaleDB pg17 with persistent volume
  - Port: 5432 (exposed to host)
  - Environment: DATABASE_PASSWORD, DATABASE_NAME
  - Volume: database_data (persistent)
  - Volume mount: ./database/init → /docker-entrypoint-initdb.d
  - Health check: pg_isready

**MQTT Broker:** NOT included (external by design - EMQX Cloud or Portainer)

### 2. Database Init Scripts

**Source:** `../trakrf-web/db/init/*.sql` (12 files)

**Destination:** `database/init/` (copy as-is, no modifications)

**Scripts to migrate:**
```
01-prereqs.sql           - TimescaleDB extension, trakrf schema, utility functions
02-accounts.sql          - Account management
03-users.sql             - User management
04-account_users.sql     - Multi-tenancy junction
05-locations.sql         - Location tracking
06-devices.sql           - Device registry
07-antennas.sql          - Antenna metadata
08-assets.sql            - Asset tracking
09-tags.sql              - RFID tag data
10-events.sql            - Event logging
11-messages.sql          - MQTT message storage
99-sample-data.sql       - Sample/seed data
```

**Important:** Copy verbatim - do NOT modify schemas yet. Changes come later via Go migrations.

### 3. Unified Environment Configuration

**File:** `.env.local.example` (root)

**Structure:**
```bash
# =============================================================================
# ⚠️  SECURITY: This is a TEMPLATE - Replace all placeholder values
# Copy actual credentials from ../trakrf-web/.env.local
# NEVER commit .env.local to git (it's in .gitignore)
# =============================================================================

# Backend: Database (local docker)
DATABASE_PASSWORD=your_secure_password_here
DATABASE_NAME=postgres
DATABASE_URL=postgresql://postgres:your_url_encoded_password@localhost:5432/postgres?options=-c%20search_path%3Dtrakrf,public
PG_HOST=timescaledb
PG_PORT=5432
PG_URL=postgresql://postgres:your_url_encoded_password@timescaledb:5432/postgres?sslmode=disable&options=-c%20search_path%3Dtrakrf,public

# Backend: MQTT (EMQX Cloud - copy from ../trakrf-web/.env.local)
MQTT_PROTO=mqtts
MQTT_HOST=your-emqx-cloud-host.emqxsl.com
MQTT_PORT=8883
MQTT_USER=your_mqtt_username
MQTT_PASS=your_mqtt_password
MQTT_TOPIC=your_topic/#
MQTT_URL=mqtts://your_user:your_url_encoded_pass@your-host.emqxsl.com:8883
MQTT_CLIENT_ID=trakrf-platform-local

# Backend: Cloud DB (optional - copy from ../trakrf-web/.env.local)
CLOUD_PG_URL=postgres://user:password@your-cloud-host.tsdb.cloud.timescale.com:12345/tsdb?sslmode=require&options=-c%20search_path%3Dtrakrf,public

# Frontend: VITE_* vars (exposed to browser)
VITE_API_URL=http://localhost:8080/api
VITE_APP_NAME="TrakRF Platform"
VITE_LOG_LEVEL=debug
VITE_BLE_MOCK_ENABLED=true
VITE_DEFAULT_ANTENNA_POWER=25
VITE_DEFAULT_INVENTORY_SESSION=1
VITE_BLE_COMMAND_TIMEOUT=10000

# Frontend: BLE Bridge (E2E tests - adjust to your bridge server IP)
BLE_MCP_HOST=192.168.x.x
BLE_MCP_WS_PORT=8080
BLE_MCP_HTTP_PORT=8081
BLE_MCP_HTTP_TOKEN=your_bridge_token
```

**Security Notes:**
- ⚠️ `.env.local.example` contains NO secrets - only placeholders
- ⚠️ Developer copies to `.env.local` and fills in real values from `../trakrf-web/.env.local`
- ⚠️ `.env.local` MUST be in .gitignore (already is)
- ⚠️ NEVER commit actual credentials to git

**Note on duplication:** DATABASE_PASSWORD + DATABASE_URL both exist because URL needs URL-encoded password. This is acceptable - can't easily template env vars.

### 4. direnv Integration

**File:** `.envrc` (root, already exists)

**Content:**
```bash
dotenv .env.local
```

**Behavior:**
- Loads when you `cd` into monorepo
- Backend reads all vars from shell
- Vite only exposes `VITE_*` vars to browser bundle
- Secrets stay server-side automatically

### 5. Just Commands

**Update:** `justfile`

**Add docker commands:**
```just
# Docker Compose orchestration
db-up:
    docker-compose up -d timescaledb
    @echo "⏳ Waiting for database to be ready..."
    @sleep 5
    @docker-compose exec timescaledb pg_isready -U postgres || echo "Database starting..."

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
    @echo "✅ Database reset complete"

db-status:
    @docker-compose ps timescaledb
    @docker-compose exec timescaledb pg_isready -U postgres && echo "✅ Database is ready" || echo "❌ Database not ready"
```

### 6. Documentation Updates

**Update:** `README.md` (root)

**Add section:**
```markdown
## Local Development

### Prerequisites
- Docker & Docker Compose
- direnv (optional, auto-loads .env.local)

### Setup
```bash
# Copy environment template
cp .env.local.example .env.local

# Start database
just db-up

# Verify database is ready
just db-status

# View logs
just db-logs

# Connect to database
just db-shell
```

### Database Management
- `just db-up` - Start TimescaleDB
- `just db-down` - Stop services
- `just db-logs` - View database logs
- `just db-shell` - Connect to psql
- `just db-reset` - Reset database (WARNING: deletes all data)
```

**Add note about external MQTT:**
```markdown
### External Services

**MQTT Broker (EMQX Cloud):**
- Readers publish to cloud broker
- Backend subscribes from cloud (configured in .env.local)
- Enables remote developer access to live tag stream
- Cost: ~$1-2/month
- Alternative: Run local EMQX on Portainer for isolated testing
```

## Validation Criteria

**Docker Compose:**
- [ ] `docker-compose up` starts cleanly
- [ ] TimescaleDB container running with health check passing
- [ ] Port 5432 accessible from host
- [ ] Volume persists data between restarts

**Database Schema:**
- [ ] All 12 init scripts execute without errors
- [ ] `trakrf` schema exists
- [ ] All tables created (accounts, users, locations, devices, antennas, assets, tags, events, messages)
- [ ] TimescaleDB extension enabled
- [ ] Sample data loaded (99-sample-data.sql)

**Environment Configuration:**
- [ ] `.env.local.example` exists with all required vars
- [ ] `.envrc` loads .env.local with direnv
- [ ] DATABASE_URL includes URL-encoded password
- [ ] PG_URL uses container name `timescaledb`
- [ ] MQTT vars point to working cloud broker

**Just Commands:**
- [ ] `just db-up` starts database successfully
- [ ] `just db-down` stops cleanly
- [ ] `just db-logs` shows logs
- [ ] `just db-shell` connects to psql
- [ ] `just db-reset` destroys and recreates database
- [ ] `just db-status` reports health

**Documentation:**
- [ ] README.md documents docker commands
- [ ] README.md explains external MQTT pattern
- [ ] Comments in .env.local.example explain each section

## Success Metrics

**Infrastructure:**
- Database starts in < 10 seconds
- All init scripts execute without errors
- Database accessible at localhost:5432
- Health check passes consistently

**Developer Experience:**
- Single command (`just db-up`) starts everything
- One .env.local file to configure (easy onboarding)
- Clear documentation for all commands
- `psql` access for manual queries

**Overall Success:**
- ✅ Developer can run `just db-up` and have working database
- ✅ Schema matches trakrf-web exactly
- ✅ Ready for backend development (Phase 2)

## Migration Strategy

**Phase 1 (This Spec):**
1. Create `database/init/` directory
2. Copy all 12 SQL files from trakrf-web as-is
3. Create docker-compose.yaml
4. Create .env.local.example with working MQTT/DB values
5. Update justfile with docker commands
6. Update README.md
7. Validate: `just db-up` works

**Phase 2 (Next Spec - Backend Bootstrap):**
- Initialize Go module
- MQTT subscriber connects to cloud broker
- Parse messages, insert to local TimescaleDB
- Uses SQL migrations (copied from init scripts)

**Phase 3+ (Future):**
- Transition to Go migrations (golang-migrate)
- Refactor schema as needed
- All changes tracked in migration history

## Key Decisions

**External MQTT Broker:**
- Rationale: Cloud broker enables remote dev collaboration, readers stay configured
- Cost: ~$1-2/month (negligible)
- Alternative: Local EMQX on Portainer for offline testing

**Copy SQL Scripts As-Is:**
- Rationale: Get working database first, refactor later
- Migration path: Phase 2 uses these scripts, Phase 3+ converts to Go migrations

**Unified .env.local:**
- Rationale: Single source of truth, easier onboarding
- Security: Vite prefix filtering keeps secrets server-side
- Duplication: DATABASE_PASSWORD + DATABASE_URL acceptable (URL encoding required)

**No Backend in This Phase:**
- Rationale: Clean separation - infrastructure first, then code
- Next step: Backend bootstrap spec will use this foundation

## Out of Scope

❌ Go backend code (Phase 2)
❌ MQTT ingester logic (Phase 2)
❌ Schema modifications (Phase 3+)
❌ Frontend changes (already complete)
❌ Local EMQX broker (external by design)
❌ Migration to Go migrations (Phase 3+)

## Estimated Scope

**Files to create:**
- docker-compose.yaml (1 file)
- .env.local.example (1 file)
- database/init/*.sql (12 files copied)

**Files to modify:**
- justfile (add docker commands)
- README.md (add local development section)
- .gitignore (ensure .env.local excluded)

**Complexity:** 2/10 (straightforward migration + docker setup)
**Time:** 30-45 minutes
