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

