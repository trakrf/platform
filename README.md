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
├── backend/         # Go API server (placeholder)
├── frontend/        # React web app (@trakrf/frontend - handheld RFID app)
├── database/        # SQL migrations
├── docs/            # Documentation
│   └── frontend/    # Frontend docs (architecture, vendor specs)
├── marketing/       # Marketing website
└── docker-compose.yml
```

**Frontend**: Full-featured React + TypeScript app for CS108 RFID handheld readers with Web Bluetooth integration, comprehensive test suite (Vitest + Playwright), and mock testing support. See [docs/frontend/](docs/frontend/) for architecture and vendor specs.

## Development

### Prerequisites
- Go 1.21+
- Node.js 24+ (we recommend [nvm](https://github.com/nvm-sh/nvm) or [fnm](https://github.com/Schniz/fnm))
- [pnpm](https://pnpm.io/) (package manager for frontend)
- Docker & Docker Compose
- TimescaleDB (via Docker or TigerData cloud)
- Just (command runner) - https://just.systems/
- [direnv](https://direnv.net/) (optional, for auto-loading `.env.local`)

### Quick Start
```bash
# Clone the repo
git clone https://github.com/trakrf/platform
cd platform

# Set up Node version (if using nvm/fnm)
nvm use  # or: fnm use

# Set up environment (if using direnv)
cp .env.local.example .env.local
direnv allow  # auto-loads .env.local when cd'ing into directory

# Start dependencies
docker-compose up -d timescaledb

# Run migrations
cd backend && go run cmd/migrate/main.go up

# Start backend
go run cmd/server/main.go

# Start frontend (new terminal)
cd frontend
pnpm install  # Already done if you've run validate
pnpm dev      # Dev server with hot reload
# OR: pnpm dev:mock  # Dev server with BLE mock (no hardware needed)
```

**Note:** This project uses `pnpm` exclusively for frontend dependency management.

### Validation

Run validation checks:
```bash
# Full validation (lint, test, build)
just validate

# Individual checks
just lint        # Lint backend + frontend
just test        # Test backend + frontend
just build       # Build backend + frontend

# Frontend-only validation
just frontend           # Lint + typecheck + test + build
just frontend-lint      # ESLint
just frontend-typecheck # TypeScript
just frontend-test      # Vitest unit tests
just frontend-build     # Vite production build
```

See `justfile` for all available commands.

### Environment Variables
```bash
# Backend (.env)
DATABASE_URL=postgres://user:pass@localhost/trakrf
JWT_SECRET=your-secret-key
MQTT_BROKER=tcp://localhost:1883

# Frontend (.env)
REACT_APP_API_URL=http://localhost:8080
```

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

