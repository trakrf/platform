# Batch enqueue into the outbox

## Metadata
**Workspace**: backend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-1277
**Parent spec**: [../TRA-NaN-notification-service/spec.md](../TRA-NaN-notification-service/spec.md) (Phase 1, MR 05 of 06)
**Depends on**: MR 02
**Branch**: `feat/tra-1277-05-batch-enqueue` from `feat/tra-1277-02-delivery-columns`

## ELI5 (open the PR description with this)
We can now drop a whole stack of messages into the outbox in one go. Either the whole
stack goes in or none of it does. A message already in the outbox is counted as a "duplicate"
and not queued again, and the counters are updated only once the stack is safely in.

## Outcome
`outbox.EnqueueBatchTx` (caller's tx) and `outbox.BatchEnqueuer.EnqueueBatch` (own tx, metrics after commit); new `EnqueueDuplicate` metric result.

## Validation Criteria
- [x] Empty batch → no transaction opened (`TestEnqueueBatch_EmptyBatchOpensNoTransaction`)
- [x] Command `OrgID` ≠ batch org → error before any transaction (`TestEnqueueBatch_OrgMismatchRejectedBeforeTransaction`)
- [x] Mixed inserted/duplicate → results in input order; duplicates get no River insert (`TestEnqueueBatch_MixedInsertedAndDuplicateKeepsInputOrder`, fakes; real-DB proof is MR 06)
- [x] Failed batch → zero metrics recorded (`TestEnqueueBatch_FailedBatchRecordsNoMetrics`)
- [x] Error text contains no payload bytes (same test: names the failing `DeliveryID`, not the payload's address or body)
- [x] `trakrf_outbox_enqueues_total{result="duplicate"}` increments (`TestMetrics_RecordEnqueue_CountsDuplicates`, and via `EnqueueBatch` in the mixed test)
- [x] `just backend lint` and `just backend test` pass (also `just validate` exit 0)

## Out of scope
Everything not listed above; see the parent spec's MR table. No caller is wired in this MR.

## References
- [Plan](plan.md)
