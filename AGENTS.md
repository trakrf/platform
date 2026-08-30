# AGENTS.md — trakrf working agreements

Byte-identical in `docs`, `platform` and `infra`; each repo's `CLAUDE.md` imports
it and adds its own specifics. Public repos — write for a world reader.

Only what a fresh session gets wrong otherwise. Branch, commit and merge rules
are in `CONTRIBUTING.md`, tooling choices in `.claude/csw.json`; neither is
restated here. `ble-mcp-test` is not in this family and keeps its own.

## Specs, plans and notes are never tracked

Per-session artifacts, disposable by decision. Ignored: `superpowers/`,
`docs/superpowers/`, `.superpowers/`, `docs/notes/` — each repo's `CLAUDE.md`
names which to write to. A plan worth a reviewer's time goes in the PR body.

**Re-used, or merely re-read?** Re-used lives in the repo under a _behaviour_
name — `describe` blocks, log prefixes and artifact dirs included. Merely
re-read belongs on the ticket, with the narrative and evidence. Durable
conclusions go to `CLAUDE.md`, an ADR or a code comment. Keep ticket ids out of
published prose.

## Working rules

- Ask when requirements are unclear; never delete code without instruction
- Verify before claiming done: run it, read the output, report what it said
- Verify the premise, not just the citation
- Search wide, then filter — absence from a narrowed search is not absence
- No invented imports; keep files under 500 lines
