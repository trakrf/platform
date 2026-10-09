# Delivery routing schema (migration 000048)

## Metadata
**Workspace**: backend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-1277
**Parent spec**: [../TRA-NaN-notification-service/spec.md](../TRA-NaN-notification-service/spec.md) (Phase 1, MR 01 of 06)
**Depends on**: none
**Branch**: `feat/tra-1277-01-delivery-schema` from `origin/main`

## ELI5 (open the PR description with this)
We are adding new empty boxes to the database. Every "message to send" record
gets boxes for *which event*, *which person*, *which asset* and *the message itself*. We also
add a brand-new "do not contact" list. Nothing writes to any of these yet: this MR only builds
the shelves.

## Outcome
Migration 000048 adds nullable `event_id`, `recipient_id`, `asset_id`, `payload` to `notification_deliveries` and creates `notification_suppressions`; checksums updated.

## Validation Criteria
- [ ] `just backend migrate` applies 000048 cleanly
- [ ] `just backend migrate-down` then `just backend migrate` round-trips cleanly
- [ ] `git diff origin/main -- backend/migrations/checksums.txt` adds exactly 2 lines, changes none
- [ ] `go test ./migrations/` passes
- [ ] `just backend lint` and `just backend test` pass

## Out of scope
Everything not listed above; see the parent spec's MR table. No caller is wired in this MR.

## References
- [Plan](plan.md)
