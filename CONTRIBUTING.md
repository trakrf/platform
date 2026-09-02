# Contributing to TrakRF Platform

We love contributions! This guide will help you get started quickly.

## What is this project?

TrakRF Platform is an RFID/BLE asset tracking system for manufacturing and logistics. It provides real-time location tracking, historical analytics, and seamless integration with ERP/WMS systems. The platform consists of a Go backend, React frontend, TimescaleDB for time-series data, and an integrated MQTT broker for device communication.

## Before You Start

### Required Tools
- **just** - The entry point for every workflow. Run recipes from the project root
- **Go 1.25+** - For backend development (required for Air hot-reload)
- **Node.js 18+ and pnpm 9+** - For frontend development. pnpm only; use `pnpm dlx`, never `npx`
- **Docker & Docker Compose** - Runs TimescaleDB and the backend
- **direnv** - Auto-loads `.env.local`
- **Git** - For version control

### Quick Setup
```bash
# 1. Fork this repo on GitHub

# 2. Clone your fork
git clone https://github.com/YOUR_USERNAME/platform.git
cd platform

# 3. Configure environment. Optional — every value has a working local default,
#    so the stack comes up with no env file at all. You need this only for
#    things with no sensible default, such as MQTT credentials.
cp .env.local.example .env.local
direnv allow

# 4. Generate the embedded build targets. Takes a couple of minutes cold and
#    needs no supervision, so it is worth backgrounding. Without it the build
#    fails with `pattern frontend/dist: no matching files found`.
just bootstrap

# 5. Start the stack: database, then migrations, then the backend
just dev

# 6. Run the tests
just test
```

There is **one** local env file, `.env.local`, and it holds connection *parts*
rather than URLs. `just bootstrap` symlinks `.env` to it, because docker compose
reads `.env` while direnv reads `.env.local`. See the README for why that matters
and for the local database roles — this guide does not restate it.

## Making Changes

### 1. Create a Branch
```bash
# Branch naming: <type>/<slug>
# - feat/add-xyz       (new features)
# - fix/broken-xyz     (bug fixes)
# - docs/update-xyz    (documentation)
# - chore/tidy-xyz     (everything else)
#
# Maintainers working a tracked issue use <type>/<ticket>-<slug>,
# e.g. feat/tra-1065-kitting-capability.

git checkout -b feat/add-asset-history
```

### 2. Write Your Code

**Project Philosophy:**
- **Clean Architecture** - Separate concerns between API, business logic, and data layers
- **Real-time First** - Design for live updates and streaming data
- **Multi-tenant** - Always consider data isolation
- **API-driven** - Frontend consumes only documented APIs

**Backend Example (Good):**
```go
// Clear service method with proper error handling
func (s *AssetService) GetLocation(ctx context.Context, assetID string) (*Location, error) {
    if err := s.validateAssetAccess(ctx, assetID); err != nil {
        return nil, fmt.Errorf("access denied: %w", err)
    }
    
    return s.repo.GetLatestLocation(ctx, assetID)
}
```

**Frontend Example (Good):**
```typescript
// Direct API call with proper typing
export async function fetchAssetLocation(assetId: string): Promise<Location> {
  const response = await api.get<Location>(`/assets/${assetId}/location`);
  return response.data;
}
```

### 3. Test Your Changes

Run recipes from the project root; `just` delegates into each workspace.

```bash
# Everything — lint, test and build across backend, frontend, cli and database
just validate

# Or a single workspace
just backend test
just frontend test
just lint

# Integration tests. They are in-package behind a build tag, not a separate
# directory, and they need a live database plus PG_ADMIN_URL — without it the
# whole package fails at setup rather than skipping.
just database up
just backend test-integration
```

### 4. Commit Your Work
```bash
# Use conventional commits
git commit -m "feat: add historical location queries"
git commit -m "fix: handle MQTT reconnection"
git commit -m "docs: update API examples"
```

## Testing Guide

### Backend Unit Tests
```go
// backend/services/asset_test.go
func TestAssetService_GetLocation(t *testing.T) {
    // Test with mock repository
    repo := &mocks.AssetRepository{}
    service := services.NewAssetService(repo)
    
    // Define expectations and test
}
```

### Frontend Tests
```typescript
// frontend/src/services/__tests__/asset.test.ts
describe('Asset Service', () => {
  it('fetches asset location', async () => {
    const location = await fetchAssetLocation('asset-123');
    expect(location).toHaveProperty('latitude');
  });
});
```

### API Integration Tests
```bash
# Run the full stack locally: database, migrations, then backend and frontend
just dev-local

# Run the API contract tests against it
just test-contract
```

## Submitting Your Work

1. **Push to your fork:**
   ```bash
   git push origin feat/add-asset-history
   ```

2. **Open a Pull Request:**
    - Go to https://github.com/trakrf/platform
    - Click "New Pull Request"
    - Select your branch
    - Describe what you changed and why

