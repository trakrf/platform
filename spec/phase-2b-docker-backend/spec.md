# Spec: Phase 2B - Docker Backend Integration

**Author:** Mike Stankavich
**Date:** 2025-10-17
**Status:** Draft
**Phase:** 2B of Epic (TrakRF Platform Migration)
**Workspace:** backend

## Overview

Containerize the Go backend HTTP server (from Phase 2A) and integrate it into the existing docker-compose development environment. Enable hot-reload for rapid development iteration.

## Context

**What we have (Phase 2A):**
- Working Go HTTP server with health endpoints
- `just backend` commands (lint, test, build, run)
- Binary builds to `backend/server` with version injection
- TimescaleDB running in docker-compose

**What we need (Phase 2B):**
- Dockerfile for Go backend (multi-stage build)
- Backend service in docker-compose.yaml
- Air hot-reload for development
- Integration with TimescaleDB service
- Updated Just commands for containerized workflow

**Why now:**
- Complete the "Docker Dev Environment" epic
- Enable full-stack local development (database + backend in containers)
- Prepare for Phase 3 (database migrations via backend)
- Mirror production deployment patterns

## Goals

### Primary Goals
1. **Containerize Go backend** - Production-ready Dockerfile
2. **Development hot-reload** - Air for instant code changes
3. **Service integration** - Backend ↔ TimescaleDB connectivity
4. **Developer ergonomics** - Single `just dev` command to run full stack

### Non-Goals
- ❌ Frontend containerization (separate future phase)
- ❌ Multi-stage build optimization (good to have, but not required)
- ❌ Docker secrets management (use .env.local for now)
- ❌ Production Kubernetes configs (Railway handles deployment)

## Success Metrics

### Functional Requirements
- [ ] `just backend-dev` starts containerized backend with hot-reload
- [ ] Backend container connects to TimescaleDB container
- [ ] Health endpoints accessible at http://localhost:8080
- [ ] Code changes trigger automatic reload (< 5 second cycle)
- [ ] Backend logs visible via `docker-compose logs -f backend`
- [ ] `just backend-stop` stops containers cleanly

### Quality Requirements
- [ ] Dockerfile follows best practices (multi-stage, minimal layers)
- [ ] Image size < 50MB (Go produces small binaries)
- [ ] Development image includes Air and source mounting
- [ ] Production image is standalone (contains compiled binary only)
- [ ] .dockerignore excludes unnecessary files

### Integration Requirements
- [ ] Backend service defined in docker-compose.yaml
- [ ] Backend depends_on TimescaleDB with health check
- [ ] Backend uses PG_URL from .env.local
- [ ] Backend and TimescaleDB on same Docker network
- [ ] Port 8080 exposed for health checks

## User Stories

### Story 1: Backend Developer Starting Work
**As a** backend developer
**I want to** run `just backend-dev`
**So that** I have backend + database running with hot-reload

**Acceptance Criteria:**
- Single command starts both services
- Backend auto-reloads on code changes
- Logs stream to terminal
- Services stop cleanly with Ctrl+C

### Story 2: Full-Stack Developer Testing Integration
**As a** full-stack developer
**I want to** test frontend → backend → database flow
**So that** I can develop features end-to-end locally

**Acceptance Criteria:**
- Backend accessible at localhost:8080
- Backend can query TimescaleDB
- Health endpoints return 200 OK
- Database connection visible in logs

### Story 3: New Team Member Onboarding
**As a** new developer
**I want to** follow README instructions
**So that** I can run the platform without Go/PostgreSQL installation

**Acceptance Criteria:**
- README documents `just backend-dev` workflow
- Only Docker required (no Go installation needed for dev)
- .env.local.example has backend environment variables
- Clear error messages if services fail

## Technical Design

### Architecture

```
┌─────────────────────────────────────────────────┐
│ Docker Compose Development Environment          │
├─────────────────────────────────────────────────┤
│                                                  │
│  ┌──────────────────┐      ┌─────────────────┐ │
│  │   TimescaleDB    │      │   Go Backend    │ │
│  │   (Phase 1)      │◄─────┤   (Phase 2B)    │ │
│  │                  │      │                 │ │
│  │  Port: 5432      │      │  Port: 8080     │ │
│  │  Volume: data    │      │  Volume: src    │ │
│  └──────────────────┘      └─────────────────┘ │
│                                 │               │
│                                 ▼               │
│                            Air Hot-Reload       │
│                            (Development)        │
└─────────────────────────────────────────────────┘
              │
              ▼
        localhost:8080
        (Health Endpoints)
```

### Files to Create

**1. `backend/Dockerfile`**
```dockerfile
# Development stage (with Air hot-reload)
FROM golang:1.21-alpine AS development
WORKDIR /app
RUN go install github.com/cosmtrek/air@latest
COPY go.mod go.sum ./
RUN go mod download
COPY . .
CMD ["air", "-c", ".air.toml"]

# Production stage (standalone binary)
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags "-X main.version=0.1.0-dev" -o server .

FROM alpine:latest AS production
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
```

