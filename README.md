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
├── backend/         # Go API server
├── frontend/        # React web app
├── database/        # SQL migrations
├── marketing/       # Marketing website
└── docker-compose.yml
```

## Development

### Prerequisites
- Go 1.21+
- Node.js 18+
- Docker & Docker Compose
- TimescaleDB (via Docker or TigerData cloud)
- Just (command runner) - https://just.systems/

### Quick Start
```bash
# Clone the repo
git clone https://github.com/trakrf/platform
cd platform

# Start dependencies
docker-compose up -d timescaledb

# Run migrations
cd backend && go run cmd/migrate/main.go up

# Start backend
go run cmd/server/main.go

# Start frontend (new terminal)
cd frontend && pnpm install && pnpm start
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

