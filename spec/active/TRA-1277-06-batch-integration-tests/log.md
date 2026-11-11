# Build Log: Prove batch enqueue against a real database

## Session: 2026-10-09
Worktree: `.claude/worktrees/tra-1277-06` · Branch: `feat/tra-1277-06-batch-integration-tests` (on MR 05 @ 11edec23)
Total tasks: 5 · Tests only, no production code changed.

Setup:
- `EnterWorktree` refused from this session; worked by absolute path into the worktree.
- `just bootstrap` (with the brief's PATH) → exit 0.
- Reused, not redeclared: `fakeAdapter` (worker_test.go), `enqueueCount` (metrics_test.go; both are untagged `package outbox_test`, so visible to the integration-tagged file). `alwaysEntitled` was not needed (BatchEnqueuer has no entitlement check).
- Second org for the RLS case inserted via `db.AdminPool` (CreateTestAccount hardcodes identifier `test-org`).
- TDD note: the code under test already exists (MR 05), so each test was written against it and run; no red phase was possible without changing production code.

### Tasks 1-5: fixtures, happy path + replay, injected failure, concurrency, RLS — ✅ Complete
File: `backend/internal/notification/outbox/batch_integration_test.go` (5 tests + `batchFixture` helper, `failingNthInserter`)
- Happy path: 3 commands → 3 rows, each with event/recipient/asset ids, `require.JSONEq` payload, `river_job_id` = result's job id; 3 jobs; ok=3, duplicate=0.
- Replay: same batch → 3 × Duplicate with zero ids; rows 3, jobs 3; ok=3, duplicate=3.
- Failure: inserter fails on call 2 → `ErrorIs` injected; exactly 2 insert calls; 0 rows for the org (AdminPool), 0 jobs; ok=0, duplicate=0.
- Concurrency: two goroutines released by `close(start)`; no error from either; 3 queued + 3 duplicate across both; rows 3, jobs 3; ok=3, duplicate=3. No serialization/deadlock error seen (both txs insert in the same order, so the loser blocks on the unique index then takes ON CONFLICT DO NOTHING).
- RLS: `GetNotificationDeliveryByDeliveryID(orgB, id)` → "not found". Since that query also filters on `org_id`, added a predicate-free `SELECT count(*)` under `WithOrgTx` for each org: org A sees 1, org B sees 0 — RLS alone decides.

Gates:
- `flock .../trakrf_test.lock go test -tags=integration -p 1 ./internal/notification/outbox/ -run 'TestEnqueueBatch_' -count=3 -v` → all 5 integration tests PASS in each of 3 runs (15/15), `ok ... 24.688s`.
- `just lint` (backend) → `✓ check-rls-guard: clean`, fmt/vet clean.
- `go build ./...` → ok. `go test ./internal/notification/outbox/...` → ok.
- `flock ... go test -tags=integration -p 1 ./internal/notification/outbox/...` → `ok ... 22.694s`; 26 PASS lines, 0 FAIL.
- `flock .../validate.lock just validate` → exit=0: 69 Go packages ok; frontend 2605 passed / 33 skipped, 0 failed; script tests 99 + 37 + 21 (1 skipped) + 43 passed, 0 failed; CLI 25 passed. `git status` afterwards: only the new test file.

## Summary
Completed: 5 · Failed: 0 · No bug found in batch.go.
