# CLAUDE.md

## Package Managers
- **Backend**: `go mod`, `go get`
- **Frontend**: `pnpm` exclusively (`pnpm dlx` instead of `npx`)

## Task Runner (Just)
- **Run everything from the project root** — delegate with `just <workspace> <cmd>` rather than `cd`-ing in
- Workspaces: `frontend`, `backend`, `cli`, `database` (aliases `fe`, `be`, `db`)
- Combined across all four: `just lint`, `just test`, `just build`, `just validate`
- Every workspace justfile sets `fallback := true`, so root recipes still resolve from inside one

## Cluster Ops
- `just ops <recipe> [args]` forwards to the trakrf/infra justfile; bare `just ops` lists what's available
- Shortcuts: `just gcp-auth`, `just psql preview`, `just logs prod 1h`
- Cluster/namespace/pod knowledge stays in infra — never reimplement a kubectl incantation here
- Infra checkout is `TRAKRF_INFRA_DIR`, else a sibling `infra/`; set it in `.env.local` if yours is elsewhere

## Git Workflow
- **Never push directly to main** — all changes via PR
- **Never squash merge** — `gh pr merge --merge`
- Branch naming: `<type>/tra-NNNN-slug`, e.g. `feat/tra-1065-kitting-capability`, `fix/broken-xyz`, `chore/...`, `docs/...`
- Conventional commits: `feat:`, `fix:`, `docs:`, `chore:`
- Prefer incremental commits over amending

## Migrations
- Adding a migration **requires** `just backend migrate-checksums` — CI fails without it, and applied migrations are immutable

## Testing
- Playwright e2e **never runs in CI** — green CI does not mean e2e passes; run it against preview yourself
- **Name specs and tooling for what they do, never for a ticket** — `locate-mask-length-variants.spec.ts`, not `tra-1120-…`. Cite the ticket in the file header. Applies to `describe` blocks, log prefixes and artifact dirs, since those appear in output
- **Re-used vs merely re-read** — reusable things live in the repo under a behaviour name; point-in-time records go on the ticket, and working notes in `docs/notes/` stay untracked (see `.gitignore`)

## Hardware access
- **`ble-mcp-test` is test tooling only, never the product path** — the app reaches a CS108 solely via browser `navigator.bluetooth`
- **One connection at a time**: a running bridge blocks preview/prod hand-testing, and an idle bridge port does *not* mean the reader is free — see `docs/ble-hardware-access.md`

## Preview Deployments
- Opening/updating a PR auto-deploys to `https://app.preview.trakrf.id`
- See `.github/workflows/sync-preview.yml` for details

## Stack
- Go backend, React/TypeScript frontend, TimescaleDB, Go CLI, MQTT ingestion (`mqtt-rpc/`)
- Architecture context: `README.md`, `docs/architecture-decisions.md`, `docs/adr/`, `docs/logical-schema.md`

## Worktrees
- Git worktrees live in `.claude/worktrees/` (gitignored) — where the native `EnterWorktree` tool writes
- **`just bootstrap` is the first thing to run in a new worktree** — installs deps and generates the two gitignored `go:embed` targets. Without it `just validate` fails with `pattern frontend/dist: no matching files found`, naming neither the step nor the fix
- Idempotent and near-instant when warm, so re-run it freely. Cold it takes a couple of minutes with no decisions in it — **run it backgrounded** at session start and let planning overlap it, rather than blocking or delegating it to a subagent
- **An unbootstrapped tree can invalidate a verification, not just delay it.** Everything fails, so a deliberate-break check records the failure it expected and proves nothing. If a check fails, confirm bootstrap ran before believing the result
