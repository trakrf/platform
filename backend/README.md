# Backend (Go)

Go API server for TrakRF platform.

## Structure

```
backend/
├── cmd/           # Application entrypoints
│   ├── server/    # API server
│   └── migrate/   # Database migrations
├── internal/      # Private application code
│   ├── api/       # HTTP handlers
│   ├── service/   # Business logic
│   └── repository/ # Data access
└── pkg/           # Public libraries
```

## Development

### Prerequisites
- Go 1.21+
- TimescaleDB (PostgreSQL)

### Setup
```bash
cd backend

# Install dependencies
go mod download

# Run tests
go test ./...

# Build
go build ./...

# Run server (when implemented)
go run cmd/server/main.go
```

## Validation

### From backend/ directory:
```bash
# Lint
go fmt ./...
go vet ./...

# Test
go test ./...

# Build
go build ./...
```

### From project root (via Just):
```bash
just backend-lint
just backend-test
just backend-build
just backend  # All backend checks
```

## Database

Backend uses TimescaleDB (PostgreSQL with time-series extensions). Connection configuration via environment variables.

## Architecture

- **Clean Architecture** - Separation of concerns (API, service, repository layers)
- **Dependency Injection** - Testable, modular design
- **Context-aware** - All operations support context for cancellation and timeouts
