# List an asset's switched-on subscribers

## Metadata
**Workspace**: backend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-1277
**Parent spec**: [../TRA-NaN-notification-service/spec.md](../TRA-NaN-notification-service/spec.md) (Phase 1, MR 04 of 06)
**Depends on**: none (uses 000047 from `main`)
**Branch**: `feat/tra-1277-04-subscriber-listing` from `origin/main`

## ELI5 (open the PR description with this)
Given an asset, we can now get the list of people who asked to hear about it. Only
subscriptions that are still switched on are listed. For each person we also say whether they
are paused or deleted, so the next step can explain why they were skipped.

## Outcome
`(*Storage).ListSubscribersForAsset(ctx, orgID, assetID) ([]notificationrecipient.Subscriber, error)`: one org-scoped join, filtered on `s.is_active`.

## Validation Criteria
- [x] No subscriptions → empty, non-nil slice
- [x] One recipient on email and SMS → two rows
- [x] Switched-off subscription → absent
- [x] Inactive recipient → present with `RecipientActive=false`
- [x] Another org's subscriptions never returned
- [x] `just backend lint` and the backend Go tests pass (run via `just validate`: 69 packages `ok`), plus `./internal/storage/` under `-tags=integration`

## Out of scope
Everything not listed above; see the parent spec's MR table. No caller is wired in this MR.

## References
- [Plan](plan.md)
