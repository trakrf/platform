# Implementation Plan: Provider-neutral email contracts
Generated: 2026-09-25
Specification: spec.md

## Understanding
Define sender commands, accepted submission results, safe failure categories and callback events/consumer. Keep SDK types outside the contracts. A provider acceptance is not delivery. Callbacks carry provider/event/message identity and occurrence/receipt times; no outgoing delivery record is required.

## Decisions
- Session 1; dependencies: None. Follow the user-approved three-session boundary.
- Reuse SMS boundary conventions while keeping email and transactional email independent.
- Use the already pinned github.com/resend/resend-go/v2 v2.28.0; do not migrate transactional email.

## Relevant Files
- backend/internal/notification/email/contracts.go
- Reference: backend/internal/notification/sms/contracts.go and backend/internal/notification/twilio/.

## Architecture Impact
Backend notification email only. No production activation or live traffic. Provider-specific types stay outside the neutral contract. Later tasks introduce persistence/routes/runtime wiring.

## Task Breakdown
1. Read this spec/plan/log, prerequisite contracts and the preceding session handoff.
2. Write behavioral tests covering: Compile independent sender/consumer test doubles without Resend imports; verify safe errors and acceptance semantics.
3. Run tests and observe missing behavior, then implement the smallest change satisfying the technical requirements.
4. Run the validation commands, inspect results, review diff and update log with decisions and remaining work.
5. At task 3/6 boundaries, record branch/worktree, commit, completed criteria, exact results, interfaces, blockers and next-session startup prompt; stop.

## Validation Commands
```sh
just backend test ./internal/notification/email
```
Expected: exit 0; no real provider traffic. Record any environmental blockers explicitly.
