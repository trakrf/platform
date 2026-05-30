# Feature: Comment org_users.deleted_at as reserved/unused (TRA-890)

## Metadata
**Workspace**: database
**Type**: docs (schema self-documentation; no behavior change)

## Outcome
The `org_users.deleted_at` column carries a Postgres `COMMENT` declaring it intentionally reserved/unused, so the schema stops implying a soft-delete capability the code does not honor.

## User Story
As an engineer reading the trakrf schema
I want `org_users.deleted_at` to explicitly state that membership removal is a hard delete
So that I don't assume a soft-delete contract exists and write reads that depend on a `deleted_at IS NULL` filter that nothing maintains.

## Context
**Current**: `org_users` (defined in `backend/migrations/000003_organizations_and_users.up.sql:83-97`) has a nullable `deleted_at TIMESTAMPTZ` at line 93. Membership removal hard-deletes the row — `Storage.RemoveMember` runs `DELETE FROM trakrf.org_users WHERE org_id=$1 AND user_id=$2` (`backend/internal/storage/org_users.go:127-140`). The column is never written on the remove path, so the schema implies a soft-delete capability the code does not honor.
**Desired**: Keep the column (do NOT drop it) and add a `COMMENT ON COLUMN` marking it reserved/unused, placed in the consolidated cutover schema alongside the existing `COMMENT ON TABLE org_users` (line 109).
**Examples**: Comment convention is established in the same file — `COMMENT ON TABLE`/`COMMENT ON COLUMN` are unqualified (no `trakrf.` prefix), placed immediately after a table's triggers/constraints (e.g. `000003...up.sql:41-43`, `:75-78`, `:109`).

## Technical Requirements
- Add `COMMENT ON COLUMN org_users.deleted_at IS '...'` to `000003_organizations_and_users.up.sql`, directly after the existing `COMMENT ON TABLE org_users` at line 109.
- The comment must land in the **consolidated foundation stack** (edit migration `000003` in place). Do NOT add a follow-on `000011+` migration on top of the old stack. This is consistent with the up-only clean-foundation pattern; existing DBs pick it up on rebuild (re-migrating preview is acceptable).
- Keep the column. Do NOT alter `RemoveMember`, do NOT wire any soft-delete behavior. (Access audit log is a separate post-launch ticket.)
- Match existing convention: unqualified table name (`org_users`, not `trakrf.org_users`), single-quoted string, one statement.
- Comment wording (finalize in review), faithful to the ticket's suggested text:
  `Reserved/unused. Membership removal is a hard delete (see Storage.RemoveMember) for unambiguous access revocation. Membership history, if needed, will come from a separate append-only access audit log rather than soft-deleting this row.`

## Validation Criteria
- [ ] `000003_organizations_and_users.up.sql` contains exactly one new `COMMENT ON COLUMN org_users.deleted_at` statement, after the table comment.
- [ ] No new migration files added; no other migration files changed.
- [ ] `org_users.deleted_at` column definition (line 93) is unchanged; `RemoveMember` is unchanged.
- [ ] A fresh `just migrate` against an empty DB applies cleanly through 000003 (no SQL syntax error).
- [ ] After migrate, `\d+ trakrf.org_users` (or `col_description`) shows the comment on `deleted_at`.

## Success Metrics
- [ ] Migration stack applies clean end-to-end (foundation 000001–000010).
- [ ] `psql` `col_description('trakrf.org_users'::regclass, <deleted_at attnum>)` returns the comment text.
- [ ] Diff is a single added SQL line (plus the spec/plan/log artifacts) — no Go code change, no behavior change.

## References
- Ticket: https://linear.app/trakrf/issue/TRA-890
- `backend/migrations/000003_organizations_and_users.up.sql:93,109`
- `backend/internal/storage/org_users.go:127-140` (RemoveMember hard delete)
- Related decision memory: org_users is an authz table; hard delete = structurally-gone access.
