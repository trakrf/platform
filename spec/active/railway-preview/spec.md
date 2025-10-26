# Feature: Railway Preview Environment (TRA-81)

## Origin
This specification emerged from the need to deploy a preview environment that merges all open PRs automatically and deploys to Railway at `app.preview.trakrf.id`. The pattern is based on the proven implementation in `../trakrf-handheld/.github/workflows/sync-preview.yml`.

## Domain Architecture (Full Picture)

### Current State
```
trakrf.id              → Next.js marketing (trakrf-web, ShipFast template)
hh-preview.trakrf.id   → Handheld preview (trakrf-handheld, to be replaced)
(no app.trakrf.id)     → Platform app not deployed yet
```

### After TRA-81 (Immediate)
```
trakrf.id              → Next.js marketing (trakrf-web, unchanged)
hh-preview.trakrf.id   → Handheld preview (trakrf-handheld, still active)
app.preview.trakrf.id  → Platform preview (this spec) ✅ NEW
```

### Production Target (Future)
```
trakrf.id              → Astro marketing (new repo, Cloudflare Pages)
preview.trakrf.id      → Astro marketing preview (Cloudflare Pages)
app.trakrf.id          → Platform production (Railway)
app.preview.trakrf.id  → Platform preview (Railway)
```

### Repository Evolution
```
Phase 1 (TRA-81):     platform repo → app.preview.trakrf.id ✅
Phase 2 (Migration):  hh-preview.trakrf.id → SUNSET (trakrf-handheld deprecated)
Phase 3 (Marketing):  trakrf.id migrated (trakrf-web deprecated, new Astro repo)
```

### Scope Boundaries
**TRA-81 Includes:**
- ✅ Platform app preview at `app.preview.trakrf.id`
- ✅ Railway deployment (backend with embedded frontend + TimescaleDB Cloud)
- ✅ GitHub Actions PR merge workflow
- ✅ Replaces `hh-preview.trakrf.id` (trakrf-handheld preview)

**Out of Scope (Separate Work):**
- ❌ Marketing site migration (Next.js → Astro)
- ❌ Marketing preview deployment (`preview.trakrf.id`)
- ❌ Cloudflare Pages setup
- ❌ Production platform deployment (`app.trakrf.id`)
- ❌ Sunsetting `hh-preview.trakrf.id` (happens after Phase 2)

## Outcome
Developers can preview all open PR changes together in a single environment before merging to main, reducing integration issues and providing immediate feedback on deployment status.

## User Story
**As a developer**
I want my PR changes automatically merged with other open PRs and deployed to a preview environment
So that I can test integration with other in-flight changes and catch conflicts early

## Context

### Discovery
- The `trakrf-handheld` project has a working preview deployment system
- GitHub Actions workflow merges all open PRs sequentially into a `preview` branch
- Railway auto-deploys when the `preview` branch updates
- PR comments provide deployment status and conflict notifications
- Pattern is proven and ready to replicate

### Current State
- No preview environment exists for the platform monorepo
- PRs are merged to main without testing integration with other PRs
- No automated deployment to Railway
- Manual testing required before merge

### Desired State
- Automated `preview` branch that merges all open (non-draft) PRs
- Railway deployment to `app.preview.trakrf.id` on every preview branch update
- PR comments notify developers of deployment success or merge conflicts
- Full stack deployment: Go backend + React frontend + TimescaleDB
- Zero manual intervention for preview updates
- Replaces current `trakrf-handheld` preview environment

## Technical Requirements

### GitHub Actions Workflow
- **Source**: Copy `../trakrf-handheld/.github/workflows/sync-preview.yml` as-is
- **Triggers**: PR events (opened, synchronize, reopened, closed) and push to main
- **Workflow Steps**:
  1. Reset `preview` branch to `main`
  2. Fetch all open, non-draft PRs (sorted by creation date)
  3. Merge PRs sequentially into `preview`
  4. On conflict: abort merge, track conflict, post PR comment
  5. Push `preview` branch (force-with-lease)
  6. Post success comments on merged PRs with deployment URL
- **Concurrency**: Serialize preview syncs to avoid race conditions
- **Permissions**: `contents: write`, `pull-requests: write`

### Railway Configuration

#### railway.json (Optional - Railway auto-detects root Dockerfile)
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

**Note**: Dockerfile located at repository root (not `backend/Dockerfile`) to support future omnibus self-hosting container.

- **Railway Services**: 1 service (Go backend with embedded frontend)
- **Database**: TimescaleDB Cloud (external managed service, NOT on Railway)
- **Deployment Strategy**:
  - Backend serves both API (`/api/v1/*`) AND frontend (`/*`)
  - Frontend built during Docker build, embedded via `go:embed`
  - Backend `frontend.go` handles static file serving with cache headers
