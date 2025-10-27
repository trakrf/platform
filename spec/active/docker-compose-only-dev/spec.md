# Feature: Docker Compose-Only Development Environment

## Origin
This specification emerged from confusion caused by dual development modes (local + Docker Compose) and follows the justfile delegation refactoring completed in the previous PR. With a clean justfile structure in place, this is the ideal time to standardize on a single development workflow.

## Outcome
Developers use **only** Docker Compose for full-stack development, eliminating confusion from multiple development modes and ensuring consistency between development and deployment environments.

## User Story
**As a developer**
I want a single, consistent development environment using Docker Compose
So that I don't have to choose between local and containerized workflows or debug environment-specific issues

## Context

### Discovery
- Current project has **two parallel development modes**:
  - `just dev` → Docker Compose (backend + database)
  - `just dev-local` → Local processes (parallel frontend + backend)
  - Backend `just dev` → Local go run connecting to docker postgres
  - Backend `just dev-cloud` → Local go run connecting to cloud postgres
  - Frontend `just dev` → Local vite dev server
- User feedback: "finding the mingling of local development and docker compose development to be confusing"
- Project has **NO production users** (development mode only) → breaking changes acceptable
- Fresh justfile refactoring completed → clean slate to standardize

### Current State
```
Root justfile:
- dev          → Docker: database + backend container
- dev-local    → Local: parallel frontend + backend
- dev-stop     → Stop docker services
- dev-logs     → View docker logs

Backend justfile:
- dev          → Local: go run (connects to docker DB or cloud DB)
- dev-cloud    → Local: go run with PG_URL_CLOUD

Frontend justfile:
- dev          → Local: pnpm dev (vite dev server)
```

**Problems**:
1. Developers must decide: local or docker?
2. Different behaviors: local uses host networking, docker uses container networking
3. ENV var confusion: `PG_URL` vs `PG_URL_LOCAL` vs `PG_URL_CLOUD`
4. Inconsistent debugging: backend in docker vs backend on host
5. Multiple ways to achieve the same goal

### Desired State
```
Root justfile:
- dev          → Docker Compose: ALL services (database + backend + frontend)
- dev-stop     → Stop all docker services
- dev-logs     → View logs for all services (or specific service)
- dev-shell    → Shell into backend container (for debugging)

Backend justfile:
- (Remove local dev targets)
- Keep: lint, test, build, validate (run in container or on host for CI)

Frontend justfile:
- (Remove local dev target? Or keep for edge cases?)
- Keep: lint, typecheck, test, build, validate
```

**Key Decision**: Frontend dev experience
- **Option A**: Dockerize frontend (consistency priority)
- **Option B**: Keep frontend local (fast refresh priority)
- **Recommendation**: Start with Option B, revisit if needed

## Technical Requirements

### 1. Remove Local Development Targets

**Backend justfile** - Remove these recipes:
- `dev` (local go run)
- `dev-cloud` (local go run with cloud DB)

**Root justfile** - Remove/replace:
- `dev-local` (parallel local processes)

**Frontend justfile** - Decision needed:
- Keep `dev` for local vite server? (fast refresh, HMR)
- Or dockerize with volume mounts?

### 2. Enhance Docker Compose Configuration

**Validate/fix these services**:

#### Backend Service
```yaml
backend:
  build: ./backend
  ports:
    - "8080:8080"      # API server
  volumes:
    - ./backend:/app          # Hot reload?
    - ./database/migrations:/app/database/migrations  # ✅ Already correct
  environment:
    PG_URL: postgresql://postgres:postgres@timescaledb:5432/postgres
    BACKEND_PORT: 8080
    BACKEND_LOG_LEVEL: debug
  depends_on:
    timescaledb:
      condition: service_healthy
```

**Requirements**:
- ✅ Port forward correct
- ❓ Volume mount for hot reload? (Air or fresh for Go?)
- ✅ ENV vars correct
- ✅ Health check dependency

#### Frontend Service (if dockerized)
```yaml
frontend:
  build: ./frontend
  ports:
    - "5173:5173"      # Vite dev server
  volumes:
    - ./frontend:/app
    - /app/node_modules  # Don't overwrite container node_modules
  environment:
    VITE_API_URL: http://localhost:8080
```

**Requirements**:
- Port forward correct
- Volume mount for HMR (hot module reload)
- Node modules optimization
- Vite config for docker

#### Database Service
```yaml
timescaledb:
  # Already configured
  # Verify health check works
```

### 3. Update Justfile Commands

