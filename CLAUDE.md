# CLAUDE.md

## Package Managers
- **Backend**: `go mod`, `go get`
- **Frontend**: `pnpm` exclusively (`pnpm dlx` instead of `npx`)

## Working Directory
- **Always run commands from the project root** — use `just frontend <cmd>` / `just backend <cmd>` delegation rather than `cd`-ing into subdirectories

## Task Runner (Just)
- Root delegates: `just frontend <cmd>`, `just backend <cmd>`
- Combined: `just lint`, `just test`, `just validate` runs both workspaces
- Workspace justfiles have `set fallback := true` for root recipes
- From workspace dirs, unqualified commands run local recipes

## Git Workflow
- **Never push directly to main** — all changes via PR
- Branch naming: `feature/add-xyz`, `fix/broken-xyz`, `docs/update-xyz`
- Conventional commits: `feat:`, `fix:`, `docs:`, `chore:`
- Prefer incremental commits over amending

## Preview Deployments
- Opening/updating a PR auto-deploys to `https://app.preview.trakrf.id`
- See `.github/workflows/sync-preview.yml` for details

## Stack
- Go backend, React/TypeScript frontend, TimescaleDB
- Read `PLANNING.md` for architecture and project context

## Worktrees
- Git worktrees live in `.claude/worktrees/` — where the native `EnterWorktree` tool writes and where `claude -w` resume looks. The superpowers worktree skill defers to the native tool. Covered by the `.claude/` gitignore; no symlink.

## Verification
- Run relevant tests before claiming completion
- Report actual test results — no false optimism