- **Deploy Branch**: `preview` (Railway watches this branch)
- **Domain**: `app.preview.trakrf.id`
- **Auto-Deploy**: Enabled on preview branch push
- **Build Detection**: Railway can auto-detect via Nixpacks (monorepo support)

### Database Migrations
- **Requirement**: Auto-run migrations on every Railway deploy
- **Implementation**: Docker entrypoint script runs migrations before starting server
- **Script**: `backend/docker-entrypoint.sh`
  ```bash
  #!/bin/sh
  set -e
  echo "Running database migrations..."
  migrate -path /app/database/migrations -database "$PG_URL" up
  echo "Starting server..."
  exec "$@"
  ```
- **Dockerfile Changes**: Copy migrations and entrypoint script into production image

### Environment Variables (Railway)
**Backend Service** (single service - serves API + frontend):
- `PG_URL=<TimescaleDB Cloud connection string>` (Railway secret, see credentials below)
- `BACKEND_PORT=8080` (read by app at main.go:95)
- `BACKEND_LOG_LEVEL=debug` (verbose for preview, optional)
- `JWT_SECRET=<generate-random-32-char-string>` (Railway secret, read at jwt.go:67)
- `BACKEND_CORS_ORIGIN=disabled` (same-origin deployment, read at middleware.go:59)
- `PORT=8080` (Railway standard, optional - defaults to BACKEND_PORT)

**Database:**
- External TimescaleDB Cloud instance (preview environment)
- Connection string stored as Railway secret
- Backend connects via `PG_URL`

**Frontend Build** (embedded in backend binary):
- Frontend built during Docker image build
- Vite build output embedded via `go:embed frontend/dist`
- No separate frontend service or env vars needed

### TimescaleDB Preview Credentials
**Service**: `trakrf-preview` (Timescale Cloud - Free Tier)

**Connection Details**:
- **Host**: `hxumbw51zr.lezu4cbb98.tsdb.cloud.timescale.com`
- **Port**: `34826`
- **Database**: `tsdb`
- **Username**: `tsdbadmin`
- **Password**: `<see .env.local>`
- **Connection String**: `postgres://tsdbadmin:<password>@hxumbw51zr.lezu4cbb98.tsdb.cloud.timescale.com:34826/tsdb?sslmode=require`

**Usage**:
- Add to Railway as `PG_URL` environment variable (full connection string with password)
- Add to local `.env.local` as `PG_URL_PREVIEW` for testing (credentials in .env.local.example)
- SSL required (`sslmode=require`)

**Security Note**: Actual password stored in `.env.local` only (gitignored), never committed to spec or git.

### Health Checks
- **Liveness**: `GET /healthz` → 200 OK (process alive)
- **Readiness**: `GET /readyz` → 200 OK (DB connected)
- **Railway Config**: `healthcheckPath: "/healthz"`, `healthcheckTimeout: 100`

## Code Examples

### Workflow Structure (Unchanged from Handheld)
```yaml
on:
  pull_request:
    types: [opened, synchronize, reopened, closed]
  push:
    branches: [main]

jobs:
  sync-preview:
    runs-on: ubuntu-latest
    steps:
      - name: Reset preview to main
        run: git checkout -B preview origin/main

      - name: Get all open PRs
        # Uses GitHub API to fetch open, non-draft PRs

      - name: Merge PRs into preview
        # Sequentially merge, handle conflicts

      - name: Push preview branch
        run: git push origin preview --force-with-lease

      - name: Post deployment status comment
        # Notify PRs of deployment or conflicts
```

### Docker Entrypoint (New)
```dockerfile
# backend/Dockerfile
COPY --from=development /usr/local/bin/migrate /usr/local/bin/migrate
COPY database/migrations /app/database/migrations
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./server"]
```

## Testing Strategy

### Pre-Deploy Testing
```bash
# Test PR merge locally
git checkout -B preview origin/main
git fetch origin pr-branch:pr-branch
git merge pr-branch --no-edit

# Test production Docker build
cd backend
docker build --target production -t trakrf/backend:preview .
docker run -e PG_URL=postgresql://... trakrf/backend:preview

# Test migration entrypoint
just db-up
./backend/docker-entrypoint.sh ./backend/server
```

### Post-Deploy Validation
```bash
# Health checks
curl https://app.preview.trakrf.id/healthz   # 200 OK
curl https://app.preview.trakrf.id/readyz    # 200 OK (DB connected)
curl https://app.preview.trakrf.id/health    # JSON response

# API smoke test
curl https://app.preview.trakrf.id/api/v1/accounts  # 401 or valid response

# Frontend accessibility
open https://app.preview.trakrf.id  # Verify JS/CSS loads
```

