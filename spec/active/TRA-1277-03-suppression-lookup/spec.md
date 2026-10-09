# Opt-out lookup (`IsSuppressed`)

## Metadata
**Workspace**: backend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-1277
**Parent spec**: [../TRA-NaN-notification-service/spec.md](../TRA-NaN-notification-service/spec.md) (Phase 1, MR 03 of 06)
**Depends on**: MR 01
**Branch**: `feat/tra-1277-03-suppression-lookup` from `feat/tra-1277-01-delivery-schema`

## ELI5 (open the PR description with this)
Before we message someone, we need to ask: "did this person tell us to stop?" This MR
adds that question. It ignores upper/lower case in the address, ignores opt-outs that were
later cleared, and if the database can't answer, it says "I don't know" (an error), never "go ahead".

## Outcome
`(*Storage).IsSuppressed(ctx, orgID, channel, address) (bool, error)`, matching the spec's `SuppressionChecker`.

## Validation Criteria
- [x] Uncleared row → suppressed (integration subtest `uncleared_row`)
- [x] Different case (`Foo@X.test` vs `foo@x.test`) → suppressed (`different_case`)
- [x] Cleared row → not suppressed (`cleared_row`)
- [x] Same address on the other channel → not suppressed (`same_address,_other_channel`)
- [x] Same address in another org → not suppressed (`same_address,_other_org`)
- [x] `just backend check-rls-guard` clean (run as part of `just backend lint`: `✓ check-rls-guard: clean`)
- [x] `just backend lint` and `just backend test` pass (lint clean; Go tests run via `just validate`, 69 packages ok)

## Out of scope
Everything not listed above; see the parent spec's MR table. No caller is wired in this MR.

## References
- [Plan](plan.md)
