# Implementation Log: Resend SDK submission adapter

## 2026-09-25 — Session planning
- Status: Ready for session 1.
- Worktree: /home/nick/.codex/worktrees/3504/platform
- Branch: feature/tra-1267-email
- Base: db31d10917111ba66c3edc23040f5560ab6399a9 (completed SMS runtime and persistence).
- Created spec/plan/log from user-provided plan. No child issue links were supplied.
- Remaining: implementation and validation described in plan.md.

## 2026-09-25 — Task 3 implementation
- Added SDK-based sender, HTTP error-status wrapper, and fake-transport tests in backend/internal/notification/resend/{sender.go,transport.go,sender_test.go}.
- `just backend test ./internal/notification/resend`: RED (sender API absent).
- `just backend test -race ./internal/notification/... ./internal/services/email`: GREEN (all 6 packages).
- SDK owns auth/serialization/success decoding; thin transport only intercepts non-2xx status and closes/discards error body because pinned v2.28.0 otherwise erases most status codes. No new dependency or SDK migration.
- User steering: prefer SDK over raw API and avoid duplicating SDK error handling. Kept SDK for sending with only application-safe classification and timeout/input handling.
- Errors: 3xx/4xx permanent except 408 and 429 transient; 408/5xx, network loss, in-flight cancellation/deadline and malformed/missing success ID have OutcomeUnknown=true. Pre-canceled context is unsubmitted. No internal retry or idempotency yet (task 4).
- Tests cover payloads for text/HTML/both, input/config rejection, disabled construction, 12 HTTP status classes with JSON/non-JSON bodies, malformed success, network errors, pre/in-flight context deadlines/cancellation, configured timeout, redirects, caller-client immutability, concurrent calls and error redaction.
- Fixed provider endpoint prevents process-global RESEND_BASE_URL from redirecting credentials; tests inject HTTP transport while retaining the real SDK.
- References inspected: pinned module source resend.go/emails.go/errors.go; https://github.com/resend/resend-go and https://resend.com/docs/api-reference/emails/send-email.
- Remaining before boundary: backend lint/test/build results, fresh-context review and final handoff. Tasks 4–8 remain intentionally unimplemented.

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

## Next session startup instructions

No automatic creation of a new root implementation session is available. Start a fresh conversation with the following prompt:

```text
Implement session 2 of TRA-1267 in /home/nick/.codex/worktrees/3504/platform on feature/tra-1267-email. Read spec/active/TRA-1267-03-resend-submission/log.md first, then the spec.md/plan.md/log.md files for tasks 04-submission-idempotency, 05-callback-persistence, and 06-resend-webhooks. Read backend/internal/notification/email/contracts.go and backend/internal/notification/resend/{config,sender,transport}.go before editing. Session 1 implementation snapshot is 3bd5d305c2829eac01a8a858c876abdf608cf648, on completed SMS base db31d10917111ba66c3edc23040f5560ab6399a9; subsequent documentation/review-fix commits are recorded in git log. Implement ONLY tasks 4–6, then update logs and stop with a fresh-session startup prompt for tasks 7–8. Use the Resend Go SDK, preserve existing transactional email and SMS behavior, and make no real sends or production activation. Run relevant tests and record exact results. Specifications and interface decisions in the task directories are durable user-requested context.
```