## Validation Criteria
- [ ] Preview branch auto-updates within 2 minutes of PR update
- [ ] Railway deploys preview branch within 5 minutes
- [ ] PR comments posted within 1 minute of preview sync
- [ ] Healthcheck at `https://app.preview.trakrf.id/healthz` returns 200 OK
- [ ] Merge conflicts detected and reported via PR comments
- [ ] Database migrations run successfully on every deploy
- [ ] Frontend accessible at `https://app.preview.trakrf.id`
- [ ] Backend API accessible at `https://app.preview.trakrf.id/api/v1/*`
- [ ] Multiple PRs can be merged and tested together
- [ ] When PR closes, preview branch re-merges remaining PRs
- [ ] Replaces trakrf-handheld preview environment

## Conversation References

### Key Insights
- **Pattern Reuse**: "let's copy our preview setup from ../trakrf-handheld and use preview.trakrf.id"
- **Merge Strategy**: "the idea is that we merge branches from all open PRs and deploy to preview.trakrf.id with a github action"
- **Source Reference**: "see ../trakrf-handheld/.github/workflows/sync-preview.yml"

### Decisions Made
- **Single Merged Preview**: Use one preview environment for all PRs (not per-PR environments)
- **Workflow Reuse**: Copy handheld workflow as-is (project-agnostic design)
- **Frontend Strategy**: Prefer serving from backend for simplicity (can migrate to separate service later)
- **Migration Automation**: Use entrypoint script to ensure migrations always run

### Implementation Concerns
- **Monorepo Complexity**: Platform has backend + frontend + external database (handheld was frontend-only)
- **Migration Safety**: Need auto-run migrations without manual intervention
- **External Database**: TimescaleDB Cloud connection, network access, credential management
- **Domain Management**: DNS configuration for `app.preview.trakrf.id`
- **Cost**: Railway Hobby tier = ~$10/mo (2 services) + TimescaleDB Cloud (separate billing)

## Edge Cases & Constraints

### Edge Cases
1. **All PRs Conflict**: Preview branch = main (no PRs merged)
2. **Concurrent Updates**: Concurrency group serializes syncs
3. **Force Push Race**: Fallback to force push if force-with-lease fails
4. **Migration Failure**: Deploy fails, healthcheck fails, Railway retries
5. **Empty Preview**: If no open PRs, preview = main

