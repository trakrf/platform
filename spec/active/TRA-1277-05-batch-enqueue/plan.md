# Implementation Plan: Batch enqueue into the outbox
Generated: 2026-10-09
Specification: spec.md · Parent: ../TRA-NaN-notification-service/spec.md

## ELI5
We can now drop a whole stack of messages into the outbox in one go. Either the whole
stack goes in or none of it does. A message already in the outbox is counted as a "duplicate"
and not queued again, and the counters are updated only once the stack is safely in.

## Files
**Create**
- `backend/internal/notification/outbox/batch.go`
- `backend/internal/notification/outbox/batch_test.go`

**Modify**
- `backend/internal/notification/outbox/metrics.go`
- `backend/internal/notification/outbox/metrics_test.go`
- `backend/internal/notification/outbox/outbox.go` (shared `deliveryMaxAttempts` const only)

**Reference patterns**
- backend/internal/notification/outbox/outbox.go: `Enqueue` (row → `InsertTx` → link job id in one `WithOrgTx`)
- backend/internal/notification/outbox/outbox_test.go: fake style

## Tasks
### Task 1: `EnqueueDuplicate`
Add `EnqueueDuplicate EnqueueResult = "duplicate"` to `metrics.go`; extend `metrics_test.go`.

### Task 2: `EnqueueBatchTx`
```go
type BatchResult struct {
    DeliveryID             string
    NotificationDeliveryID int64 // 0 when Duplicate
    RiverJobID             int64 // 0 when Duplicate
    Duplicate              bool
}

// JobInserter: *river.Client[pgx.Tx] satisfies it; an interface so tests can fail the Nth insert.
type JobInserter interface {
    InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// EnqueueBatchTx runs in the caller's transaction and never commits or rolls back.
// Any error means the caller must roll back. Records no metrics.
func EnqueueBatchTx(ctx context.Context, tx pgx.Tx, store batchTxStore, jobs JobInserter, cmds []Command) ([]BatchResult, error)
```
Per command: `InsertNotificationDeliveryIfAbsentTx`; not inserted → `Duplicate`, continue;
else `jobs.InsertTx(DeliveryJobArgs{OrgID, NotificationDeliveryID}, MaxAttempts: deliveryMaxAttempts)`
→ `SetNotificationDeliveryRiverJobIDTx`. Wrap errors with the `DeliveryID` only.
Move `6` into `const deliveryMaxAttempts = 6` (keep the TRA-398 comment) and use it in `Enqueue` too.

### Task 3: `BatchEnqueuer`
```go
func NewBatchEnqueuer(store batchStore, jobs JobInserter, metrics *Metrics) *BatchEnqueuer
func (e *BatchEnqueuer) EnqueueBatch(ctx context.Context, orgID int, cmds []Command) ([]BatchResult, error)
```
Empty → `nil, nil`. Org mismatch → error. One `WithOrgTx` around `EnqueueBatchTx`. On error return
it (nothing recorded). After commit: `RecordEnqueue(EnqueueOK)` or `RecordEnqueue(EnqueueDuplicate)` per result.

### Task 4: Unit tests (`batch_test.go`, fakes only)
In-memory store whose `WithOrgTx` calls `fn(nil)` and records commit/rollback; inserter that can
fail on call N. Cover every acceptance criterion above.

## Risks
* Partial commit: `EnqueueBatchTx` returns on the first error; `WithOrgTx` rolls back. Proven against a real DB in MR 06.
* Same `DeliveryID` twice in one batch: `ON CONFLICT` makes the second a `Duplicate`; no special case.

## Validation gates (every task)
`just bootstrap` once in a fresh worktree (else `go:embed` targets are missing), then after each change:
`just backend lint` → `go build ./...` → `just backend test` for the touched package →
`just backend test-integration <pkg>` where integration tests changed (needs local Postgres; `just database up`).
Any failure: fix and re-run. 3 failed attempts on the same gate → stop and ask.

## Ship
PR title `feat: …` (no ticket id in published prose); body opens with the ELI5 above, then the
Validation Criteria with the exact command output. Base the PR on `feat/tra-1277-02-delivery-columns`.
