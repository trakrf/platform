# Prove batch enqueue against a real database

## Metadata
**Workspace**: backend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-1277
**Parent spec**: [../TRA-NaN-notification-service/spec.md](../TRA-NaN-notification-service/spec.md) (Phase 1, MR 06 of 06)
**Depends on**: MR 05
**Branch**: `feat/tra-1277-06-batch-integration-tests` from `feat/tra-1277-05-batch-enqueue`

## ELI5 (open the PR description with this)
This MR adds tests only. Against a real database, it checks the promises MR 05 made: sending the
same stack twice doesn't queue anything twice; if something breaks halfway, nothing is left
behind; and two workers doing the same stack at the same moment still produce exactly one of each.

## Outcome
`batch_integration_test.go` covering the spec's atomicity, idempotency, concurrency and RLS criteria for the outbox.

## Validation Criteria
- [ ] N commands → N rows with payload + `river_job_id`, N River jobs, `ok=N`
- [ ] Same batch again → all `Duplicate`; row and job counts unchanged; `duplicate=N`
- [ ] River insert fails on the 2nd command → error; 0 rows and 0 jobs for the org; no metrics
- [ ] Two concurrent identical batches → exactly N rows and N jobs in total
- [ ] Org B cannot read org A's rows
- [ ] `just backend lint` and `just backend test` pass

## Out of scope
Everything not listed above; see the parent spec's MR table. No caller is wired in this MR.

## References
- [Plan](plan.md)
