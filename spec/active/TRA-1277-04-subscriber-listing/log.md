# Build Log: List an asset's switched-on subscribers

## Session: 2026-10-09
Worktree: `.claude/worktrees/tra-1277-04` · Branch: `feat/tra-1277-04-subscriber-listing` (from `origin/main` @ eb5a18af)
Starting task: 1 · Total tasks: 3

Setup:
- No `spec/stack.md`; gates are the `just` recipes in plan.md.
- `just bootstrap` (with `~/go/bin` on PATH) → "✅ Bootstrap complete"; `git status` clean afterwards.
- Integration runs are serialised under the shared `trakrf_test.lock` (one test DB name for all worktrees).

### Task 1: `Subscriber` type — ✅ Complete
File: `backend/internal/models/notificationrecipient/notificationrecipient.go`
- Added `Subscriber` as planned (no JSON tags: internal to routing, not an API shape).

### Task 3 (written first, TDD): integration tests — ✅ Complete
File: `backend/internal/storage/notification_subscribers_integration_test.go` (new; reuses `strPtr`, `boolPtr`, `createOrg` from the package's existing test files)
- Red: `go vet -tags=integration ./internal/storage/` → `db.Store.ListSubscribersForAsset undefined`.

### Task 2: `ListSubscribersForAsset` — ✅ Complete
File: `backend/internal/storage/notification_recipients.go` (300 → 333 lines)
- Query as planned, inside `WithOrgTx`, explicit `org_id` on both the WHERE and the join; empty non-nil slice when none.
- First locked run: 4 PASS, 1 FAIL (`OneRowPerChannel`). A diagnostic re-run of just that test passed.
  Cause: subscription ids come from `generate_obfuscated_id` and are not monotonic, so `ORDER BY s.id`
  is deterministic but not creation order; the test had assumed email (created first) sorts first.
  Fix (test only): assert ids ascend and match subscriptions by channel. The query keeps `ORDER BY s.id` as specified.
- After fix: `go test -tags=integration -p 1 -count=5 ./internal/storage/ -run TestSubscribersForAsset` → 25/25 PASS, `ok`.

### Gates — ✅ Complete
- `just lint` (backend) → `✓ check-rls-guard: clean`; fmt/vet clean. `go build ./...` → ok.
- `go test -tags=integration -p 1 ./internal/storage/` (locked) → exit 0, `ok ... 320.172s`; 250 PASS, 0 FAIL,
  8 SKIP (pre-existing `t.Skip` in user/org-member/NewStorage tests, untouched here).
- `just validate` (locked) → exit 0: 69 Go packages `ok`, 0 FAIL; frontend 2605 passed / 33 skipped, 0 failed;
  script tests 99 + 37 + 21 + 43 passed, 0 failed; CLI 25 passed, 0 failed.
- `git status` after validate: only the intended files.

## Summary
Total tasks: 3 · Completed: 3 · Failed: 0
Deviation: the "one recipient, two channels" test checks ids ascend and matches rows by channel, because
obfuscated ids make `ORDER BY s.id` differ from creation order.
Ready for /check: YES
