# Development stage (with Air hot-reload)
FROM golang:1.25-alpine AS development
WORKDIR /app

# Install Air for hot-reload
RUN go install github.com/air-verse/air@v1.63.0

# Copy go.mod and go.sum for dependency management
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

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

# Copy source and build
COPY . .
RUN go build -ldflags "-X main.version=0.1.0-dev" -o server .

# Final production image
FROM alpine:3.20 AS production
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy server binary (migrations are embedded via go:embed)
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
