# Implementation Plan: Railway Preview Environment
Generated: 2025-01-26
Specification: spec.md

## Understanding

Deploy an automated preview environment that:
1. Merges all open PRs into a `preview` branch via GitHub Actions
2. Auto-deploys to Railway at `https://app.preview.trakrf.id`
3. Runs database migrations automatically on every deploy
4. Posts deployment status to PR comments

This replicates the proven pattern from `trakrf-handheld` but adapted for a full-stack Go + React monorepo with external TimescaleDB.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `../trakrf-handheld/.github/workflows/sync-preview.yml` (lines 1-182) - Proven preview workflow to copy
- `backend/Dockerfile` (lines 9-12) - migrate CLI installation pattern
- `backend/Dockerfile` (lines 28-30) - go.mod layer caching pattern
- `backend/Dockerfile` (line 42) - Copy migrate from builder stage pattern
- `backend/main.go` (line 27) - `//go:embed frontend/dist` expects `backend/frontend/dist` path

**Files to Create**:
- `.github/workflows/sync-preview.yml` - GitHub Actions workflow (copy from reference, adapt URL)
- `Dockerfile` (repository root) - 4-stage build: frontend → backend → production
- `scripts/docker-entrypoint.sh` - Migration runner + server starter
- `railway.json` (repository root, optional) - Explicit Railway configuration for clarity

**Files to Modify**:
- None (infrastructure-only changes)

## Architecture Impact

- **Subsystems affected**: CI/CD (GitHub Actions), Docker Build, Railway Deployment
- **New dependencies**: None (uses existing tools: pnpm, Go, migrate CLI, Railway)
- **Breaking changes**: None (adds preview environment, no changes to existing code)

## Task Breakdown

### Task 1: Create GitHub Actions Workflow
**File**: `.github/workflows/sync-preview.yml`
**Action**: CREATE
**Pattern**: Copy from `../trakrf-handheld/.github/workflows/sync-preview.yml` lines 1-182

**Implementation**:
1. Create `.github/workflows/` directory
2. Copy entire reference workflow
3. Update PR comment URL from Railway reference to `https://app.preview.trakrf.id` (line ~164)
4. Keep everything else unchanged (workflow is project-agnostic)

**Key sections**:
```yaml
# Triggers (lines 3-7): PR events + push to main
on:
  pull_request:
    types: [opened, synchronize, reopened, closed]
  push:
    branches: [main]

# Permissions (lines 16-18): Required for git push + PR comments
permissions:
  contents: write
  pull-requests: write

# PR merge logic (lines 80-114): Sequential merge with conflict detection
# Comment posting (lines 160-172): Success + conflict notifications
```

**Changes from reference**:
- Line ~164: Update URL to `https://app.preview.trakrf.id`
- All other lines: Copy as-is

**Validation**:
- File created at `.github/workflows/sync-preview.yml`
- Syntax check: `cat .github/workflows/sync-preview.yml | grep "app.preview.trakrf.id"`
- No code validation needed (infrastructure only)

---

### Task 2: Create Root Dockerfile
**File**: `Dockerfile` (repository root)
**Action**: CREATE
**Pattern**: 4-stage build following `backend/Dockerfile` patterns

**Implementation**:
```dockerfile
# Stage 1: Frontend Builder
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend

# Install pnpm
RUN npm install -g pnpm@latest

# Copy package files for layer caching (answer: go.mod pattern)
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Copy source and build
COPY frontend/ .
RUN pnpm run build
# Output: /app/frontend/dist

# Stage 2: Backend Builder
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app/backend

# Install migrate CLI (pattern from backend/Dockerfile:9-12)
RUN wget -qO- https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | \
    tar xvz && \
    mv migrate /usr/local/bin/migrate && \
    chmod +x /usr/local/bin/migrate

# Copy go.mod for layer caching (pattern from backend/Dockerfile:28-30)
COPY backend/go.mod ./
RUN go mod download

# Copy backend source
COPY backend/ .

# Copy frontend dist to expected location (answer: explicit COPY between stages)
# go:embed at backend/main.go:27 expects backend/frontend/dist
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist

# Build server
RUN go build -ldflags "-X main.version=0.1.0-preview" -o server .

# Stage 3: Production
FROM alpine:3.20 AS production
RUN apk --no-cache add ca-certificates
WORKDIR /app

# Copy migrate CLI (pattern from backend/Dockerfile:42)
COPY --from=backend-builder /usr/local/bin/migrate /usr/local/bin/migrate

# Copy server binary
COPY --from=backend-builder /app/backend/server /server

# Copy database migrations (critical: must match entrypoint path)
COPY database/migrations /app/database/migrations

# Copy entrypoint script (answer: scripts/ for deployment tooling)
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/server"]
```

