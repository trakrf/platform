# Build Log: Persist routing context on delivery rows

## Session: 2026-10-09
Worktree: `.claude/worktrees/tra-1277-02` · Branch: `feat/tra-1277-02-delivery-columns` (on MR 01's migration commit 1dd84172)
Starting task: 1 · Total tasks: 4

Setup:
- `EnterWorktree` refused (the parent session runs from the repo root), so every command runs with the worktree as an absolute path.
- `just bootstrap` with the brief's PATH → "✅ Bootstrap complete"; `git status` clean afterwards.
- The unique index `idx_notification_deliveries_org_delivery` on `(org_id, delivery_id)` (migration 000044) is the `ON CONFLICT` target.

### Task 4 first (TDD): integration tests — red
File: `backend/internal/storage/notification_deliveries_integration_test.go`
- `TestNotificationDelivery_InsertIfAbsentRoundTripsRoutingContext`: seeds a real asset (`testutil.CreateTestAsset`) and recipient (`CreateNotificationRecipient`) for the FKs; round-trips all four fields through both getters (payload with `require.JSONEq`); a second insert with the same `delivery_id` but a different event and payload returns `(0, false, nil)` and leaves the row unchanged.
- `TestNotificationDelivery_InsertIfAbsentNilRoutingContextReadsBackNil`: unset fields and an empty (non-nil) payload read back nil.
- `go vet -tags=integration ./internal/storage/` → `unknown field EventID in struct literal of type notificationdelivery.NotificationDelivery` (expected red).

### Task 1: model and `Command` fields — ✅ Complete
- `NotificationDelivery` gains `EventID *string`, `RecipientID *int`, `AssetID *int`, `Payload []byte`.
- `outbox.Command` gains `EventID string`, `RecipientID int`, `AssetID int`; the `Payload` comment now says the batch path persists it and it never goes in River job args or logs. `Enqueue` is unchanged.

### Task 2: `InsertNotificationDeliveryIfAbsentTx` — ✅ Complete
- Signature as planned: `(ctx, tx pgx.Tx, orgID int, d NotificationDelivery) (int64, bool, error)`.
- `ON CONFLICT (org_id, delivery_id) DO NOTHING RETURNING id`; `pgx.ErrNoRows` → `(0, false, nil)`; an empty payload is sent as NULL.
- `InsertNotificationDeliveryTx` is unchanged.

### Task 3: read the new columns — ✅ Complete
- `notificationDeliveryColumns` gains `event_id, recipient_id, asset_id, payload`; the new `scanNotificationDelivery` helper is used by both getters. It also does the channel/state string conversion the getters used to repeat.
- pgx v5 sent `[]byte` to the `jsonb` parameter as raw JSON with no cast, so the plan's `json.RawMessage` / `::jsonb` fallback was not needed.

### Gates
- `flock … go test -tags=integration -p 1 -count=1 ./internal/storage/ -run TestNotificationDelivery -v` → 3 `--- PASS`, `ok internal/storage 6.139s` (green).
- `just lint` (backend) → `✓ check-rls-guard: clean`, fmt/vet clean, exit 0.
- `go build ./...` → exit 0.
- `go test -count=1 ./...` → 64 packages `ok`, 0 failures.
- `flock … go test -tags=integration -p 1 -count=1 ./internal/storage/ ./internal/notification/outbox/ -v` → `ok internal/storage 364.628s`, `ok internal/notification/outbox 16.954s`; 293 `--- PASS`, 0 `--- FAIL`, 8 `--- SKIP` (all pre-existing `t.Skip` placeholders such as `users_test.go` "Requires test database"). The outbox and worker tests (`TestEnqueue_*`, `TestWorker_*`, `TestDeliveryWorker_*`) are unchanged and pass.
- Test tidy-up after that run: assertions moved out of the `WithOrgTx` closures so an insert error is reported before the `inserted` check. Re-ran `-run TestNotificationDelivery` under the lock → 3 `--- PASS`, `ok … 6.300s`.
- `flock validate.lock just validate` (worktree root) → exit 0. Go: 69 packages `ok`, 0 `--- FAIL`. Frontend: `Test Files 239 passed | 2 skipped`, `Tests 2605 passed | 33 skipped`. Script tests: `25 passed, 0 failed`, `passed: 99 failed: 0`, `passed: 37 failed: 0`, `database readiness: 0 failed assertions`. `git status` afterwards: only the 4 intended files and this log.

## Summary
Total tasks: 4 · Completed: 4 · Failed: 0
Deviations: none from the plan's code; `EnterWorktree` could not be used (see Setup).
Ready for /check: YES
