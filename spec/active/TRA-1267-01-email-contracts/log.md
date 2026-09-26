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