**Critical paths to verify**:
- Frontend dist: `/app/frontend/dist` → `backend/frontend/dist` → embedded in server binary
- Migrations: `database/migrations/` → `/app/database/migrations` → read by entrypoint
- Entrypoint: `scripts/docker-entrypoint.sh` → `/usr/local/bin/docker-entrypoint.sh` → chmod +x

**Validation**:
- File created at `Dockerfile` (repository root)
- Build test (Task 5): `docker build -t trakrf/preview:test .`

---

### Task 3: Create Migration Entrypoint Script
**File**: `scripts/docker-entrypoint.sh`
**Action**: CREATE
**Pattern**: Standard entrypoint pattern (migrations → exec)

**Implementation**:
```bash
#!/bin/sh
set -e

echo "🗄️  Running database migrations..."

# Run migrations (path must match Dockerfile COPY)
migrate -path /app/database/migrations -database "$PG_URL" up

echo "✅ Migrations complete"
echo "🚀 Starting server..."

# exec replaces shell process with server (proper signal handling)
exec "$@"
```

**Key decisions**:
- Migration path: `/app/database/migrations` (matches Dockerfile COPY)
- Database URL: `$PG_URL` environment variable (Railway secret)
- Error handling: `set -e` fails fast on migration errors
- Signal handling: `exec "$@"` passes signals to server process
- Future-ready: Easy to add `MIGRATION_PG_URL` for separate DDL credentials (answer: future consideration)

**Validation**:
- File created at `scripts/docker-entrypoint.sh`
- Execute permission will be set in Dockerfile RUN chmod
- Functional test in Task 5 (local Docker run)

---

### Task 4: Create Railway Configuration
**File**: `railway.json` (repository root)
**Action**: CREATE
**Pattern**: Explicit configuration (optional but clarifies for GKE migration)

**Implementation**:
```json
{
  "$schema": "https://railway.app/railway.schema.json",
  "build": {
    "builder": "DOCKERFILE",
    "dockerfilePath": "Dockerfile"
  },
  "deploy": {
    "startCommand": "/server",
    "healthcheckPath": "/healthz",
    "healthcheckTimeout": 100,
    "restartPolicyType": "ON_FAILURE",
    "restartPolicyMaxRetries": 3
  }
}
```

**Why explicit config** (answer: clarity for future migration):
- Railway auto-detects Dockerfile, but explicit config documents intent
- Healthcheck configuration prevents unhealthy deploys
- When migrating to GKE, this serves as reference for k8s manifest
- Restart policy matches production expectations

**Validation**:
- File created at `railway.json`
- JSON syntax check: `cat railway.json | jq .`
- Railway will validate schema on deploy

---

### Task 5: Local Docker Build & Test
**Action**: VALIDATE
**Pattern**: Build and run with preview DB credentials (answer: full stack validation)

**Implementation**:
```bash
# 1. Build the Dockerfile
docker build -t trakrf/preview:test .

# Expected output:
# ✓ Frontend builder: pnpm install + build
# ✓ Backend builder: go mod download + build + migrate CLI
# ✓ Production: Alpine + copied artifacts
# Build time: ~3-5 minutes (first build), ~1-2 minutes (cached)

# 2. Run container with preview DB credentials
docker run --rm \
  -e PG_URL="postgres://tsdbadmin:<password>@hxumbw51zr.lezu4cbb98.tsdb.cloud.timescale.com:34826/tsdb?sslmode=require" \
  -e BACKEND_PORT=8080 \
  -e JWT_SECRET="preview-test-secret-change-in-railway" \
  -e BACKEND_CORS_ORIGIN=disabled \
  -p 8080:8080 \
  trakrf/preview:test

# Expected output:
# 🗄️  Running database migrations...
# migrate: Version 6 complete
# ✅ Migrations complete
# 🚀 Starting server...
# Server starting on port 8080

# 3. Test healthcheck (in another terminal)
curl http://localhost:8080/healthz
# Expected: 200 OK

curl http://localhost:8080/readyz
# Expected: 200 OK (confirms DB connection)

curl http://localhost:8080/health
# Expected: JSON with status + DB info
```

