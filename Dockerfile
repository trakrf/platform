# Stage 1: Frontend Builder
FROM node:24-alpine AS frontend-builder
WORKDIR /app

# Build-time args for Vite (must be available when frontend builds)
ARG VITE_SENTRY_DSN
ARG APP_ENV
ENV VITE_SENTRY_DSN=$VITE_SENTRY_DSN
# Derive VITE_ENVIRONMENT from APP_ENV (single source of truth)
ENV VITE_ENVIRONMENT=$APP_ENV

# Build metadata — same values passed to the backend stage. Exposed as VITE_*
# so the Vite plugin can emit dist/version.json for curl-able drift detection.
ARG COMMIT_SHA=unknown
ARG BUILD_TAG=dev
ENV VITE_COMMIT_SHA=$COMMIT_SHA
ENV VITE_BUILD_TAG=$BUILD_TAG

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

# Build-time metadata injected via -ldflags so /health can report the
# deployed commit. Defaults keep `docker build` working without --build-arg.
ARG COMMIT_SHA=unknown
ARG BUILD_TAG=dev
ARG VERSION=0.1.0-preview

# Copy go.mod for layer caching
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Install swag CLI for generating Swagger docs
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

# Copy backend source
COPY backend/ .

# Generate Swagger 2.0 spec (docs directory is gitignored; swag emits docs/swagger.json)
RUN swag init -g main.go --parseDependency --parseInternal

# Generate the OpenAPI 3.0 specs that swaggerspec embeds via go:embed.
# Both public and internal specs are embedded into the binary; CI owns the
# drift check against the committed copy in docs/api/.
RUN mkdir -p internal/handlers/swaggerspec && \
    go run ./internal/tools/apispec \
        --in docs/swagger.json \
        --public-out internal/handlers/swaggerspec/openapi.public \
        --internal-out internal/handlers/swaggerspec/openapi.internal

# Copy frontend dist to expected location for go:embed
# go:embed at backend/main.go:27 expects backend/frontend/dist
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist

# Build server with build metadata injected via ldflags. BUILD_TIME is
# evaluated inside the container so it reflects the actual build, not the
# invocation of docker build.
RUN BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) && \
    CGO_ENABLED=0 GOOS=linux go build \
        -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT_SHA} -X main.tag=${BUILD_TAG} -X main.buildTime=${BUILD_TIME}" \
        -o server .

# Stage 3: Production
FROM alpine:3.20 AS production
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy server binary (migrations are embedded via go:embed)
COPY --from=backend-builder /app/backend/server /server

EXPOSE 8080

CMD ["/server"]
