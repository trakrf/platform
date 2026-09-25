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