**What this validates**:
- ✓ Dockerfile builds successfully (all 4 stages)
- ✓ Frontend embedded in backend binary (go:embed works)
- ✓ Migrations run automatically (entrypoint works)
- ✓ Server starts and connects to preview DB
- ✓ Healthchecks respond correctly

**Get password from** (answer: local .env.local):
```bash
grep PG_URL_PREVIEW .env.local
```

**Validation**:
- Docker build completes without errors
- Container starts and shows migration + server logs
- All 3 healthcheck endpoints return successful responses
- Container runs for 30+ seconds without crashes

---

### Task 6: Railway Project Setup (Manual)
**Action**: MANUAL RAILWAY CONFIGURATION
**Pattern**: One-time setup via Railway dashboard

**Implementation steps**:

1. **Create new Railway project**:
   - Name: "TrakRF Platform Preview"
   - Region: us-west (or closest to your location)

2. **Connect GitHub repository**:
   - Repository: `trakrf/platform`
   - Deploy branch: `preview` (will be created by workflow)
   - Root directory: `/` (monorepo root)
   - Auto-deploy: Enabled

3. **Configure environment variables**:
   ```bash
   PG_URL=postgres://tsdbadmin:<password>@hxumbw51zr.lezu4cbb98.tsdb.cloud.timescale.com:34826/tsdb?sslmode=require
   BACKEND_PORT=8080
   BACKEND_LOG_LEVEL=debug
   JWT_SECRET=<generate-random-32-chars>  # openssl rand -base64 32
   BACKEND_CORS_ORIGIN=disabled
   PORT=8080
   ```

4. **Verify Railway settings**:
   - Build command: Automatic (uses Dockerfile)
   - Start command: `/server` (from railway.json)
   - Healthcheck: `/healthz` (from railway.json)
   - Deploy trigger: Push to `preview` branch

5. **Note Railway deployment URL** (for DNS setup):
   - Format: `{service-name}-production-{random}.up.railway.app`
   - Example: `trakrf-preview-production-a1b2.up.railway.app`
   - Save this CNAME value for Task 7

**Generate JWT_SECRET**:
```bash
openssl rand -base64 32
```

**Get PG_URL password**:
```bash
grep PG_URL_PREVIEW .env.local | cut -d= -f2
```

**Validation**:
- Railway project created and visible in dashboard
- GitHub repository connected
- Environment variables configured (6 vars)
- Auto-deploy enabled on `preview` branch
- Railway CNAME URL noted for DNS configuration

---

### Task 7: DNS Configuration (Manual Terraform)
**Action**: MANUAL TERRAFORM UPDATE
**Pattern**: Update trakrf/infra repository (answer: Terraform managed in separate repo)

**Implementation**:

1. **Get Railway CNAME** (from Task 6):
   - Railway deployment URL: `{service-name}-production-{random}.up.railway.app`

2. **Update Terraform configuration** (in `github.com/trakrf/infra`):
   ```hcl
   # Add to DNS configuration
   resource "cloudflare_record" "app_preview" {
     zone_id = var.trakrf_zone_id
     name    = "app.preview"
     type    = "CNAME"
     value   = "{railway-cname-from-task-6}"
     proxied = false  # Direct to Railway for SSL
     ttl     = 1      # Auto TTL
   }
   ```

3. **Apply Terraform**:
   ```bash
   cd ~/github.com/trakrf/infra
   terraform plan
   terraform apply
   ```

4. **Verify DNS propagation**:
   ```bash
   # Wait 1-2 minutes for DNS propagation
   dig app.preview.trakrf.id CNAME

   # Test HTTPS (Railway auto-provisions SSL)
   curl https://app.preview.trakrf.id/healthz
   # Expected: 200 OK
   ```

**Validation**:
- Terraform apply successful
- DNS resolves to Railway CNAME
- HTTPS works (Railway SSL certificate auto-provisioned)
- Healthcheck responds at `https://app.preview.trakrf.id/healthz`

---

### Task 8: End-to-End Workflow Test
**Action**: VALIDATE
**Pattern**: Create test PR to verify full workflow

