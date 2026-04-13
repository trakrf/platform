# TRA-363: Docker build + GHCR publish — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two GitHub Actions workflows: one that builds backend + ingester Docker images on every PR and pushes them to `ghcr.io/trakrf/{backend,ingester}` on merge to `main`, and a bundled lint workflow that runs `just lint` + `just test`.

**Architecture:** Two independent workflow files. `docker-build.yml` uses a matrix to build both images in parallel, `docker/build-push-action@v6` with GHA buildx cache scoped per image, and conditionally pushes only on `main`. `lint.yml` runs `just lint` + `just test` on a single runner with Go + pnpm caching. `GITHUB_TOKEN` with `packages: write` permission handles GHCR auth — no PAT.

**Tech Stack:** GitHub Actions, `docker/setup-buildx-action@v3`, `docker/login-action@v3`, `docker/metadata-action@v5`, `docker/build-push-action@v6`, `actions/setup-go@v5`, `pnpm/action-setup@v4`, `actions/setup-node@v4`, `extractions/setup-just@v2`.

**Spec:** [`docs/superpowers/specs/2026-04-13-tra-363-docker-build-ghcr-design.md`](../specs/2026-04-13-tra-363-docker-build-ghcr-design.md)

---

## File Structure

Both files are new; nothing in the repo is modified.

- **Create:** `.github/workflows/docker-build.yml` — PR = build only, main = build + push to GHCR
- **Create:** `.github/workflows/lint.yml` — `just lint` + `just test` on both PR and main

Each file has one responsibility. They share no state and can fail independently.

---

## Task 1: Create lint.yml

**Files:**
- Create: `.github/workflows/lint.yml`

This task goes first because it's self-contained and gives us a pattern to confirm on the PR before we add the more complex docker-build workflow.

- [ ] **Step 1: Create `.github/workflows/lint.yml` with the exact contents below**

```yaml
name: Lint and Test

on:
  pull_request:
  push:
    branches: [main]

concurrency:
  group: lint-${{ github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}

permissions:
  contents: read

jobs:
  lint-test:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
          cache: true
          cache-dependency-path: backend/go.sum

      - name: Set up pnpm
        uses: pnpm/action-setup@v4
        with:
          version: 9

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'pnpm'
          cache-dependency-path: frontend/pnpm-lock.yaml

      - name: Install just
        uses: extractions/setup-just@v2

      - name: Install frontend deps
        working-directory: frontend
        run: pnpm install --frozen-lockfile

      - name: Lint
        run: just lint

      - name: Test
        run: just test
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/lint.yml
git commit -m "ci(tra-363): add lint + test workflow"
```

---

## Task 2: Create docker-build.yml

**Files:**
- Create: `.github/workflows/docker-build.yml`

- [ ] **Step 1: Create `.github/workflows/docker-build.yml` with the exact contents below**

```yaml
name: Docker Build and Push

on:
  pull_request:
  push:
    branches: [main]

concurrency:
  group: docker-build-${{ github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}

permissions:
  contents: read
  packages: write

jobs:
  build:
    runs-on: ubuntu-latest
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
            target: ''
    name: build-${{ matrix.image }}
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        if: github.event_name != 'pull_request'
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/trakrf/${{ matrix.image }}
          tags: |
            type=raw,value=latest,enable={{is_default_branch}}
            type=sha,prefix=sha-,format=short

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: ${{ matrix.context }}
          file: ${{ matrix.dockerfile }}
          target: ${{ matrix.target }}
          platforms: linux/amd64
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha,scope=${{ matrix.image }}
          cache-to: type=gha,mode=max,scope=${{ matrix.image }}
```

Notes for the engineer:

- `target: ''` for the ingester is intentional — the `build-push-action` treats an empty string as "no target" and builds the default (only) stage of the single-stage `ingester/Dockerfile`. Do NOT add a conditional or a second matrix branch; the empty string is the clean way.
- `docker/metadata-action@v5`'s `enable={{is_default_branch}}` means `:latest` is only applied on pushes to `main`; on PRs the tag list will only include `sha-<short>`, which is correct (push is disabled anyway, but the tag list still gets computed).

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/docker-build.yml
git commit -m "ci(tra-363): add docker build + GHCR push workflow"
```

---

## Task 3: Push the branch and open the PR

**Files:** none (git + `gh` CLI only)

- [ ] **Step 1: Push the branch**

```bash
git push -u origin feature/tra-363-docker-build-ghcr
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "ci(tra-363): docker build + GHCR publish, plus lint/test workflow" --body "$(cat <<'EOF'
## Summary
- Add `.github/workflows/docker-build.yml` — builds `ghcr.io/trakrf/backend` and `ghcr.io/trakrf/ingester` in parallel. PRs build-only; merges to `main` push `:latest` + `:sha-<short>`.
- Add `.github/workflows/lint.yml` — runs `just lint` + `just test` on PR and main.