### Prerequisite interface decisions and cautions
- `email.Sender.SendEmail(ctx, email.Command)` returns acceptance-only `email.Submission{ProviderMessageID}`; `DeliveryID` is required, globally unique and stable across attempts. No delivery row, worker or scheduler exists yet.
- `email.ProviderError` exposes Kind, HTTPStatus and OutcomeUnknown. Unknown outcome is deliberately conservative; never infer a safe resend merely from transient errors.
- `email.CallbackConsumer.HandleEvent(ctx, email.CallbackEvent)` returns nil after durable insert or duplicate. Deduplicate on `(Provider, ProviderEventID)`, not message ID. The callback has message ID, Type, OccurredAt and ReceivedAt; no outgoing delivery reference is required. The event carries no recipient or raw body/reason.
- `resend.ConfigFromEnv()` is not yet called during backend startup; runtime construction/mounting belongs to task 7. Constructors reject disabled configuration; disabled runtime should expose no sender/routes.
- SDK v2.28.0 is already pinned; `SendWithOptions` supports idempotency. Current adapter uses `SendWithContext`; task 4 adds stable delivery-derived keys and verifies current provider window and changed-payload behavior using primary documentation.
- Current wrapper retains HTTP status but discards provider bodies. Task 4 must assess Resend's distinct HTTP 409 idempotency outcomes (changed payload vs concurrent request) before deciding whether bounded allowlisted error names are needed. Do not start retaining arbitrary provider text.
- Fixed SDK base URL is https://api.resend.com/; test via NewSenderWithHTTPClient and fake RoundTripper. Original http.Client is copied; redirects disabled. No SDK upgrade or raw API replacement was introduced.
- Configuration: NOTIFICATION_EMAIL_ENABLED=false by default; NOTIFICATION_EMAIL_FROM; RESEND_WEBHOOK_SECRET=whsec_<base64>; NOTIFICATION_EMAIL_TIMEOUT=10s default; shared RESEND_API_KEY. The key alone never enables notifications. Existing transactional client remains unchanged.
- Bootstrap/build need `PATH="$PWD/docs/notes/tra-1267/bin:$PATH"` in this worktree. On a new machine run `just bootstrap` with swag installed. New migrations require `just backend migrate-checksums`; never edit applied migrations.
- No real customer traffic, suppression policy, recipient management, transactional migration or production enablement. Storage integration tests need a test database; use the repository's integration workflow.

## 2026-09-25 — Final review and session boundary
- Task 1 commit: fff89de6; task 2 commit: 484d1ab8; task 3 commit: 3bd5d305. All implementation acceptance criteria for tasks 1–3 are complete.
- Fresh-context read-only review of db31d109..3bd5d305: no Critical or Important findings; session 1 ready. Reviewer independently ran `just backend test -race -count=1 ./internal/notification/... ./internal/services/email` (PASS) and `git diff --check db31d109..3bd5d305` (PASS).
- Final: minor (deferred): concurrent failure test in sender_test.go checks ProviderError type but could additionally assert transient/429/OutcomeUnknown=false. Serial status tests already assert these fields; implementation has no shared mutable request status. Optional test strengthening is left for later.
- Final: Ruling: reviewer set aside idempotency/conflicts (task 4), callback persistence/verification (tasks 5–6), runtime routes/startup (task 7), and operational validation (task 8) — these are required in subsequent fresh sessions, not omissions from this session — cost if ignored later: incomplete email feature, so do not activate this session's library code alone.
- Final: Ruling: reviewer set aside transactional migration/reserved-recipient policy and scheduling/suppression/recipient management — keep them excluded as explicitly requested — cost: notification workflows and policy need their separately scoped implementation. Production activation is excluded from ALL sessions, not deferred to task 8.
- Final: Ruling: reviewer accepted tracked task docs and SMS starting revision — retain the earlier user-authorized choices — cost: task-document maintenance and possible future rebase.
- Session boundary: STOPPED after task 3. All eight task directories have durable specs/plans/logs; tasks 4–8 are pending. No automatic fresh root session tool is available. Use the copyable prompt above.

## 2026-09-25 — Published for review
- Pull request: https://github.com/trakrf/platform/pull/684
- Head: `feature/tra-1267-03-resend-submission`; base: `feature/tra-1267-02-email-config`. Stacked in task dependency order.
- User requested a separate MR for each completed task, published after each batch of three. Session 1 tasks 1–3 have been pushed and opened; no merge requested.
- Pre-publication `just backend test`: PASS. Existing validation and review evidence above still applies; no implementation changes since review.

### Publishing instructions for subsequent sessions
- At each session boundary, publish one separate PR per completed task, using stacked base branches where dependencies have not yet merged. Session 2 publishes tasks 4–6; the final session publishes tasks 7–8.
- Current stack: #682 → #683 → #684. Merge in that order; retarget remaining PRs to main once their prerequisites merge. Do not merge automatically.
- Continue implementation on local `feature/tra-1267-email`; dedicated task branches preserve the individual PR boundaries. Check current remote merge state before choosing bases for later task PRs.