**Implementation**:

1. **Create test branch and PR**:
   ```bash
   git checkout -b test/preview-workflow
   echo "# Preview Test" > PREVIEW_TEST.md
   git add PREVIEW_TEST.md
   git commit -m "test: verify preview deployment workflow"
   git push -u origin test/preview-workflow
   gh pr create --title "Test: Preview Workflow" --body "Testing preview deployment automation"
   ```

2. **Verify GitHub Actions workflow**:
   - Go to Actions tab: `https://github.com/trakrf/platform/actions`
   - Verify "Sync Preview Branch" workflow triggered
   - Check workflow logs:
     - ✓ Reset preview to main
     - ✓ Found test PR
     - ✓ Merged test PR into preview
     - ✓ Pushed preview branch

3. **Verify PR comment posted**:
   - Check test PR for comment from github-actions[bot]
   - Expected: "🚀 Preview Deployment Update"
   - Expected: "Railway preview environment will update shortly"
   - Expected URL: `https://app.preview.trakrf.id`

4. **Verify Railway deployment**:
   - Check Railway dashboard for new deployment
   - Verify build logs show:
     - ✓ Frontend build (pnpm)
     - ✓ Backend build (Go)
     - ✓ Migrations run (6 migrations)
     - ✓ Server started
     - ✓ Healthcheck passed

5. **Verify preview environment**:
   ```bash
   # Healthcheck
   curl https://app.preview.trakrf.id/healthz
   # Expected: 200 OK

   # API endpoint
   curl https://app.preview.trakrf.id/api/v1/accounts
   # Expected: 401 Unauthorized (auth required - correct behavior)

   # Frontend
   curl -I https://app.preview.trakrf.id/
   # Expected: 200 OK with text/html
   ```

6. **Test PR close workflow**:
   ```bash
   gh pr close --delete-branch
   ```
   - Verify workflow re-runs
   - Verify preview branch updates (test PR removed)

**Validation criteria** (from spec):
- ✓ Preview branch auto-updates within 2 minutes of PR update
- ✓ Railway deploys preview branch within 5 minutes
- ✓ PR comments posted within 1 minute of preview sync
- ✓ Healthcheck at `https://app.preview.trakrf.id/healthz` returns 200 OK
- ✓ Frontend accessible at `https://app.preview.trakrf.id`
- ✓ Backend API accessible at `https://app.preview.trakrf.id/api/v1/*`
- ✓ When PR closes, preview branch re-merges remaining PRs

---

## Risk Assessment

### Risk: Frontend dist path mismatch
**Probability**: Medium
**Impact**: High (build succeeds but server crashes on startup)
**Mitigation**:
- Explicit `COPY --from=frontend-builder /app/frontend/dist ./frontend/dist` in Dockerfile
- Local Docker test (Task 5) validates path before Railway deploy
- Server startup will fail immediately if path is wrong (fast feedback)

