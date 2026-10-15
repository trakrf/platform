# Implementation Plan: Opt-out lookup (`IsSuppressed`)
Generated: 2026-10-09
Specification: spec.md · Parent: ../TRA-NaN-notification-service/spec.md

## ELI5
Before we message someone, we need to ask: "did this person tell us to stop?" This MR
adds that question. It ignores upper/lower case in the address, ignores opt-outs that were
later cleared, and if the database can't answer, it says "I don't know" (an error), never "go ahead".

## Files
**Create**
- `backend/internal/storage/notification_suppressions.go`
- `backend/internal/storage/notification_suppressions_integration_test.go`

**Modify**
- none

**Reference patterns**
- backend/internal/storage/notification_recipients.go: `WithOrgTx` + explicit `org_id` predicate

## Tasks
### Task 1: `IsSuppressed`
```go
// IsSuppressed reports whether address has an uncleared opt-out on channel.
// Case-insensitive, matching the unique index. A lookup error is returned and
// never read as "not suppressed": nothing may be sent while opt-out state is unknown.
func (s *Storage) IsSuppressed(ctx context.Context, orgID int, ch notificationdelivery.Channel, address string) (bool, error) {
    const q = `SELECT EXISTS (SELECT 1 FROM trakrf.notification_suppressions
        WHERE org_id = $1 AND channel = $2 AND lower(address) = lower($3) AND cleared_at IS NULL)`
    // inside s.WithOrgTx — never s.pool directly
}
```

### Task 2: Integration tests
Seed rows through `db.AdminPool` (no write API exists yet). Use a second org from
`testutil.CreateTestAccount` for the cross-org case.

## Risks
* None beyond RLS: always go through `WithOrgTx`.

## Validation gates (every task)
`just bootstrap` once in a fresh worktree (else `go:embed` targets are missing), then after each change:
`just backend lint` → `go build ./...` → `just backend test` for the touched package →
`just backend test-integration <pkg>` where integration tests changed (needs local Postgres; `just database up`).
Any failure: fix and re-run. 3 failed attempts on the same gate → stop and ask.

## Ship
PR title `feat: …` (no ticket id in published prose); body opens with the ELI5 above, then the
Validation Criteria with the exact command output. Base the PR on `feat/tra-1277-01-delivery-schema`.
