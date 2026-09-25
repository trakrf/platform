# Implementation Log: Provider-neutral email contracts

## 2026-09-25 — Session planning
- Status: Ready for session 1.
- Worktree: /home/nick/.codex/worktrees/3504/platform
- Branch: feature/tra-1267-email
- Base: db31d10917111ba66c3edc23040f5560ab6399a9 (completed SMS runtime and persistence).
- Created spec/plan/log from user-provided plan. No child issue links were supplied.
- Remaining: implementation and validation described in plan.md.

## 2026-09-25 — Task 1 complete
- Added `backend/internal/notification/email/contracts.go` and external-package contract tests.
- `just backend test ./internal/notification/email`: RED (no implementation), then GREEN (2 tests).
- Sender: `SendEmail(context.Context, Command) (Submission, error)`; command holds stable globally unique DeliveryID, one bare To, Subject, Text, HTML. Submission only contains provider message ID and means acceptance, never delivery.
- Safe error kinds include invalid, disabled, permanent, transient, canceled, timeout, unknown. OutcomeUnknown is independent of kind; original provider errors are never retained. Standard context errors remain matchable.
- Consumer: `HandleEvent(context.Context, CallbackEvent) error`; nil means durably recorded or duplicate. Provider + ProviderEventID identifies event; message ID and occurrence/receipt times retained without requiring an outgoing delivery ID.
- Ruling: user-requested spec/plan/log directories are tracked despite the SMS revision's generic AGENTS.md preference for untracked session artifacts. User explicitly requests durable per-task context; cost is documentation maintenance.
- Ruling: use existing isolated worktree on feature/tra-1267-email based on completed SMS commit db31d109, rather than the supplied old main revision. Required by the user; eventual integration may need rebasing.
- Bootstrap initially failed for missing swag; installed v1.16.6 under ignored docs/notes/tra-1267/bin and reran successfully with PATH override. No generated tracked drift.
- Preflight: tasks 2/3 share enabled config; task 3/4 share stable DeliveryID; task 5/6 consume callback identity + times; task 7 consumes sender/config/consumer. No conflicting interfaces found.
- Remaining: task 2 configuration, then task 3 adapter. Session boundary after task 3.

## 2026-09-25 — Session 1 validation snapshot
- Worktree: `/home/nick/.codex/worktrees/3504/platform`; branch: `feature/tra-1267-email`.
- Implementation snapshot: `3bd5d305c2829eac01a8a858c876abdf608cf648`; prerequisite SMS commit: `db31d10917111ba66c3edc23040f5560ab6399a9`.
- Completed criteria: provider-neutral contracts, explicit disabled-by-default config, SDK submission with validation/context/timeouts/provider IDs and safe failure categories. No runtime wiring or sends activated.
- `just backend lint`: PASS (RLS guard, go fmt, go vet).
- `just backend test`: PASS (888 top-level tests, 62 packages; 10 existing DB-dependent placeholders skipped: TestSignup, TestLogin, TestListOrgMembers, TestNewStorage, TestListUsers, TestGetUserByID, TestGetUserByEmail, TestCreateUser, TestUpdateUser, TestSoftDeleteUser).
- `just backend test -race ./internal/notification/... ./internal/services/email`: PASS (58 top-level tests, 6 packages).
- `just backend build`: initial failure because local swag was not on PATH; `PATH="$PWD/docs/notes/tra-1267/bin:$PATH" just backend build`: PASS. Swag v1.16.6 was installed locally during bootstrap.
- `git diff --check`: PASS. Existing transactional email, SMS, and serve files have no diff against the SMS base.
- Detailed output is in ignored `docs/notes/tra-1267/`; this log records durable command results.
- No blockers for session 2. Repository-wide `just validate` and live-database callback integration tests remain session 3/task 8 work; this session changed backend library code only.
- Next task: 4, submission idempotency, in a fresh implementation session. Do not continue tasks 4–6 in this context.
- Boundary review: no blocking findings; task 1 implementation is commit fff89de6. Final handoff and one deferred test-strengthening note are in ../TRA-1267-03-resend-submission/log.md.

## 2026-09-25 — Published for review
- Pull request: https://github.com/trakrf/platform/pull/682
- Head: `feature/tra-1267-01-email-contracts`; base: `main`. Stacked in task dependency order.
- User requested a separate MR for each completed task, published after each batch of three. Session 1 tasks 1–3 have been pushed and opened; no merge requested.
- Pre-publication `just backend test`: PASS. Existing validation and review evidence above still applies; no implementation changes since review.
