# Build Log: Batch enqueue into the outbox

## Session: 2026-10-09
Worktree: `.claude/worktrees/tra-1277-05` · Branch: `feat/tra-1277-05-batch-enqueue` (from MR 02 @ cf56a6ab)
Starting task: 1 · Total tasks: 4

Setup:
- `just bootstrap` (with `~/go/bin` on PATH) → "✅ Bootstrap complete"; `git status` clean afterwards.
- `go list -deps ./internal/storage` has no `notification/outbox`, so an in-package test file can import `storage` for a compile-time check without a cycle.
- River v0.47.0 `client.go:1851`: `func (c *Client[TTx]) InsertTx(ctx context.Context, tx TTx, args JobArgs, opts *InsertOpts) (*rivertype.JobInsertResult, error)`; `JobInserter` matches it with `TTx = pgx.Tx`.

### Task 1: `EnqueueDuplicate` — ✅ Complete
Files: `metrics.go`, `metrics_test.go`
- Test first: `TestMetrics_RecordEnqueue_CountsDuplicates` → build failure `undefined: outbox.EnqueueDuplicate` (red).
- Added the const → `--- PASS: TestMetrics_RecordEnqueue_CountsDuplicates`.
- Added an `enqueueCount` gather helper (the existing test's loop, made reusable) instead of pulling in `prometheus/testutil`, which nothing in the repo uses yet.

### Task 2–4: `EnqueueBatchTx`, `BatchEnqueuer`, unit tests — ✅ Complete
Files: `batch.go`, `batch_test.go`, `storage_contract_test.go`, `outbox.go`
- Tests first (`batch_test.go`, fakes only; `WithOrgTx` fake calls `fn(nil)` and records commit/rollback; inserter fails on call N) → build failure `undefined: batchStore` (red).
- Implemented `batch.go`; `outbox.go` only gains `const deliveryMaxAttempts = 6` (ladder comment kept) used by `Enqueue`.
- Compile-time checks: `var _ JobInserter = (*river.Client[pgx.Tx])(nil)` in `batch.go`; `var _ batchStore = (*storage.Storage)(nil)` in `storage_contract_test.go` (package `outbox`, test-only so production code still doesn't import storage).
- `go test ./internal/notification/outbox/ -v` → 5 new batch tests PASS, all 16 unit tests PASS.

### Gates
- `just lint` (backend) → `✓ check-rls-guard: clean`, fmt/vet clean, exit 0.
- `go build ./...` → exit 0.
- `go test ./...` → 64 packages `ok`, 0 failures.
- `flock …/trakrf_test.lock go test -tags=integration -p 1 -count=1 -v ./internal/notification/outbox/` → `ok` (14.5s); 21 PASS, 0 FAIL, 0 SKIP (includes the existing `TestEnqueue_SourceRollbackLeavesNoJobAndNoAuditRow` and the worker/runtime tests).
- `flock …/validate.lock just validate` → exit 0: 69 Go `ok` lines; frontend `Tests 2605 passed | 33 skipped`; script tests 99/37/21/43 passed, 0 failed; CLI 25 passed, 0 failed. `git status` afterwards shows only this MR's files.

## Summary
Total tasks: 4 · Completed: 4 · Failed: 0
Deviations: none from the plan's code shape. `JobInserter`'s compile-time assertion sits in `batch.go` (production) rather than a test file.