### Risk: Migration path incorrect
**Probability**: Medium
**Impact**: High (migrations don't run, schema out of sync)
**Mitigation**:
- Document paths clearly: `COPY database/migrations /app/database/migrations`
- Entrypoint uses same path: `migrate -path /app/database/migrations`
- Local Docker test (Task 5) runs migrations against preview DB
- Railway healthcheck fails if migrations fail (rollback)

### Risk: Railway build timeout
**Probability**: Low
**Impact**: Medium (deployment fails, manual retry needed)
**Mitigation**:
- Layer caching for pnpm (package.json) and Go (go.mod)
- First build ~5 minutes, cached builds ~2 minutes
- Railway timeout is 10 minutes (sufficient headroom)

### Risk: Workflow merge conflicts
**Probability**: Medium (expected behavior)
**Impact**: Low (detected and reported)
**Mitigation**:
- Workflow handles conflicts gracefully (abort merge, post comment)
- Developer gets immediate feedback via PR comment
- Preview continues with non-conflicting PRs

### Risk: Database migration failure on deploy
**Probability**: Low
**Impact**: High (deployment fails, preview unavailable)
**Mitigation**:
- Migrations tested locally first (Task 5)
- Railway healthcheck fails if server doesn't start
- Migration failures logged in Railway console
- Database rollback not automatic (preview DB can be reset manually if needed)

## Integration Points

**GitHub → Railway**:
- Workflow creates/updates `preview` branch
- Railway watches `preview` branch for changes
- Automatic deployment triggered on push

**Dockerfile → Server**:
- Frontend dist embedded via go:embed at `backend/frontend/dist`
- Server serves both API (`/api/v1/*`) and frontend (`/*`)
- Healthchecks at `/healthz`, `/readyz`, `/health`

**Entrypoint → Database**:
- Reads `PG_URL` environment variable
- Runs migrations from `/app/database/migrations`
- Exits on migration failure (Railway detects via healthcheck)

**Railway → DNS**:
- Railway provides CNAME: `{service}-production-{random}.up.railway.app`
- Terraform creates: `app.preview.trakrf.id` → Railway CNAME
- Railway auto-provisions SSL certificate

## VALIDATION GATES (MANDATORY)

**CRITICAL**: This feature is infrastructure-only (no backend/frontend code changes).

**After each infrastructure task**:
- File syntax validation (YAML, JSON, Bash, Dockerfile)
- Local build test (Task 5 - before Railway deploy)

**No code validation needed**:
- Backend validation: N/A (no Go code changes)
- Frontend validation: N/A (no React code changes)
- Lint/typecheck/test: N/A (infrastructure only)

**Final validation sequence** (Task 8):
1. GitHub Actions workflow syntax (automatic on push)
2. Dockerfile build success (local + Railway)
3. Database migrations run (entrypoint logs)
4. Server startup (healthcheck passes)
5. Preview environment accessible (curl tests)

**Enforcement**:
- If local Docker build fails (Task 5) → Fix Dockerfile before Railway deploy
- If Railway deploy fails → Check logs, fix issue, manual redeploy
- If healthcheck fails → Check entrypoint logs, verify DB connection
- If frontend 404 → Verify dist path in Dockerfile

**No iterative validation loops** - infrastructure changes are atomic (build or fail).

## Validation Sequence

### Per-Task Validation:
- **Task 1** (Workflow): YAML syntax check via `cat .github/workflows/sync-preview.yml | grep "app.preview.trakrf.id"`
- **Task 2** (Dockerfile): Created at repository root, syntax validated in Task 5
- **Task 3** (Entrypoint): Bash syntax check via `bash -n scripts/docker-entrypoint.sh`
- **Task 4** (Railway config): JSON syntax check via `cat railway.json | jq .`
- **Task 5** (Docker test): Full build + run validation with preview DB
- **Task 6** (Railway setup): Manual verification in Railway dashboard
- **Task 7** (DNS): DNS propagation + HTTPS test via curl
- **Task 8** (E2E test): Full workflow validation with test PR

### Final Validation (Task 8):
```bash
# All validation checks from spec (lines 261-273)
curl https://app.preview.trakrf.id/healthz      # ✓ 200 OK
curl https://app.preview.trakrf.id/readyz       # ✓ 200 OK (DB connected)
curl https://app.preview.trakrf.id/health       # ✓ JSON response
curl https://app.preview.trakrf.id/api/v1/accounts  # ✓ 401 (auth required)
curl -I https://app.preview.trakrf.id/          # ✓ 200 OK (frontend)
```

## Plan Quality Assessment

**Complexity Score**: 6/10 (MEDIUM - AT THRESHOLD)

**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from comprehensive spec
✅ Reference implementation exists at `../trakrf-handheld/.github/workflows/sync-preview.yml`
✅ All clarifying questions answered (6/6)
✅ Dockerfile patterns established in `backend/Dockerfile`
✅ Spec already broken into 5 phases (implementation-ready)
✅ Preview DB credentials available for testing
✅ No code changes (infrastructure only - reduces scope)
⚠️ First time creating root Dockerfile (new pattern, but well-documented)
⚠️ Railway-specific deployment (migrating to GKE soon - but minimal coupling)

**Assessment**: High-confidence implementation plan with proven reference patterns, comprehensive spec, and clear validation strategy. Infrastructure-only scope reduces risk. Local Docker testing validates critical paths before Railway deploy.

**Estimated one-pass success probability**: 85%

**Reasoning**: Reference workflow is proven (minimal adaptation needed). Dockerfile patterns are established (just combining frontend + backend builders). Main risks are path mismatches (mitigated by explicit COPY and local testing). Railway deployment is straightforward with healthcheck validation. Comprehensive spec and phased approach significantly increase success probability.
