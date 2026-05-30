# Build Log: Comment org_users.deleted_at as reserved/unused (TRA-890)

## Session: 2026-05-30
Starting task: 1
Total tasks: 1

### Task 1: Add COMMENT ON COLUMN org_users.deleted_at
Started: 2026-05-30
File: backend/migrations/000003_organizations_and_users.up.sql

Inserted one `COMMENT ON COLUMN org_users.deleted_at` statement immediately after the
existing `COMMENT ON TABLE org_users` (line 109), matching the in-file unqualified comment
convention. No column dropped, no Go code touched, no behavior change.

Status: ✅ Complete

Validation:
- `just backend lint` → clean (check-rls-guard clean, go fmt, go vet)
- `just backend build` → success (specs regenerated, no committed drift)
- Fresh migrate on throwaway DB `tra890_verify` → "Migrations complete" version=10 dirty=false
- `col_description('trakrf.org_users', deleted_at)` → returns the comment text verbatim
- `go test ./...` → all pass (exit 0); no Go change, sanity only
- `git diff --stat` → 1 file changed, 1 insertion

Note: the local dev DB (`postgres`) is in a pre-existing dirty state (schema present but
golang-migrate version tracking out of sync — `org_role already exists`). Unrelated to this
change; the fresh-DB migrate above is the authoritative gate. Re-migrating preview is the
acceptable path per the ticket.

Completed: 2026-05-30

## Summary
Total tasks: 1
Completed: 1
Failed: 0

Ready for /csw:check: YES