**Root justfile** - Standardize on Docker Compose:
```just
# Start full development environment (all services)
dev:
    @echo "🚀 Starting Docker Compose development environment..."
    docker compose up -d timescaledb
    @echo "⏳ Waiting for database..."
    @sleep 3
    @just backend migrate
    docker compose up -d backend
    # If frontend dockerized: docker compose up -d frontend
    @echo "✅ Development environment ready"
    @echo "📱 Backend: http://localhost:8080"
    @echo "📱 Frontend: http://localhost:5173"

# Stop all development services
dev-stop:
    docker compose down

# View logs (all services or specific service)
dev-logs service="":
    @if [ -z "{{service}}" ]; then \
        docker compose logs -f; \
    else \
        docker compose logs -f {{service}}; \
    fi

# Shell into backend container
dev-shell:
    docker compose exec -it backend sh
```

### 4. Volume Mounts Validation

**Check these mounts work correctly**:
- ✅ Backend migrations: `./database/migrations:/app/database/migrations` (already confirmed working)
- ❓ Backend hot reload: `./backend:/app` (if enabled)
- ❓ Frontend HMR: `./frontend:/app` + `/app/node_modules` (if dockerized)

**Validation**:
- Edit a backend file → see changes reflected
- Edit a frontend file → HMR updates browser
- Migrations accessible from backend container

### 5. Port Forwards Validation

**Confirm these ports work**:
- `5432:5432` - TimescaleDB (for debugging with psql)
- `8080:8080` - Backend API
- `5173:5173` - Frontend dev server (if dockerized)

**Validation**:
- `curl http://localhost:8080/healthz` → 200 OK
- `curl http://localhost:5173` → Frontend loads (if dockerized)
- `psql -h localhost -U postgres` → connects to TimescaleDB

### 6. Environment Variables Cleanup

**Simplify ENV var strategy**:
- Remove `PG_URL_LOCAL` (no longer needed)
- Remove `PG_URL_CLOUD` (or move to explicit cloud-dev recipe if needed)
- Use `PG_URL` consistently (set by docker-compose.yaml)

**Backend .env.example**:
```bash
# Docker Compose mode (default)
PG_URL=postgresql://postgres:postgres@timescaledb:5432/postgres

# Optional: Cloud testing (explicit override)
# PG_URL_CLOUD=postgresql://user:pass@cloud-host:5432/dbname
```

### 7. Hot Reload Strategy

