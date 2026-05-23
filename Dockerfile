# Stage 0: Build Metadata
# Resolves the deployed commit, tag, and platform version from one of two sources:
#   1. Explicit caller --build-arg COMMIT_SHA / BUILD_TAG / APP_VERSION (GHA path,
#      see .github/workflows/docker-build.yml — passes github.sha, meta tag, and
#      `git describe --tags --always --dirty`).
#   2. Railway-provided RAILWAY_GIT_COMMIT_SHA / RAILWAY_GIT_BRANCH. Railway
#      injects these into the build environment for any ARG declared with a
#      matching name (per https://docs.railway.com/guides/dockerfiles), so no
#      railway.json dockerBuildArgs indirection is needed.
# Explicit args win; Railway args are the fallback; "unknown"/"dev" if neither.
# Platform version (TRA-485): APP_VERSION is the single source of truth, fed
# into both the Go binary (-X main.version) and the frontend (VITE_APP_VERSION).
# Railway can't run `git describe` during build (only env vars are injected),
# so we fall back to RAILWAY_GIT_BRANCH — preview UI then shows the branch
# name instead of a full describe, which is acceptable for non-prod surfaces.
# TRA-760 F2, TRA-485.
FROM alpine:3.20 AS build-meta
ARG COMMIT_SHA=
ARG BUILD_TAG=
ARG APP_VERSION=
ARG RAILWAY_GIT_COMMIT_SHA=
ARG RAILWAY_GIT_BRANCH=
RUN printf '%s' "${COMMIT_SHA:-${RAILWAY_GIT_COMMIT_SHA:-unknown}}" > /commit && \
    printf '%s' "${BUILD_TAG:-${RAILWAY_GIT_BRANCH:-dev}}" > /tag && \
    printf '%s' "${APP_VERSION:-${BUILD_TAG:-${RAILWAY_GIT_BRANCH:-dev}}}" > /version

# Stage 1: Frontend Builder
FROM node:24-alpine AS frontend-builder
WORKDIR /app

# Build-time args for Vite (must be available when frontend builds)
ARG VITE_SENTRY_DSN
ARG VITE_ENVIRONMENT=""
ENV VITE_SENTRY_DSN=$VITE_SENTRY_DSN
ENV VITE_ENVIRONMENT=$VITE_ENVIRONMENT

# Build metadata — same values passed to the backend stage. Exposed as VITE_*
# so the Vite plugin can emit dist/version.json for curl-able drift detection
# and the nav header can render the platform version (TRA-485).
COPY --from=build-meta /commit /tag /version /tmp/buildinfo/

# Install pnpm — major-pinned to 9.x. `pnpm@latest` resolved to 10.x in
# May 2026, which gates installs on explicit build-script approval
# (ERR_PNPM_IGNORED_BUILDS) and breaks the Docker build despite .npmrc
# carrying ignore-scripts=true. `pnpm@9` floats minor/patch within 9.x
# so we still pick up bugfixes; the project's `packageManager` field
# pins exactly for local dev.
RUN npm install -g pnpm@9

# Copy workspace configuration files
COPY pnpm-workspace.yaml .npmrc pnpm-lock.yaml ./

# Copy package files for layer caching
COPY frontend/package.json ./frontend/
RUN pnpm install --frozen-lockfile

# Copy source and build
COPY frontend/ ./frontend/
RUN VITE_COMMIT_SHA=$(cat /tmp/buildinfo/commit) \
    VITE_BUILD_TAG=$(cat /tmp/buildinfo/tag) \
    VITE_APP_VERSION=$(cat /tmp/buildinfo/version) \
    pnpm --filter frontend run build
# Output: /app/frontend/dist

# Stage 2: Backend Builder
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app/backend

# Build-time metadata injected via -ldflags so /health can report the
# deployed commit + platform version. Values come from build-meta;
# main.version is sourced from /version (git-describe at CI time per
# TRA-485) so /health and the frontend nav header stay in sync.
# TRA-760 F2, TRA-485.
COPY --from=build-meta /commit /tag /version /tmp/buildinfo/

# Copy go.mod for layer caching
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Install swag CLI for generating Swagger docs
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

# Copy backend source
COPY backend/ .

# Stub frontend/dist/index.html before swag init so --parseDependency walks
# main.go's //go:embed frontend/dist successfully. Without this, swag falls
# back to fully-qualified Go package names (e.g. internal_handlers_X) and the
# generated swagger.json schema names diverge from the committed public spec
# — and from the requiredFields/nullableFields maps in apispec postprocess.
# The real frontend/dist is copied from frontend-builder a few steps later;
# this stub only exists to keep swag's parser happy. TRA-505.
RUN mkdir -p frontend/dist && touch frontend/dist/index.html

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
    COMMIT_SHA=$(cat /tmp/buildinfo/commit) && \
    BUILD_TAG=$(cat /tmp/buildinfo/tag) && \
    APP_VERSION=$(cat /tmp/buildinfo/version) && \
    CGO_ENABLED=0 GOOS=linux go build \
        -ldflags "-X main.version=${APP_VERSION} -X main.commit=${COMMIT_SHA} -X main.tag=${BUILD_TAG} -X main.buildTime=${BUILD_TIME}" \
        -o server .

# Stage 3: Production
FROM alpine:3.20 AS production
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy server binary (migrations are embedded via go:embed)
COPY --from=backend-builder /app/backend/server /server

EXPOSE 8080

CMD ["/server"]
