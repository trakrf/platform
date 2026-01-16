# Stage 1: Frontend Builder
FROM node:24-alpine AS frontend-builder
WORKDIR /app

# Build-time args for Vite (must be available when frontend builds)
ARG VITE_SENTRY_DSN
ARG APP_ENV
ENV VITE_SENTRY_DSN=$VITE_SENTRY_DSN
# Derive VITE_ENVIRONMENT from APP_ENV (single source of truth)
ENV VITE_ENVIRONMENT=$APP_ENV

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

# Copy go.mod for layer caching
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

# Copy server binary (migrations are embedded via go:embed)
COPY --from=backend-builder /app/backend/server /server

EXPOSE 8080

CMD ["/server"]
