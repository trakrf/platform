# Implementation Plan: Resend SDK submission adapter
Generated: 2026-09-25
Specification: spec.md

## Understanding
Use the pinned Resend Go SDK for submissions; validate delivery ID, single recipient, subject and text/HTML; propagate context and bounded timeout; return provider message ID only on acceptance. Classify failures without retaining response text, addresses, bodies or secrets. Do not retry.

## Decisions
- Session 1; dependencies: 1, 2. Follow the user-approved three-session boundary.
- Reuse SMS boundary conventions while keeping email and transactional email independent.
- Use the already pinned github.com/resend/resend-go/v2 v2.28.0; do not migrate transactional email.

## Relevant Files
- backend/internal/notification/resend/sender.go; backend/internal/notification/resend/transport.go
- Reference: backend/internal/notification/sms/contracts.go and backend/internal/notification/twilio/.

## Architecture Impact
Backend notification email only. No production activation or live traffic. Provider-specific types stay outside the neutral contract. Later tasks introduce persistence/routes/runtime wiring.

## Task Breakdown
1. Read this spec/plan/log, prerequisite contracts and the preceding session handoff.
2. Write behavioral tests covering: Fake transport exercises payload mapping, success, malformed success, HTTP failures, network ambiguity, cancellation, timeouts, disabled mode and concurrent isolation without real sends.
3. Run tests and observe missing behavior, then implement the smallest change satisfying the technical requirements.
4. Run the validation commands, inspect results, review diff and update log with decisions and remaining work.
5. At task 3/6 boundaries, record branch/worktree, commit, completed criteria, exact results, interfaces, blockers and next-session startup prompt; stop.

## Validation Commands
```sh
just backend test -race ./internal/notification/... ./internal/services/email
```
Expected: exit 0; no real provider traffic. Record any environmental blockers explicitly.
