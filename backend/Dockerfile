# Development image used by docker-compose `backend` service for the
# Air hot-reload loop. The real production image is the root Dockerfile —
# CI (`.github/workflows/docker-build.yml`) and Railway (`railway.json`)
# both build from it. This file is intentionally development-only;
# the previous `production` target here was unused and drifted (stale
# `0.1.0-dev` version literal) — removed in TRA-485.
FROM golang:1.25-alpine AS development
WORKDIR /app

# Install Air for hot-reload
RUN go install github.com/air-verse/air@v1.63.0

# Copy go.mod and go.sum for dependency management
COPY go.mod go.sum ./
RUN go mod download

# Install swag CLI for the pre-build spec generation
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

# Copy source code
COPY . .

# Generate OpenAPI specs — required by //go:embed in internal/handlers/swaggerspec
RUN swag init -g main.go --parseDependency --parseInternal && \
    mkdir -p internal/handlers/swaggerspec && \
    go run ./internal/tools/apispec \
        --in docs/swagger.json \
        --public-out internal/handlers/swaggerspec/openapi.public \
        --internal-out internal/handlers/swaggerspec/openapi.internal

# Pre-build the application to populate build cache and verify all dependencies
RUN go build -o ./tmp/server .

# Run Air (reads .air.toml config)
CMD ["air", "-c", ".air.toml"]