3. **PR Checklist:**
    - [ ] Tests pass (backend: `go test ./...`, frontend: `pnpm test`)
    - [ ] Code follows project conventions
    - [ ] Database migrations included if needed
    - [ ] API documentation updated
    - [ ] Commit messages use conventional format

4. **How changes land:**
    - Every change goes through a PR. Nothing is pushed directly to `main`.
    - PRs are merged with `gh pr merge --merge` — never squash, never rebase.

## Common Tasks

### Adding a New API Endpoint
1. Define the route in `backend/internal/cmd/serve/router.go`
2. Implement the handler under `backend/internal/handlers/`
3. Add service layer logic under `backend/internal/services/`
4. Write tests for handler and service
5. Update the OpenAPI annotations — the spec is generated from Go, never hand-edited

### Adding a Frontend Feature
1. Create the component in the appropriate directory
2. Add API client code in `services/`
3. Update the relevant store or hook — this app uses zustand and TanStack Query
4. Add component tests

### Database Changes
1. Create the migration in `backend/migrations/`
2. Run `just backend migrate-checksums` — **CI fails without it**, and applied migrations are immutable
3. Test the migration up and down
4. Update repository interfaces
5. Consider TimescaleDB features (continuous aggregates, compression)

## Cutting a Release

TrakRF declares the platform version in the root `VERSION` file (TRA-1126). A
release is a reviewed one-line diff; CI produces the git tag and the release
image tag as outputs of the merge build. See
[`docs/adr/0004-declared-platform-version.md`](docs/adr/0004-declared-platform-version.md)
for why the version is declared rather than derived, and
[`docs/adr/0001-platform-vs-api-versioning.md`](docs/adr/0001-platform-vs-api-versioning.md)
for the three-axis versioning rationale (platform vs API contract vs spec).

### Steps

**The full procedure is [`docs/releasing.md`](docs/releasing.md)** — the backup,
the ledger relocation ordering, promotion, the post-deploy checks and rollback.
Follow it rather than the summary here.

The shape of it:

1. **Open a release PR** that does two things and nothing else: flip `VERSION`
   from `X.Y.Z-dev` to `X.Y.Z`, and move the shipping items from
   `## [Unreleased]` into a new `## [X.Y.Z] - YYYY-MM-DD` section of
   `CHANGELOG.md`. `lint-test` fails the PR if the section is missing.
   ```bash
   printf 'X.Y.Z\n' > VERSION
   just check-changelog
   ```
2. **Merge it.** The merge build IS the release build. There is no tag to push
   and no ordering to get right: the version is a property of the commit, so
   two builds of it cannot disagree.
3. `.github/workflows/docker-build.yml` bakes the version into the backend
   binary (`-X main.version`) and the frontend bundle (`VITE_APP_VERSION`),
   publishes `ghcr.io/trakrf/backend:sha-<short>`, then — in the `release` job —
   creates the git tag `vX.Y.Z`, publishes `:vX.Y.Z`, and opens the follow-up PR
   returning `VERSION` to the next `-dev`.
4. Promote, once that build is green. `promote-prod` re-tags the manifest —
   there is no rebuild — and refuses any image that is not the release commit:
   ```bash
   gh workflow run promote-prod.yml -f source=vX.Y.Z
   ```
   ArgoCD Image Updater then picks up the new digest; expect up to ~2 minutes.
5. Verify post-deploy:
   ```bash
   curl https://app.trakrf.id/health | jq '.version, .commit, .tag'
   curl https://app.trakrf.id/version.json
   # Both should report vX.Y.Z; UI nav header should match.
   ```

### What gets versioned

| Axis | Source | Bumped when |
|---|---|---|
| Platform release | Root `VERSION` file → CI mints `vX.Y.Z` | A new build is shipped |
| API contract | URL path `/api/v1/` | Breaking change to customer-facing API |
| OpenAPI spec | `info.version` in `docs/api/openapi.public.{json,yaml}` | Breaking change to spec shape (TRA-672) |

These three numbers move independently. Platform can ship many releases
inside one `/api/v1/`; spec can ship many revisions inside one platform
release. Do not couple them.

### Conventional Commits

Required. Use `feat:`, `fix:`, `docs:` or `chore:`. No tool enforces it and
version bumps are manual, but the convention is not optional.

## Getting Help

- **Questions?** Open a GitHub Discussion
- **Found a bug?** Open an issue with steps to reproduce
- **Have an idea?** Open a discussion before coding major features

## Code of Conduct

Be professional, respectful, and constructive. We're building critical infrastructure for businesses - act accordingly.

## Legal

By submitting a pull request, you agree that:

1. You have the right to submit the contribution
2. You grant DevOps To AI LLC dba TrakRF a perpetual, worldwide, non-exclusive,
   no-charge, royalty-free, irrevocable license to use your
   contribution under any terms, including commercial licensing
3. Your contribution will be licensed under BSL 1.1 for public use
4. TrakRF may relicense your contribution under different
   terms for commercial customers

This ensures we can maintain the dual licensing model (BSL for public,
commercial licenses for enterprise) while properly attributing your
contribution.
