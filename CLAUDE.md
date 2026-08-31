# CLAUDE.md

@AGENTS.md

Platform specifics only. Session artifacts go to `docs/superpowers/` and `docs/notes/`.

## Stack
Go backend · React/TypeScript frontend · TimescaleDB · Go CLI · MQTT ingestion (`mqtt-rpc/`).
Architecture: `README.md`, `docs/architecture-decisions.md`, `docs/adr/`, `docs/logical-schema.md`.

## Tooling
- Backend `go mod`; frontend `pnpm` only — `pnpm dlx`, never `npx`
- Run `just` from the project root: `just <workspace> <cmd>`, workspaces `frontend` `backend` `cli` `database` (`fe` `be` `db`)
- `just ops <recipe>` forwards to trakrf/infra and bare `just ops` lists them; cluster knowledge stays there. Checkout via `TRAKRF_INFRA_DIR`, else sibling `infra/`

## Gotchas
- A new migration requires `just backend migrate-checksums`; applied migrations are immutable
- Playwright e2e never runs in CI — run it against preview yourself
- `just bootstrap` a fresh worktree before validating, or the `go:embed` targets are missing and every check fails for that reason alone
- Opening a PR auto-deploys to `https://app.preview.trakrf.id` (`.github/workflows/sync-preview.yml`)

## Hardware
- `ble-mcp-test` is test tooling only; the app reaches a CS108 via browser `navigator.bluetooth`
- One connection at a time: a connected client blocks hand-testing, a running daemon does not
- The reader is SHARED with the ble-mcp-test session (cc2cc: `bridge`). It changes hands on an explicit message, never on a `held: false` poll — a free port is equally consistent with the other side being *between* attempts. Announce before connecting, when done, and again before a retry. INTERIM: a convention with no red state, standing in for a real lock being designed in ble-mcp-test (TRA-1221) — replace this bullet when that lands, do not keep both
- An idle bridge port does not mean the reader is free; `get_connection_state` narrows that question but does not settle it — see `docs/ble-hardware-access.md`
- The bridge is a supervised `systemctl --user` unit; `pkill` returns in 5s
