# TRA-363: Docker build + GHCR publish (platform repo)

**Linear:** [TRA-363](https://linear.app/trakrf/issue/TRA-363)
**Parent:** TRA-351 · **Siblings:** TRA-358 (infra CI) · **Unblocks:** TRA-361 (migration Job)
**Date:** 2026-04-13

## Goal

Publish `ghcr.io/trakrf/backend` and `ghcr.io/trakrf/ingester` from this repo via GitHub Actions so that TRA-361 can consume them from the migration Job, and so future work has a single source of truth for image references.

## Scope

Two new GitHub Actions workflows in `.github/workflows/`:

1. **`docker-build.yml`** — builds both images; pushes to GHCR on merge to `main`
2. **`lint.yml`** — bundled per user request; runs `just lint` + `just test` on PR/main

## Non-Goals

- Multi-arch (EKS is currently `t3.xlarge` / amd64; revisit if we move to Graviton)
- Signing / SBOM / provenance
- Chart image-tag auto-bump in `trakrf/infra`
- Deploy workflow (ArgoCD polls infra, not this repo)

## Decisions

| Decision | Value | Rationale |
| --- | --- | --- |
| Platform | `linux/amd64` only | EKS nodes are amd64 |
| GHCR visibility | Public | Repo is public (BSL); avoids `imagePullSecret` |
| Main-push tags | `:latest` + `:sha-<short>` | Skipping `:main` — redundant with `:latest` when main is the only pushing branch |
| Auth | `GITHUB_TOKEN` + `packages: write` | No PAT |
| Parallelism | Matrix over `{backend, ingester}` | Independent failures, independent caches |
| Path filters | None | Build time cheap with cache; avoids branch-protection complications |
| Cache | `type=gha` scoped per image | `scope=backend` / `scope=ingester` prevents cross-contamination |
| Concurrency | `cancel-in-progress` on PRs only | Never cancel main-branch builds — every merged sha should tag |

## docker-build.yml

**Triggers:** `pull_request` (build-only) and `push` to `main` (build + push).

**Job permissions:** `contents: read`, `packages: write`.

**Matrix:**

```yaml
strategy:
  fail-fast: false
  matrix:
    include:
      - image: backend
        context: ./backend
        dockerfile: ./backend/Dockerfile
        target: production
      - image: ingester
        context: ./ingester
        dockerfile: ./ingester/Dockerfile
        # no target — single-stage Dockerfile
```

**Steps per matrix entry:**

1. `actions/checkout@v4`
2. `docker/setup-buildx-action@v3`
3. `docker/login-action@v3` to `ghcr.io` — **guarded by** `if: github.event_name != 'pull_request'`; uses `github.actor` + `secrets.GITHUB_TOKEN`
4. `docker/metadata-action@v5` with `images: ghcr.io/trakrf/${{ matrix.image }}` and tags:
   - `type=raw,value=latest,enable={{is_default_branch}}`
   - `type=sha,prefix=sha-,format=short`
5. `docker/build-push-action@v6`:
   - `context`, `file`, `target` from matrix (omit `target` for ingester)
   - `platforms: linux/amd64`
   - `push: ${{ github.event_name != 'pull_request' }}`
   - `tags: ${{ steps.meta.outputs.tags }}`
   - `labels: ${{ steps.meta.outputs.labels }}`
   - `cache-from: type=gha,scope=${{ matrix.image }}`
   - `cache-to: type=gha,mode=max,scope=${{ matrix.image }}`

**Concurrency:**

```yaml
concurrency:
  group: docker-build-${{ github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}
```

## lint.yml

**Triggers:** `pull_request` and `push` to `main`.

**Job permissions:** `contents: read`.

**Steps:**

1. `actions/checkout@v4`
2. `actions/setup-go@v5` with `go-version: '1.25'`, `cache: true`, `cache-dependency-path: backend/go.sum`
3. `pnpm/action-setup@v4` (resolve version from `frontend/package.json` `packageManager` field; pin inline if absent)
4. `actions/setup-node@v4` with `cache: 'pnpm'`, `cache-dependency-path: frontend/pnpm-lock.yaml`
5. `extractions/setup-just@v2`
6. Install frontend deps (`pnpm install --frozen-lockfile` in `frontend/`, or `just frontend install` if that recipe exists)
7. `just lint`
8. `just test`

**Concurrency:**

```yaml
concurrency:
  group: lint-${{ github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}
```

**Open question to resolve during implementation:** does `just test` in a clean CI environment require live service deps (Timescale, etc.)? If yes, either add a services block (postgres/timescale image) or narrow to unit-only tests and create a follow-up for integration test CI. Decide at implementation time after running `just test` locally from a clean state.

## Verification

**On the PR (before merge):**

- Both `docker-build` jobs green with `push: false`
- `lint` job green
- Buildx cache writes confirmed in first-run logs (cold cache expected)

**After merge to main:**

- `docker-build` main run succeeds
- Packages appear under <https://github.com/orgs/trakrf/packages>
- Flip each package to **public** visibility in package settings (one-time, per package — first publish may inherit private)
- Unauthenticated pull works from a workstation:
  - `docker pull ghcr.io/trakrf/backend:latest`
  - `docker pull ghcr.io/trakrf/ingester:latest`
  - `docker pull ghcr.io/trakrf/backend:sha-<short>`
- Report image refs back to TRA-361 owner

## Rollback

Workflows are additive. To disable: turn off the workflow in Actions UI or revert the PR. No effect on existing ArgoCD deploys (they poll `trakrf/infra`).

## Follow-ups (explicitly not this ticket)

- Chart image-tag auto-bump in `trakrf/infra` after publish
- Multi-arch when/if EKS moves to Graviton
- Signing / SBOM / provenance attestations
- Integration-test CI (if `just test` requires services)
