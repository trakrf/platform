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
- **Name specs for the behaviour they cover, never for a ticket** — `locate-mask-length-variants.spec.ts`, not `tra-1120-locate-ambiguous-width.spec.ts`. Put the ticket reference in the file header instead. A regression spec outlives its ticket, so a ticket-named file misattributes its own failures the moment the ticket closes — a reader sees in-flight work where the truth is permanent coverage. Applies to the `describe` block too, since that is what appears in test output.

## Preview Deployments
- Opening/updating a PR auto-deploys to `https://app.preview.trakrf.id`
- See `.github/workflows/sync-preview.yml` for details

## Stack
- Go backend, React/TypeScript frontend, TimescaleDB, Go CLI, MQTT ingestion (`mqtt-rpc/`)
- Architecture context: `README.md`, `docs/architecture-decisions.md`, `docs/adr/`, `docs/logical-schema.md`

## Worktrees
- Git worktrees live in `.claude/worktrees/` (gitignored) — where the native `EnterWorktree` tool writes
