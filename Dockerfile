# Stage 0: Build Metadata
# Resolves the deployed commit, tag, and platform version.
#
# Platform version (TRA-485, redesigned by TRA-1126): the version is DECLARED in
# the root VERSION file and COPY'd in here — not derived from git ref topology,
# and not chosen by the caller. That is what makes it a property of the commit:
# identical for both arches of a multi-arch build, identical for CI, a local
# `docker build` and Railway, and unchanged by a re-run after CI mints the
# release tag. `git describe` used to fill this role and broke three releases
# doing it — see docs/adr/0004-declared-platform-version.md.
#
# VERSION_SUFFIX is appended for builds that are not a plain build of the
# commit: docker-build.yml passes `-preview+419+420` on the preview branch so
# the UI names the PRs in that composition (TRA-851) and a non-technical viewer
# can tell preview's release line from prod's. It can only make a version LESS
# clean, never more.
#
# APP_VERSION survives as an explicit dev override and as the value stamped into
# the OCI label further down (a LABEL cannot read a file). If it is set and
# disagrees with the declared version, this stage FAILS the build rather than
# shipping an image whose label and /health disagree.
#
# COMMIT_SHA / BUILD_TAG still come from the caller, falling back to Railway's
# injected RAILWAY_* args — Railway sets these for any ARG declared with a
# matching name (per https://docs.railway.com/guides/dockerfiles), so no
# railway.json dockerBuildArgs indirection is needed — then to "unknown"/"dev".
# TRA-760 F2, TRA-485, TRA-1126.
FROM alpine:3.20 AS build-meta
ARG COMMIT_SHA=
ARG BUILD_TAG=
ARG APP_VERSION=
ARG VERSION_SUFFIX=
ARG RAILWAY_GIT_COMMIT_SHA=
ARG RAILWAY_GIT_BRANCH=
COPY VERSION /VERSION
RUN set -eu; \
    declared=$(tr -d '[:space:]' < /VERSION); \
    if ! echo "${declared}" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then \
      echo "VERSION is '${declared}', which is not bare semver (e.g. 1.5.0-dev)." >&2; \
      exit 1; \
    fi; \
    version="v${declared}${VERSION_SUFFIX}"; \
    if [ -n "${APP_VERSION}" ] && [ "${APP_VERSION}" != "${version}" ]; then \
      echo "APP_VERSION build-arg is '${APP_VERSION}' but this commit declares '${version}'." >&2; \
      echo "The platform version is a property of the commit (TRA-1126); it cannot be injected." >&2; \
      exit 1; \
    fi; \
    printf '%s' "${COMMIT_SHA:-${RAILWAY_GIT_COMMIT_SHA:-unknown}}" > /commit; \
    printf '%s' "${BUILD_TAG:-${RAILWAY_GIT_BRANCH:-dev}}" > /tag; \
    printf '%s' "${version}" > /version

# Stage 1: Frontend Builder
FROM node:24-alpine AS frontend-builder
WORKDIR /app

# Build-time args for Vite (must be available when frontend builds)
# NOTE: the environment banner is no longer baked at build time — it's
# runtime-driven from the backend's ENVIRONMENT_LABEL env var (TRA-853).
ARG VITE_SENTRY_DSN
ENV VITE_SENTRY_DSN=$VITE_SENTRY_DSN

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
# main.version is sourced from /version, which build-meta derives from the
# committed VERSION file (TRA-1126), so /health, the frontend nav header and
# the OCI label all report one string for a given commit.
# TRA-760 F2, TRA-485, TRA-1126.
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

# TRA-1085: republish the platform version as an OCI label so it can be read
# straight off the registry with `docker buildx imagetools inspect`, without
# pulling and running the image. promote-prod uses it to refuse an image that is
# not a release build. Until TRA-1085 the version survived only as a Go -ldflags
# value inside the binary, which is unreadable from a manifest.
#
# Deliberately NOT org.opencontainers.image.version: docker/metadata-action
# already emits that key holding the image *tag* (sha-aa9822b), and
# build-push-action applies its labels after this one, so the value here would
# be silently overwritten with the wrong thing.
#
# A LABEL cannot read a file, so this is the one place the version arrives as a
# build-arg. It still cannot disagree with /health: build-meta above FAILS the
# build when APP_VERSION is set and differs from the declared VERSION. A build
# that passes no APP_VERSION — a bare local `docker build`, or Railway — gets an
# empty label and is therefore unpromotable, which is the correct fail-closed
# behaviour: promotable images come from CI. TRA-1126.
ARG APP_VERSION=
LABEL id.trakrf.app-version="${APP_VERSION}"

WORKDIR /app

# Copy server binary (migrations are embedded via go:embed)
COPY --from=backend-builder /app/backend/server /server

EXPOSE 8080

CMD ["/server"]
