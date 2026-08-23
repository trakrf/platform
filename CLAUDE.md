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
- **Name specs and tooling for what they do, never for a ticket** — `locate-mask-length-variants.spec.ts`, not `tra-1120-locate-ambiguous-width.spec.ts`; `characterise-suite-runs.mjs`, not `tra-1167-characterise.mjs`. Put the ticket reference in the file header instead. Anything reusable outlives its ticket, so a ticket-named file misattributes its own output the moment the ticket closes — a reader sees in-flight work where the truth is permanent tooling or coverage. Applies to `describe` blocks, log prefixes, and artifact directories too, since those are what appear in output.
  - **The exception is a point-in-time record**, which is *about* one investigation and does not outlive it: `docs/investigations/tra-1167-phase1-record.md` is correctly ticket-named, the same way a dated design doc is correctly date-named. The test is whether the artifact will be re-used or merely re-read.

## The BLE bridge is TEST TOOLING ONLY — and it holds the radio exclusively

**`ble-mcp-test` is never part of the product.** The app has exactly one way to reach a CS108: **direct Web Bluetooth**, `navigator.bluetooth` in the browser. That is the path in prod, in preview, and in any normal build. There is no bridge in the product path and there never should be.

The bridge exists solely so **tests** can drive real hardware from Node and from headless browsers. It is injected **only** when `VITE_BLE_BRIDGE_ENABLED === 'true'` (`frontend/vite.config.ts:40`, explicit early return otherwise) — set by integration tests and `pnpm dev:bridge`, never by a preview or prod build.

**Why that still constrains you:** a CS108 accepts **one connection at a time**. Whatever holds the radio excludes everything else — so test tooling that is connected blocks the real product path to that reader, and a browser session blocks the tests. That much is a property of the hardware and outlives any bridge implementation.

The inverse is the one that misleads: **an idle bridge port does not mean the reader is free.** Someone may be holding it from a browser, and that never appears as a bridge client.

> **Current implementation — expected to change.** Today's bridge (`rust-ble-test`) connects at
> **process start** and holds the link for its whole lifetime; a client disconnecting releases
> nothing, and only `SIGTERM` frees the radio. So today you must **stop the process** — closing the
> tests is not enough — and `pgrep -f 'rust-bl[e]-test'` is how you tell whether it is holding.
> `rust-ble-test` **goes away** in the replatform to `bleak`-based Python tooling, where the
> intent — not yet a guarantee — is to release the radio whenever no mock-to-bridge connection is
> active. If that lands, the rule relaxes from *"the bridge process must not be running"* to
> *"no test must be connected"*, and leaving the bridge up between runs stops being a problem.
> **Verify that behaviour before relying on it, and re-check this paragraph after the replatform.
> The paragraphs above it hold either way.**

See `reference_ble_bridge_restart` for start/stop.

## Preview Deployments
- Opening/updating a PR auto-deploys to `https://app.preview.trakrf.id`
- See `.github/workflows/sync-preview.yml` for details

## Stack
- Go backend, React/TypeScript frontend, TimescaleDB, Go CLI, MQTT ingestion (`mqtt-rpc/`)
- Architecture context: `README.md`, `docs/architecture-decisions.md`, `docs/adr/`, `docs/logical-schema.md`

## Worktrees
- Git worktrees live in `.claude/worktrees/` (gitignored) — where the native `EnterWorktree` tool writes
