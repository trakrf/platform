# Development stage (with Air hot-reload)
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
        --public-out /tmp/openapi.public \
        --internal-out internal/handlers/swaggerspec/openapi.internal

# Pre-build the application to populate build cache and verify all dependencies
RUN go build -o ./tmp/server .

# Run Air (reads .air.toml config)
CMD ["air", "-c", ".air.toml"]

# Production stage (standalone binary)
FROM golang:1.25-alpine AS builder
WORKDIR /app

# Copy go.mod and go.sum for dependency management
COPY go.mod go.sum ./
RUN go mod download

# Install swag CLI for spec generation
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

# Copy source and build
COPY . .

# Generate OpenAPI specs — required by //go:embed in internal/handlers/swaggerspec
RUN swag init -g main.go --parseDependency --parseInternal && \
    mkdir -p internal/handlers/swaggerspec && \
    go run ./internal/tools/apispec \
        --in docs/swagger.json \
        --public-out /tmp/openapi.public \
        --internal-out internal/handlers/swaggerspec/openapi.internal

RUN go build -ldflags "-X main.version=0.1.0-dev" -o server .

# Final production image
FROM alpine:3.20 AS production
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy server binary (migrations are embedded via go:embed)
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
