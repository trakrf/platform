# Development stage (with Air hot-reload)
FROM golang:1.21-alpine AS development
WORKDIR /app

# Install Air for hot-reload - download binary directly to avoid network issues
RUN apk add --no-cache curl && \
    curl -sSfL https://raw.githubusercontent.com/cosmtrek/air/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Copy go.mod only (no go.sum yet - stdlib only project)
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Run Air (reads .air.toml config)
CMD ["air", "-c", ".air.toml"]

# Production stage (standalone binary)
FROM golang:1.21-alpine AS builder
WORKDIR /app

# Copy go.mod and download dependencies
COPY go.mod ./
RUN go mod download

# Copy source and build
COPY . .
RUN go build -ldflags "-X main.version=0.1.0-dev" -o server .

# Final production image
FROM alpine:latest AS production
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
