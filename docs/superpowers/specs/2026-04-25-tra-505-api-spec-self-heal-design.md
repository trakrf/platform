# TRA-505 — `just backend api-spec` Self-Heals Frontend `dist/` Stub

- **Linear**: [TRA-505](https://linear.app/trakrf/issue/TRA-505/just-backend-api-spec-should-self-heal-frontenddist-stub)
- **Date**: 2026-04-25
- **Status**: Design approved, pending implementation plan

## Context

`backend/main.go:36` declares `//go:embed frontend/dist`. Whenever `swag init --parseDependency --parseInternal` walks `main.go` (which it does in the `api-spec` recipe), Go's package parser must be able to resolve that embed target. If `backend/frontend/dist/` is empty or missing, the `main` package partially fails to resolve and `swag` falls back to fully-qualified Go package names in schema definitions — e.g. `assets.CreateAssetWithIdentifiersRequest` becomes `github_com_trakrf_platform_backend_internal_handlers_assets.CreateAssetWithIdentifiersRequest`.

The result on a clean checkout: running `just backend api-spec` produces ~700 lines of unrelated diff churn in `docs/api/openapi.public.{json,yaml}` and the embedded `backend/internal/handlers/swaggerspec/openapi.{public,internal}.{json,yaml}` files.

CI is unaffected because the frontend build runs first and produces real `dist/` output. Local dev hits this any time someone touches a swag annotation without first running `pnpm build` from `frontend/`.

Discovered during [TRA-499](https://linear.app/trakrf/issue/TRA-499). The TRA-499 implementer worked around it by manually creating a stub `backend/frontend/dist/index.html` (matched by `.gitignore: dist/`, so it stays untracked). This ticket bakes that workaround into the recipe so future implementers don't have to know.

## Goals

1. Running `just backend api-spec` on a clean checkout (no prior frontend build) produces a diff bounded to intentional annotation changes — no fully-qualified Go package names anywhere in the generated specs.
2. Existing real `frontend/dist/` artifacts (post `pnpm build`) are left untouched — no mtime bumps on `index.html`.
3. CI behavior is unchanged.

## Non-goals

- No regression test or new CI check. Existing `api-spec.yml` drift check already produces canonical output because CI builds the frontend before generating specs.
- No refactor into a shared `_ensure-frontend-stub` recipe — only one consumer today; YAGNI.
- No fix for `lint`/`test`/`test-integration`. They invoke the Go toolchain over `./...` which would also trip on the missing embed target, but no one has reported the symptom there. Out of scope for this ticket.
- No change to `main.go`'s `//go:embed` directive itself.

## Behavior

| Starting state of `backend/frontend/dist/index.html` | Recipe action | Effect on file |
|---|---|---|
| Absent | `mkdir -p frontend/dist && touch frontend/dist/index.html` | Created as empty stub. Untracked (matched by `.gitignore`). |
| Present (real or stub) | No-op | mtime preserved. |

`swag init` then runs against a resolvable `main` package and emits short schema names.

## Changes

### `backend/justfile`

Insert a guard at the top of the `api-spec` recipe (currently lines 75–86):

```just
# Generate OpenAPI 3.0 specs (public → docs/api/ + embedded; internal → embedded only)
api-spec:
    @# swag's --parseDependency walks main.go, which has //go:embed frontend/dist.
    @# Without a real or stub file there, swag falls back to fully-qualified Go
    @# package names in schema definitions (~700 lines of churn). See TRA-505.
    @[ -e frontend/dist/index.html ] || (mkdir -p frontend/dist && touch frontend/dist/index.html)
    @echo "📚 Generating OpenAPI 3.0 specs..."
    swag init -g main.go --parseDependency --parseInternal -o docs
    @mkdir -p internal/handlers/swaggerspec ../docs/api
    go run ./internal/tools/apispec \
        --in docs/swagger.json \
        --public-out ../docs/api/openapi.public \
        --internal-out internal/handlers/swaggerspec/openapi.internal
    @cp ../docs/api/openapi.public.json internal/handlers/swaggerspec/openapi.public.json
    @cp ../docs/api/openapi.public.yaml internal/handlers/swaggerspec/openapi.public.yaml
    @echo "✅ Public spec:   docs/api/openapi.public.{json,yaml}  (committed) + swaggerspec/ (gitignored, embedded)"
    @echo "✅ Internal spec: backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml}  (gitignored, embedded)"
```

Only the comment block (3 `@#` lines) and the `[ -e ... ] || (...)` guard line are added. The rest of the recipe is unchanged.

No other files are modified.

## Testing

Manual verification during implementation. No automated tests added (see Non-goals).

1. **Stub-creation path** (the bug being fixed):
   ```sh
   rm -rf backend/frontend/dist
   git status --short backend/frontend/   # confirms clean
   just backend api-spec
   git diff --stat docs/api/ backend/internal/handlers/swaggerspec/
   ```
   Expected: diff is bounded to intentional changes; `git grep -l 'github_com_trakrf' docs/api/ backend/internal/handlers/swaggerspec/` returns nothing.

2. **No-op path** (real artifacts preserved):
   ```sh
   pnpm --dir frontend build
   stat -c %Y backend/frontend/dist/index.html > /tmp/mtime.before
   just backend api-spec
   stat -c %Y backend/frontend/dist/index.html > /tmp/mtime.after
   diff /tmp/mtime.{before,after}
   ```
   Expected: `diff` is empty (mtime unchanged), proving the conditional branch took the no-op path.

3. **CI smoke**: open the PR and confirm `api-spec.yml` drift check passes. No new failures expected.

## Edge cases

- **Race with `pnpm build` running concurrently**: not a real concern. Local dev is single-actor; CI builds frontend before backend recipes.
- **`frontend/dist/` exists as a *directory* but no `index.html` inside**: the guard's `[ -e .../index.html ]` is false, so it falls through to `mkdir -p` (no-op since dir exists) and `touch` creates the stub file. Correct behavior.
- **Symlinked `frontend/dist`**: `[ -e ... ]` follows symlinks, so a symlink pointing at a real built directory is treated as present. Also correct.

## Rollout

- Single PR on `worktree-tra-505` (Linear's canonical branch name `miks2u/tra-505-...` was not used — branch name is cosmetic, the PR will link to the ticket via title/body).
- No preview-deployment behavior change expected.
- Per the user's batched black-box verification preference, no per-ticket preview check; folds into the next batch.

## Discovered during verification

Verification revealed one residual fully-qualified Go package name in the (gitignored) internal spec: `github_com_trakrf_platform_backend_internal_models_user.User`. Root cause is unrelated to the missing-embed bug — it's a name collision between `internal/handlers/users` (plural) and `internal/models/user` (singular), which swag disambiguates via FQN. The public spec served to API consumers is unaffected; the internal spec routes are inside the session-auth middleware group. Tracked separately as [TRA-507](https://linear.app/trakrf/issue/TRA-507) (post-v1); not blocking this ticket.