**2. `backend/.air.toml`**
```toml
# Air hot-reload configuration
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/server ."
  bin = "./tmp/server"
  delay = 1000
  exclude_dir = ["tmp", "vendor"]
  include_ext = ["go", "tpl", "tmpl", "html"]
  stop_on_error = true

[log]
  time = true

[color]
  main = "magenta"
  watcher = "cyan"
  build = "yellow"
  runner = "green"
```

**3. `backend/.dockerignore`**
```
# Binaries
server
backend
tmp/

# Build artifacts
*.test
*.out

# Development files
.air.toml.local
.env
.env.local

# IDE
.vscode/
.idea/

# Git
.git/
.gitignore
```

### Files to Modify

**4. `docker-compose.yaml`** (add backend service)
```yaml
services:
  timescaledb:
    # ... existing config ...

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

**5. `justfile`** (add Docker commands)
```makefile
# Docker development commands
backend-dev:
    docker-compose up -d backend
    @echo "🚀 Backend running at http://localhost:8080"
    @echo "📊 Health check: curl localhost:8080/health"
    docker-compose logs -f backend

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

**6. `.env.local.example`** (add backend vars)
```bash
# Backend Service
PORT=8080
LOG_LEVEL=info
```

**7. `README.md`** (add Docker development section)
```markdown
## Docker Development

### Quick Start
```bash
# Start full stack (database + backend)
just dev

# Start backend only (requires db running)
just backend-dev

# View logs
just dev-logs

# Stop all services
just dev-stop
```

### Backend Development
- Code changes auto-reload via Air
- Logs stream to terminal
- Health check: http://localhost:8080/health
```

## Implementation Plan

### Phase 2B Tasks (Estimated: 6 tasks, 2-3 hours)

1. **Create Dockerfile** (15 min)
   - Multi-stage build (development + production)
   - Install Air in development stage
   - Copy and build Go code

2. **Create Air configuration** (10 min)
   - .air.toml with watch patterns
   - Build and restart on .go file changes

3. **Create .dockerignore** (5 min)
   - Exclude binaries, build artifacts, IDE files

4. **Update docker-compose.yaml** (15 min)
   - Add backend service
   - Configure environment variables
   - Setup volumes for hot-reload
   - Add depends_on with health check

5. **Update justfile** (15 min)
   - Add backend-dev, backend-stop, backend-restart
   - Add backend-shell for debugging
   - Add dev (full stack) and dev-stop

6. **Update documentation** (20 min)
   - README.md with Docker workflow
   - .env.local.example with backend vars
   - Update backend/README.md with containerized workflow

### Validation Strategy

**After each task:**
```bash
# Validate Dockerfile
docker build -f backend/Dockerfile --target development -t backend:dev ./backend
docker build -f backend/Dockerfile --target production -t backend:prod ./backend

# Validate docker-compose
docker-compose config

# Validate services
just backend-dev
curl localhost:8080/health
# Make a code change, verify reload
# Check logs show rebuild
```

**Full validation:**
```bash
# Test full workflow
just dev              # Start full stack
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/health
# Edit backend/main.go, verify reload
just dev-logs         # Check logs
just dev-stop         # Clean shutdown
```

## Dependencies

**Required:**
- ✅ Phase 1: Docker Dev Environment (TimescaleDB running)
- ✅ Phase 2A: Go Backend Core (working HTTP server)
- ✅ Docker and docker-compose installed
- ✅ .env.local configured

**Blocked by:**
- None (ready to start)

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Air hot-reload slow on file changes | Medium | Configure Air with 1s delay, exclude tmp/ |
| Docker image size bloated | Low | Multi-stage build, alpine base |
| Backend can't connect to TimescaleDB | High | Use depends_on + health check, test connectivity |
| Port conflicts (8080 in use) | Low | Make port configurable via .env.local |
| Hot-reload breaks on syntax errors | Medium | Air handles this, just fix and save again |

## Open Questions

1. **Should we use Air in production?**
   - No - production uses standalone binary from multi-stage build
   - Air is development-only (target: development)

2. **Should backend auto-migrate database on startup?**
   - No - Phase 3 will handle migrations explicitly
   - Phase 2B just needs connectivity for health checks

3. **Should we expose backend port externally (0.0.0.0)?**
   - Yes - developers need localhost:8080 access
   - Production Railway handles external exposure

4. **Should we add healthcheck to backend service in docker-compose?**
   - Optional but recommended
   - Would allow frontend service to depend_on backend later

## References

- Phase 2A Spec: `spec/phase-2-go-backend-baseline/spec.md`
- Phase 1 Spec: Docker Compose Dev Environment (merged)
- Air Documentation: https://github.com/cosmtrek/air
- Docker Multi-Stage Builds: https://docs.docker.com/build/building/multi-stage/
- Go Docker Best Practices: https://chemidy.medium.com/create-the-smallest-and-secured-golang-docker-image-based-on-scratch-4752223b7324

## Future Work (Post Phase 2B)

- Phase 3: Database migrations (go-migrate)
- Phase 4: REST API framework
- Phase 5: Authentication
- Frontend containerization (separate phase)
- Production docker-compose (Railway uses native Go, not Docker)

## Approval

- [ ] Spec reviewed
- [ ] Implementation plan approved
- [ ] Ready for `/plan` to generate detailed tasks