**Backend** (Go):
- Option A: Use [Air](https://github.com/cosmtrek/air) for hot reload in Docker
- Option B: Keep backend rebuild required (restart container)
- Recommendation: Start without hot reload, add Air if needed

**Frontend** (React):
- Option A: Dockerize with Vite HMR via volume mounts
- Option B: Keep local vite server (unchanged)
- Recommendation: Keep local for now (fast iteration)

## Code Examples

### Updated docker-compose.yaml (backend section)
```yaml
services:
  backend:
    build:
      context: ./backend
      dockerfile: Dockerfile
      target: development  # If using multi-stage
    ports:
      - "8080:8080"
    volumes:
      - ./backend:/app
      - ./database/migrations:/app/database/migrations
    environment:
      PG_URL: postgresql://postgres:postgres@timescaledb:5432/postgres
      BACKEND_PORT: 8080
      BACKEND_LOG_LEVEL: debug
    depends_on:
      timescaledb:
        condition: service_healthy
    # Optional: hot reload with Air
    # command: air -c .air.toml
```

### Updated Root justfile
```just
# ============================================================================
# Full Stack Development (Docker Compose Only)
# ============================================================================

# Start all development services
dev:
    @just database up
    @echo "⏳ Waiting for database to be ready..."
    @sleep 3
    @echo "🔄 Running migrations..."
    @just backend migrate
    @echo "🚀 Starting backend..."
    @docker compose up -d backend
    @echo "✅ Development environment ready"
    @echo "📋 Logs: just dev-logs"
    @echo "🐚 Shell: just dev-shell"

# Stop all services
dev-stop:
    docker compose down

# View logs for all services or specific service
dev-logs service="":
    #!/usr/bin/env bash
    if [ -z "{{service}}" ]; then
        docker compose logs -f
    else
        docker compose logs -f {{service}}
    fi

# Shell into backend container
dev-shell:
    docker compose exec -it backend sh

# Shell into database
db-shell:
    @just database psql
```

### Removed Commands
```just
# REMOVE from root justfile:
dev-local:  # Parallel local development

# REMOVE from backend/justfile:
dev:        # Local go run
dev-cloud:  # Local go run with cloud DB
```

## Testing Strategy

### Pre-implementation Testing
```bash
# Verify current docker compose setup
just dev
curl http://localhost:8080/healthz   # Should return 200 OK
just dev-stop

# Test database connectivity
just database up
just database status
just backend migrate-status
just database down
```

### Post-implementation Testing
```bash
# Start fresh environment
just dev

# Verify services running
docker compose ps
# Should show: timescaledb (healthy), backend (running)

# Test API connectivity
curl http://localhost:8080/healthz      # 200 OK
curl http://localhost:8080/api/v1/accounts  # 401 or valid response

# Test database connectivity from backend
just dev-shell
# Inside container:
psql $PG_URL -c "SELECT 1;"  # Should connect

# Test migrations
just backend migrate-status  # Should show current version

# Test hot reload (if implemented)
# Edit backend/main.go → save → check if service restarts

# Test logs
just dev-logs              # Should show all service logs
just dev-logs backend      # Should show only backend logs
just dev-logs timescaledb  # Should show only DB logs

# Cleanup
just dev-stop
docker compose ps          # Should show no services running
```

## Validation Criteria

### Must Have
- [ ] `just dev` starts all required services (database + backend)
- [ ] `just dev-stop` stops all services cleanly
- [ ] `just dev-logs` shows logs for all services
- [ ] `just dev-logs backend` shows logs for specific service
- [ ] `just dev-shell` provides shell access to backend container
- [ ] Backend API accessible at `http://localhost:8080`
- [ ] Database accessible from backend container
- [ ] Migrations run successfully from root: `just backend migrate`
- [ ] Volume mounts work correctly (migrations accessible)
- [ ] Health checks prevent premature backend start

### Should Have
- [ ] Backend hot reload working (if implemented)
- [ ] Frontend HMR working (if dockerized)
- [ ] Clear developer documentation (README updates)
- [ ] ENV var strategy simplified (no more PG_URL_LOCAL confusion)

### Nice to Have
- [ ] Docker Compose profiles for different scenarios
- [ ] `just dev-restart` command to restart specific service
- [ ] Better error messages if docker not running

## Conversation References

### Key Insights
- User: "finding the mingling of local development and docker compose development to be confusing"
- Context: "Project has NO production users" → breaking changes acceptable
- Timing: "Fresh from justfile refactoring" → clean slate

### Decisions Made
- **Scope**: Docker Compose standardization ONLY
- **Out of scope**: Traefik path-based routing (separate cycle)
- **Approach**: Follow-on PR after delegation refactoring ships
- **Philosophy**: Single development mode for consistency

### Implementation Concerns
- Frontend dev experience: Keep local vite or dockerize?
- Backend hot reload: Worth the complexity?
- Volume mount performance: Fast enough on all platforms?
- Database debugging: Easy access to psql?

## Edge Cases & Constraints

### Edge Cases
1. **Developer wants cloud DB testing**: Keep `backend/justfile` recipe? Or manual override?
2. **CI/CD**: Tests run on host, not in docker (faster) - keep `lint`, `test`, `build` targets
3. **Docker not installed**: Clear error message, suggest installation
4. **M1/M2 Mac performance**: Volume mounts can be slow - document workarounds
5. **Windows paths**: Volume mounts may need tweaking

### Constraints
- **Docker Compose required**: Hard dependency (acceptable for this project)
- **No backward compatibility**: Old local dev workflows will break (acceptable - no users)
- **Performance**: Volume mounts may be slower than native (acceptable tradeoff)

## Related Documents
- Previous PR: Justfile delegation pattern refactoring
- CLAUDE.md: "⚠️ DEVELOPMENT MODE - NO BACKWARD COMPATIBILITY"
- docker-compose.yaml: Current configuration
- Future work: Traefik routing (separate cycle)

## Open Questions

1. **Frontend strategy**:
   - Keep local vite dev server? (fast HMR)
   - Or dockerize for consistency? (environment parity)
   - **Recommendation**: Keep local vite, revisit if issues arise

2. **Backend hot reload**:
   - Implement Air for hot reload?
   - Or keep simple restart workflow?
   - **Recommendation**: Start simple, add if needed

3. **Cloud DB testing**:
   - Remove `dev-cloud` entirely?
   - Or keep as `backend/justfile` recipe for explicit override?
   - **Recommendation**: Document manual override, remove recipe

4. **Documentation location**:
   - Update README.md?
   - Create DEVELOPMENT.md?
   - **Recommendation**: Update README.md "Getting Started" section

## Success Metrics
- Zero confusion about "which dev mode to use"
- Faster onboarding: `git clone → just dev → start coding`
- Consistent environments: Same stack in dev and deployment
- Clear error messages: If docker not running, helpful guidance

## Future Enhancements (Post-MVP)
1. **Traefik Path-Based Routing** - Unified `localhost` domain (separate cycle)
2. **Docker Compose Profiles** - `just dev --profile minimal` for faster startup
3. **Backend Hot Reload** - Air integration for faster iteration
4. **Frontend Dockerization** - Full stack in containers if needed
5. **Dev Containers** - VS Code devcontainer.json for consistency