### Constraints
- **Railway Plan**: Hobby tier ($5/service) for preview environment
- **Database Persistence**: Keep preview data (don't reset on each deploy)
- **Branch Protection**: Only GitHub Actions can push to `preview` branch
- **Build Time**: Frontend build adds ~2-3 minutes to deployment
- **No Secrets in Git**: All config via Railway dashboard/secrets

## Implementation Phases

### Phase 1: Workflow Setup (No Railway)
- Create `.github/workflows/sync-preview.yml`
- Test PR merging and conflict detection
- Verify PR comments work

**Success**: Preview branch auto-updates, comments posted

### Phase 2: Railway Project Setup
- Create TimescaleDB Cloud preview instance (external)
- Create Railway project "TrakRF Platform Preview"
- Add Backend Railway service (single service - embeds frontend)
- Configure environment variables (PG_URL, JWT_SECRET, etc.)
- Test manual deploy

**Success**: Backend deploys, serves frontend + API, connects to TimescaleDB Cloud, healthcheck passes

### Phase 3: Auto-Migrations
- Create `backend/docker-entrypoint.sh`
- Update `backend/Dockerfile`
- Test locally, deploy to Railway

**Success**: Migrations run automatically on deploy

### Phase 4: Domain Configuration
- Add DNS: `app.preview.trakrf.id` → Railway backend service
- Configure SSL (auto via Railway)
- Test HTTPS access

**Success**: `https://app.preview.trakrf.id/healthz` returns 200

### Phase 5: End-to-End Testing
- Verify frontend loads at `https://app.preview.trakrf.id`
- Verify API accessible at `https://app.preview.trakrf.id/api/v1/*`
- Update PR comment template with preview URL
- Test full PR workflow (open → merge → deploy → comment)

**Success**: Full stack accessible at `app.preview.trakrf.id`, PRs notify with preview URL, frontend + API both working

## Decisions Made

1. **Domain Architecture**: ✅
   - `app.preview.trakrf.id` → Platform preview (backend + frontend + DB)
   - `preview.trakrf.id` → Marketing site preview (Cloudflare Pages, out of scope)
   - Replaces `trakrf-handheld` preview environment

2. **Frontend Deployment**: ✅ Embedded in backend
   - Backend serves both API and static frontend
   - Frontend built during Docker build, embedded via `go:embed`
   - Already implemented in `backend/frontend.go`
   - **Future optimization**: Move to CDN for edge performance (not load concerns)

3. **Railway Plan**: ✅ Hobby tier
   - ~$5/mo for 1 service (Backend with embedded frontend)
   - Database: TimescaleDB Cloud (separate billing)

4. **Branch Protection**: ✅ Yes
   - Only GitHub Actions can push to `preview` branch

5. **Marketing Migration**: ❌ Out of scope for TRA-81
   - Separate effort: Next.js → Astro migration
   - Source: `../trakrf-web` (ShipFast template) or scrape `https://trakrf.id`
   - Deploy to Cloudflare Pages (not Railway)

## Deprecation Timeline

1. **trakrf-handheld** (`hh-preview.trakrf.id`): ✅ Decided
   - Deprecate AFTER handheld functionality works in preview AND production
   - Not urgent, thorough testing first
   - Current: `https://hh-preview.trakrf.id`
   - Replaced by: `app.preview.trakrf.id` (platform repo) + `app.trakrf.id` (production)

2. **trakrf-web** (`trakrf.id`): ✅ Decided
   - Deprecate AFTER Astro migration completes
   - Current: `https://trakrf.id` (ShipFast Next.js template)
   - Replaced by: New marketing repo (Astro on Cloudflare Pages, separate from platform)

3. **Marketing Repo Strategy**: ✅ Decided
   - **Separate repo** (not in `platform/` monorepo)
   - **Reasoning**: Different deployment strategy (Cloudflare Pages) + different dev lifecycle
   - **Deploy**: Cloudflare Pages (not Railway)
   - **Source**: Migrate from `trakrf-web` or scrape live site

## Implementation Decisions (January 2025)

### Railway Account Strategy ✅
- **Single Railway account** - All projects on same dashboard
- **New project**: "TrakRF Platform Preview" (create new, don't reuse existing)
- **Existing project**: `trakrf-web` will be refactored to production `app.trakrf.id` later
- **No preview deployment exists yet** - This is the first preview environment

### Dockerfile Location ✅
- **Location**: Repository root (`/Dockerfile`)
- **Rationale**: Future omnibus container for self-hosting
- **Structure**: 4-stage build (tools → frontend → backend → production)
- **Embedded frontend path**: `backend/frontend/dist` (relative to backend/main.go go:embed)
- **Critical**: Must `COPY --from=frontend-builder` into backend build stage

### DNS Management ✅
- **Managed in**: `github.com/trakrf/infra` (Terraform)
- **Status**: Terraform update ready, waiting for Railway CNAME
- **Timeline**: Apply after Railway project created (Phase 4)

### TimescaleDB Credentials ✅
- **Service**: `trakrf-preview` (Free Tier - Timescale Cloud)
- **Provisioned**: Yes, credentials available (see above)
- **Local testing**: Add as `PG_URL_PREVIEW` in `.env.local`

### Environment Variable Verification ✅
All required env vars mapped to backend code:
- `PG_URL` → `storage.go:22` (required, crashes if missing)
- `BACKEND_PORT` → `main.go:95` (optional, defaults to "8080")
- `JWT_SECRET` → `jwt.go:67` (required for auth endpoints)
- `BACKEND_CORS_ORIGIN` → `middleware.go:59` (optional, defaults to "*", use "disabled" for production)
- `BACKEND_LOG_LEVEL` → Not currently used (always JSON info level)

### Dependencies Verified ✅
- ✅ `pnpm-lock.yaml` exists (271KB)
- ✅ 4 database migrations in `database/migrations/`
- ✅ Reference workflow at `../trakrf-handheld/.github/workflows/sync-preview.yml`
- ✅ Backend has `go:embed frontend/dist` at `main.go:27`
- ✅ Frontend handler implemented at `backend/internal/handlers/frontend/`

## Open Questions

1. **Marketing Timeline**: Tackle Next.js → Astro migration immediately after TRA-81 or later?
   - **Status**: Out of scope for TRA-81, defer decision

## Success Metrics
- Zero manual interventions required for preview updates
- Developers receive deployment feedback within 3 minutes of PR update
- Preview environment uptime > 99%
- Merge conflicts caught before main branch merge

## Future Enhancements (Post-MVP)
1. **CDN for Frontend Assets** - Move static files to Cloudflare/CloudFront for global edge performance (only after backend static serving becomes a bottleneck, unlikely for years)
2. **Per-PR Preview Environments** - Deploy each PR to `pr-123.app.preview.trakrf.id`
3. **Automated E2E Tests** - Run Playwright tests on preview deployments
4. **Preview Data Seeding** - Auto-seed test data on deploy
5. **Deployment Status Badges** - GitHub commit status checks
6. **Cost Optimization** - Auto-sleep preview when inactive

## Related Documents
- Source workflow: `../trakrf-handheld/.github/workflows/sync-preview.yml`
- Railway Docs: https://docs.railway.app/
- Railway Monorepos: https://docs.railway.app/develop/monorepo