Unblocks TRA-361 (migration Job will consume `ghcr.io/trakrf/backend`).

Spec: `docs/superpowers/specs/2026-04-13-tra-363-docker-build-ghcr-design.md`

## Test plan
- [ ] `Lint and Test` workflow green on this PR
- [ ] `build-backend` job green with `push: false`
- [ ] `build-ingester` job green with `push: false`
- [ ] After merge: packages appear at https://github.com/orgs/trakrf/packages
- [ ] After merge: `docker pull ghcr.io/trakrf/backend:latest` succeeds unauthenticated (once flipped to public)
- [ ] After merge: `docker pull ghcr.io/trakrf/ingester:latest` succeeds unauthenticated

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Task 4: Verify the PR checks

**Files:** none (observation task)

- [ ] **Step 1: Watch the checks**

```bash
gh pr checks --watch
```

Expected: three green checks — `lint-test`, `build-backend`, `build-ingester`.

- [ ] **Step 2: Diagnose failures (if any)**

Common failure modes and fixes to apply inline:

- **`just test` fails due to missing services (e.g., Postgres/Timescale).** Two options:
  - (a) Narrow the CI test call: replace `just test` with `just frontend test` + `just backend test -short` (or a unit-only recipe) if one exists. Commit as `ci(tra-363): scope CI test to unit tests`.
  - (b) Add a `services:` block to the `lint-test` job with a `timescale/timescaledb` container and wire `DATABASE_URL` env. If more than ~15 lines of setup, split into a follow-up ticket and go with option (a) for now.
- **`pnpm install --frozen-lockfile` fails** because the pnpm version doesn't match the lockfile's `lockfileVersion: '9.0'`: change `pnpm/action-setup@v4`'s `version:` from `9` to the exact major/minor (e.g., `9.12.3`). Commit as `ci(tra-363): pin pnpm version`.
- **Backend Docker build fails at the `production` target.** Read `backend/Dockerfile` and confirm the target name is spelled `production` (already verified at plan time: line 34). If something changed, match the current target name.
- **GHCR login step runs on a PR.** It shouldn't — the `if: github.event_name != 'pull_request'` guard is on the step. If login fails on a PR, something modified that line; restore the guard.

Each fix is its own commit.

---

## Task 5: Post-merge verification

**Files:** none (observation task, runs after PR is merged to main)

- [ ] **Step 1: Watch the `main` run**

```bash
gh run watch
```

Expected: `build-backend` and `build-ingester` both green; `lint-test` also green.

- [ ] **Step 2: Confirm packages exist**

Open in a browser: <https://github.com/orgs/trakrf/packages>

Expected: `backend` and `ingester` packages listed.

- [ ] **Step 3: Flip each package to public visibility**

In the GitHub UI, for each package:
1. Click the package → `Package settings`
2. Under `Danger Zone` → `Change package visibility` → Public
3. Confirm by typing the package name

This is a one-time per-package action. Subsequent pushes retain visibility.

- [ ] **Step 4: Verify unauthenticated pull works**

From a workstation (or a machine where you're not logged into GHCR):

```bash
docker logout ghcr.io
docker pull ghcr.io/trakrf/backend:latest
docker pull ghcr.io/trakrf/ingester:latest
SHORT_SHA=$(git rev-parse --short HEAD)
docker pull ghcr.io/trakrf/backend:sha-${SHORT_SHA}
```

Expected: all three pulls succeed without prompting for credentials.

- [ ] **Step 5: Report image refs to TRA-361 owner**

Post a comment on TRA-361 with:
- `ghcr.io/trakrf/backend:latest` (floating) and `ghcr.io/trakrf/backend:sha-<short>` (pinned) are now available.
- Pull is unauthenticated.
- The migration Job can swap from `migrate/migrate` + ConfigMap to `ghcr.io/trakrf/backend:sha-<short>` with an appropriate entrypoint.

---

## Self-Review Notes

- **Spec coverage:** All four spec sections map to tasks — §1-2 → Task 2; §3 → Task 1; §4 verification → Tasks 4 + 5. The "just test deps" open question is handled explicitly in Task 4 Step 2 with two fallback paths.
- **No placeholders:** verified — all YAML is literal; all commands runnable.
- **Type consistency:** image names, tags, paths, and target names match the Dockerfile stages verified at plan time (`production` at `backend/Dockerfile:34`, single-stage `ingester/Dockerfile`).
