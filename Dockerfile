# Stage 1: Frontend Builder
FROM node:24-alpine AS frontend-builder
WORKDIR /app

# Install pnpm
RUN npm install -g pnpm@latest

# Copy workspace configuration files
COPY pnpm-workspace.yaml .npmrc pnpm-lock.yaml ./

# Copy package files for layer caching
COPY frontend/package.json ./frontend/
RUN pnpm install --frozen-lockfile

# Copy source and build
COPY frontend/ ./frontend/
RUN pnpm --filter frontend run build
# Output: /app/frontend/dist

# Stage 2: Backend Builder
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app/backend

# Install build dependencies
RUN apk add --no-cache wget tar

# Install migrate CLI (pattern from backend/Dockerfile:9-12)
RUN wget -qO- https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | \
    tar xvz && \
    mv migrate /usr/local/bin/migrate && \
    chmod +x /usr/local/bin/migrate

# Copy go.mod for layer caching (pattern from backend/Dockerfile:28-30)
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Install swag CLI for generating Swagger docs
RUN go install github.com/swaggo/swag/cmd/swag@latest

# Copy backend source
COPY backend/ .

# Generate Swagger documentation (docs directory is gitignored)
RUN swag init -g main.go --parseDependency --parseInternal

# Copy frontend dist to expected location for go:embed
# go:embed at backend/main.go:27 expects backend/frontend/dist
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist

# Build server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.version=0.1.0-preview" -o server .

# Stage 3: Production
FROM alpine:3.20 AS production
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy migrate CLI (pattern from backend/Dockerfile:42)
COPY --from=backend-builder /usr/local/bin/migrate /usr/local/bin/migrate

# Copy server binary
COPY --from=backend-builder /app/backend/server /server

# Copy database migrations (must match entrypoint path)
COPY database/migrations /app/database/migrations

# Copy entrypoint script
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/server"]
