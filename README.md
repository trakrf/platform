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
```

**4. Stop services**
```bash
# Stop all services
just dev-stop

# Or stop database
just db-down
```

### Development Workflow

This project uses Just's delegation pattern for monorepo task management.

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

### Docker Commands

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
# Run backend natively (from backend/ directory)
cd backend && just dev       # Starts at localhost:8080

# Or use delegation from root
just backend dev             # Starts at localhost:8080

# Run validation (from root with delegation)
just backend lint            # Format + lint
just backend test            # Run tests
just backend build           # Build binary
just backend validate        # All checks
```

**Note:** Docker is the recommended workflow. Native commands are available for those who prefer it.

### Validation

Run validation checks using the delegation pattern:
```bash
# Full validation (lint, test, build)
just validate

# Combined checks across all workspaces
just lint        # Lint backend + frontend
just test        # Test backend + frontend
just build       # Build backend + frontend

# Backend validation (delegation from root)
just backend validate     # All backend checks
just backend lint         # go fmt + go vet
just backend test         # go test
just backend build        # go build

# Frontend validation (delegation from root)
just frontend validate    # All frontend checks
just frontend lint        # ESLint
just frontend typecheck   # TypeScript
just frontend test        # Vitest unit tests
just frontend build       # Vite production build

# Workspace-specific (from workspace directory)
cd backend && just validate   # Backend-only validation
cd frontend && just validate  # Frontend-only validation
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

## REST API

**Phase 4A - Foundation + User Management** (v1)

The backend exposes RESTful JSON APIs under `/api/v1`:

### Health Checks
```bash
GET /healthz   # K8s liveness probe
GET /readyz    # K8s readiness probe (with DB check)
GET /health    # Human-friendly health status (JSON)
```

### Accounts
```bash
GET    /api/v1/accounts         # List all accounts (paginated)
GET    /api/v1/accounts/:id     # Get account by ID
POST   /api/v1/accounts         # Create new account
PUT    /api/v1/accounts/:id     # Update account
DELETE /api/v1/accounts/:id     # Soft delete account
```

### Users
```bash
GET    /api/v1/users            # List all users (paginated)
GET    /api/v1/users/:id        # Get user by ID
POST   /api/v1/users            # Create new user
PUT    /api/v1/users/:id        # Update user
DELETE /api/v1/users/:id        # Soft delete user
```

### Account Users (RBAC Junction)
```bash
GET    /api/v1/accounts/:account_id/users                # List users in account
POST   /api/v1/accounts/:account_id/users                # Add user to account
PUT    /api/v1/accounts/:account_id/users/:user_id       # Update user role/status
DELETE /api/v1/accounts/:account_id/users/:user_id       # Remove user from account
```

**Response Format:**
- Success: `{"data": {...}}` or `{"data": [...], "pagination": {...}}`
- Error: RFC 7807 Problem Details with request ID for tracing

**Pagination:** `?page=1&per_page=20` (default: page 1, 20 items, max 100)

Full API documentation coming soon at `/api/docs`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines on:
- Development setup and workflow
- Code style and architecture patterns
- Testing requirements
- Pull request process
- Commit message conventions

