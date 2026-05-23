# Contributing to TrakRF Platform

We love contributions! This guide will help you get started quickly.

## What is this project?

TrakRF Platform is an RFID/BLE asset tracking system for manufacturing and logistics. It provides real-time location tracking, historical analytics, and seamless integration with ERP/WMS systems. The platform consists of a Go backend, React frontend, TimescaleDB for time-series data, and an integrated MQTT broker for device communication.

## Before You Start

### Required Tools
- **Go 1.25+** - For backend development (required for Air hot-reload)
- **Node.js 18+** - For frontend development
- **Docker & Docker Compose** - For running dependencies
- **Git** - For version control
- **TimescaleDB** - Via Docker or TigerData cloud

### Quick Setup
```bash
# 1. Fork this repo on GitHub

# 2. Clone your fork
git clone https://github.com/YOUR_USERNAME/platform.git
cd platform

# 3. Start dependencies
docker-compose up -d timescaledb

# 4. Set up environment
cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env

# 5. Run migrations
cd backend && go run cmd/migrate/main.go up

# 6. Run tests
go test ./...
cd ../frontend && pnpm test
```

## Making Changes

### 1. Create a Branch
```bash
# Branch naming:
# - feature/add-xyz    (new features)
# - fix/broken-xyz     (bug fixes)
# - docs/update-xyz    (documentation)

git checkout -b feature/add-asset-history
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
```bash
# Backend tests
cd backend
go test ./...
go test -race ./...  # Race condition check

# Frontend tests
cd frontend
pnpm test
pnpm run lint

# Integration tests (requires running services)
docker-compose up -d
go test ./tests/integration -tags=integration
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
# Run full stack locally
docker-compose up -d
cd backend && go run cmd/server/main.go &
cd frontend && pnpm dev &

# Run API tests
cd tests/api && pnpm test
```

## Submitting Your Work

1. **Push to your fork:**
   ```bash
   git push origin feature/add-asset-history
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

## Common Tasks

### Adding a New API Endpoint
1. Define the route in `backend/api/routes.go`
2. Implement handler in appropriate controller
3. Add service layer logic
4. Write tests for handler and service
5. Update API documentation

### Adding a Frontend Feature
1. Create component in appropriate directory
2. Add API client code in `services/`
3. Update relevant Redux store/hooks
4. Add component tests
5. Update Storybook if applicable

### Database Changes
1. Create migration in `database/migrations/`
2. Test migration up and down
3. Update repository interfaces
4. Consider TimescaleDB features (continuous aggregates, compression)

## Cutting a Release

TrakRF uses `git describe` for the platform version (TRA-485). A release is
a single tag push; CI does the rest. See
[`docs/adr/0001-platform-vs-api-versioning.md`](docs/adr/0001-platform-vs-api-versioning.md)
for the three-axis versioning rationale (platform vs API contract vs spec).

### Steps

1. Update `CHANGELOG.md` — move items from `## [Unreleased]` to a new
   `## [vX.Y.Z] - YYYY-MM-DD` section.
2. Tag and push:
   ```bash
   git tag vX.Y.Z
   git push --tags
   ```
3. CI (`.github/workflows/docker-build.yml`) computes
   `git describe --tags --always --dirty`, bakes it into the backend binary
   (`-X main.version`) and the frontend bundle (`VITE_APP_VERSION`), and
   publishes `ghcr.io/trakrf/backend:sha-<short>` plus the standard tag set.
4. Deploy:
   - **Railway prod** — pinned to a semver tag. The tag push triggers
     Railway's auto-rebuild from source. No further action.
   - **GKE prod** (post TRA-351 cutover) — bump the image tag in
     `trakrf-infra/helm/trakrf-backend/values-gke.yaml` and merge; ArgoCD
     syncs. Two-repo dance is automatable via Option 1 (auto-PR from tag
     push) per TRA-485 — until then it is manual.
5. Verify post-deploy:
   ```bash
   curl https://app.trakrf.id/health | jq '.version, .commit, .tag'
   curl https://app.trakrf.id/version.json
   # Both should report vX.Y.Z; UI nav header should match.
   ```

### What gets versioned

| Axis | Source | Bumped when |
|---|---|---|
| Platform release | `git tag vX.Y.Z` → `git describe` | A new build is shipped |
| API contract | URL path `/api/v1/` | Breaking change to customer-facing API |
| OpenAPI spec | `info.version` in `docs/api/openapi.public.{json,yaml}` | Breaking change to spec shape (TRA-672) |

These three numbers move independently. Platform can ship many releases
inside one `/api/v1/`; spec can ship many revisions inside one platform
release. Do not couple them.

### Conventional Commits

Optional. The git-log readability convention (`feat:`, `fix:`, `chore:`,
`docs:`) is encouraged but no tool depends on it — bumps are manual.

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
