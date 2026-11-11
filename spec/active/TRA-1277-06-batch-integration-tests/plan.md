# Implementation Plan: Prove batch enqueue against a real database
Generated: 2026-10-09
Specification: spec.md · Parent: ../TRA-NaN-notification-service/spec.md

## ELI5
This MR adds tests only. Against a real database, it checks the promises MR 05 made: sending the
same stack twice doesn't queue anything twice; if something breaks halfway, nothing is left
behind; and two workers doing the same stack at the same moment still produce exactly one of each.

## Files
**Create**
- `backend/internal/notification/outbox/batch_integration_test.go`

**Modify**
- none

**Reference patterns**
- backend/internal/notification/outbox/outbox_integration_test.go:22-95: River client from `NewRuntime` (never started), counting `trakrf.river_job` by kind via `db.AdminPool`
- `fakeAdapter` (`worker_test.go:30`, no build tag) and `alwaysEntitled`: reuse, don't redeclare

## Tasks
### Task 1: Fixtures
`SetupTestDBFull`, `CreateTestAccount`, `CreateTestAsset`, a real recipient (FKs), River client via
`outbox.NewRuntime(db.AppPool, fakeAdapter{}, db.Store, metrics)` (not started), fresh `prometheus.NewRegistry()` per test.

### Task 2: Happy path + replay

### Task 3: Injected failure
Wrap `runtime.Client` in a `JobInserter` that fails on call 2.

### Task 4: Concurrency
Two goroutines released by a shared start channel (no sleeps); assert totals, not per-call results.

### Task 5: RLS
`GetNotificationDeliveryByDeliveryID(orgB, id)` → not found.

## Risks
* Flaky concurrency: use a start barrier and assert only on final counts.

## Validation gates (every task)
`just bootstrap` once in a fresh worktree (else `go:embed` targets are missing), then after each change:
`just backend lint` → `go build ./...` → `just backend test` for the touched package →
`just backend test-integration <pkg>` where integration tests changed (needs local Postgres; `just database up`).
Any failure: fix and re-run. 3 failed attempts on the same gate → stop and ask.

## Ship
PR title `feat: …` (no ticket id in published prose); body opens with the ELI5 above, then the
Validation Criteria with the exact command output. Base the PR on `feat/tra-1277-05-batch-enqueue`.
