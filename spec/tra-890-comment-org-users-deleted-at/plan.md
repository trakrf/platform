# Implementation Plan: Comment org_users.deleted_at as reserved/unused (TRA-890)
Generated: 2026-05-30
Specification: spec.md

## Understanding
Add a single Postgres `COMMENT ON COLUMN org_users.deleted_at` statement to the consolidated
foundation migration so the schema documents that membership removal is a hard delete and the
`deleted_at` column is intentionally reserved/unused. No column is dropped, no Go code changes,
no behavior change. Edit the foundation file in place (up-only clean stack) — do NOT add a
follow-on migration.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/migrations/000003_organizations_and_users.up.sql:41-43` — `COMMENT ON TABLE` + `COMMENT ON COLUMN` style: unqualified table name, single-quoted, one statement per line.
- `backend/migrations/000003_organizations_and_users.up.sql:109` — existing `COMMENT ON TABLE org_users IS '...'`; the new column comment goes immediately after this line.
- `backend/internal/storage/org_users.go:127-140` — `RemoveMember` hard-delete, referenced by the comment text (verified: `DELETE FROM trakrf.org_users`).

**Files to Create**: none.

**Files to Modify**:
- `backend/migrations/000003_organizations_and_users.up.sql` (after line 109) — add one `COMMENT ON COLUMN org_users.deleted_at` statement.

## Architecture Impact
- **Subsystems affected**: database (schema metadata only).
- **New dependencies**: none.
- **Breaking changes**: none. Existing DBs that already ran 000003 will not see the comment until rebuilt; re-migrating preview is acceptable and consistent with the up-only foundation pattern (stack.md: no migration paths required pre-launch).

## Task Breakdown

### Task 1: Add the column comment
**File**: `backend/migrations/000003_organizations_and_users.up.sql`
**Action**: MODIFY (insert after line 109)
**Pattern**: mirror the `COMMENT ON TABLE org_users` line directly above.

**Implementation**:
```sql
COMMENT ON TABLE org_users IS 'Junction table managing user membership and roles within organizations';
COMMENT ON COLUMN org_users.deleted_at IS 'Reserved/unused. Membership removal is a hard delete (see Storage.RemoveMember) for unambiguous access revocation. Membership history, if needed, will come from a separate append-only access audit log rather than soft-deleting this row.';
```

**Validation**: fresh migrate applies clean; comment present on the column (see Validation Sequence).

## Risk Assessment
- **Risk**: SQL syntax error / wrong placement breaks the whole foundation migrate.
  **Mitigation**: run a fresh `just migrate` against a clean local DB and confirm 000003 applies; verify with `col_description`.
- **Risk**: someone reads the rename-sweep memory and tries to also touch RemoveMember / drop the column.
  **Mitigation**: spec explicitly forbids both; this PR is comment-only.

## Integration Points
- None. Pure schema-comment addition.

## VALIDATION GATES (MANDATORY)
Commands from `spec/stack.md` (run from project root):
- Gate 1 — Lint: `just backend lint`
- Gate 2 — Build: `just backend build`
- Gate 3 — Migration applies clean against a fresh DB: `just backend migrate` (local TimescaleDB)
- Gate 4 — Comment is present: `psql "$PG_URL" -tAc "SELECT col_description('trakrf.org_users'::regclass, (SELECT attnum FROM pg_attribute WHERE attrelid='trakrf.org_users'::regclass AND attname='deleted_at'));"` returns the comment text.

If any gate fails → fix → re-run → repeat. Do not proceed until all pass.

## Validation Sequence
After the edit: `just backend lint` → `just backend build` → fresh `just backend migrate` → `col_description` check → `just backend test` (no Go change expected to fail).
Final: `just validate` (or at minimum backend lint+build+test) green before ship.

## Plan Quality Assessment

**Complexity Score**: 1/10 (LOW)
**Confidence Score**: 10/10 (HIGH)

**Confidence Factors**:
✅ Clear, pre-decided requirements from the ticket.
✅ Existing comment pattern found at 000003:41-43,109.
✅ Hard-delete premise verified in code (org_users.go:127-140).
✅ No open clarifying questions; no new packages; single-line change.

**Assessment**: One-line, behavior-free schema-comment addition following an established in-file convention.

**Estimated one-pass success probability**: 99%

**Reasoning**: The only real failure modes are a typo'd SQL string or misplacement, both caught by a fresh migrate + col_description check.
