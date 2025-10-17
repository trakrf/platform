```markdown
# TrakRF Platform

RFID/BLE asset tracking platform for manufacturing and logistics.

## Architecture

**Stack:**
- Backend: Go
- Frontend: React
- Database: TimescaleDB
- MQTT: Integrated broker
- Deployment: Docker

**Structure:**
```
platform/
├── backend/            # Go API server (placeholder)
├── frontend/           # React web app (@trakrf/frontend - handheld RFID app)
├── database/
│   └── migrations/     # Versioned SQL migrations (golang-migrate)
├── docs/               # Documentation
│   └── frontend/       # Frontend docs (architecture, vendor specs)
├── marketing/          # Marketing website
└── docker-compose.yml
```

**Frontend**: Full-featured React + TypeScript app for CS108 RFID handheld readers with Web Bluetooth integration, comprehensive test suite (Vitest + Playwright), and mock testing support. See [docs/frontend/](docs/frontend/) for architecture and vendor specs.

## Development

### Prerequisites
- Docker & Docker Compose
- Just (task runner) - https://just.systems/
- direnv (optional but recommended - auto-loads `.env.local`)

**Note:** Go and Node.js are NOT required for Docker-based development. Install them only if you want to run services natively.

### Quick Start (Docker-First)

**1. Configure environment**
```bash
# Create .env file (required for Docker Compose)
cat > .env << EOF
POSTGRES_PASSWORD=postgres
POSTGRES_DB=postgres
PG_URL=postgresql://postgres:postgres@timescaledb:5432/postgres?sslmode=disable
BACKEND_PORT=8080
BACKEND_LOG_LEVEL=info
EOF

# For additional configuration, use .env.local (optional)
# cp .env.local.example .env.local
# Edit .env.local for MQTT credentials, etc.

# Enable direnv (auto-loads .env.local)
direnv allow
```

**2. Start full stack**
```bash
# Start database + backend with hot-reload
# This will:
#   1. Start TimescaleDB
#   2. Run database migrations automatically
#   3. Start backend with Air hot-reload
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

**Database Migrations:**
```bash
just db-migrate-up         # Apply all pending migrations
just db-migrate-down       # Rollback last migration
just db-migrate-status     # Show current migration version
just db-migrate-create foo # Create new migration pair (000XXX_foo.up/down.sql)
just db-migrate-force 5    # Force version to 5 (recovery only)
```

Migrations run automatically on `just dev` startup. Manual migration commands are useful for:
- Production deployments
- Testing migration rollback
- Creating new migrations
- Recovery from failed migrations

### Native Development (Optional)

If you have Go 1.25+ installed, you can run backend natively:

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

### Validation

Run validation checks:
```bash
# Full validation (lint, test, build)
just validate

# Individual checks
just lint        # Lint backend + frontend
just test        # Test backend + frontend
just build       # Build backend + frontend

# Backend-only validation (native)
just backend           # Lint + test + build
just backend-lint      # go fmt + go vet
just backend-test      # go test
just backend-build     # go build

# Frontend-only validation
just frontend           # Lint + typecheck + test + build
just frontend-lint      # ESLint
just frontend-typecheck # TypeScript
just frontend-test      # Vitest unit tests
just frontend-build     # Vite production build
```

See `justfile` for all available commands.

## Features

### MVP (Current)
- [ ] JWT authentication
- [ ] Asset management (CRUD)
- [ ] Location tracking via RFID/BLE
- [ ] MQTT ingestion pipeline
- [ ] Real-time asset location queries

### Roadmap
- [ ] Continuous aggregates for historical data
- [ ] REST API with key authentication
- [ ] ERP/WMS integrations
- [ ] Multi-tenant isolation
- [ ] Self-hosted deployment options

## Deployment

### SaaS (Railway)
```bash
# Single container deployment
docker build -t trakrf/platform .
railway up
```

### Self-Hosted
```bash
# Customer deployment via docker-compose
docker-compose -f docker-compose.prod.yml up
```

## License

Business Source License (BSL) - see [LICENSE](LICENSE)

## API Documentation

API documentation available at `/api/docs` when running locally.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines on:
- Development setup and workflow
- Code style and architecture patterns
- Testing requirements
- Pull request process
- Commit message conventions

