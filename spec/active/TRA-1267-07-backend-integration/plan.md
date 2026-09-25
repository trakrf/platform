# Implementation Plan: Backend notification email wiring
Generated: 2026-09-25
Specification: spec.md

## Understanding
Construct/expose sender interface and mount verified callbacks with persistent consumer only when explicitly enabled. Add bounded safe logs/metrics. Preserve SMS and existing transactional email.

## Decisions
- Session 3; dependencies: 4, 6. Follow the user-approved three-session boundary.
- Reuse SMS boundary conventions while keeping email and transactional email independent.
- Use the already pinned github.com/resend/resend-go/v2 v2.28.0; do not migrate transactional email.

## Relevant Files
- backend/internal/notification/runtime.go; backend/internal/cmd/serve/serve.go; backend/internal/cmd/serve/router.go
- Reference: backend/internal/notification/sms/contracts.go and backend/internal/notification/twilio/.

## Architecture Impact
Backend notification email only. No production activation or live traffic. Provider-specific types stay outside the neutral contract. Later tasks introduce persistence/routes/runtime wiring.

## Task Breakdown
1. Read this spec/plan/log, prerequisite contracts and the preceding session handoff.
2. Write behavioral tests covering: Test startup/config failures, disabled and enabled routes, persistent consumer wiring, and transactional/SMS regressions.
3. Run tests and observe missing behavior, then implement the smallest change satisfying the technical requirements.
4. Run the validation commands, inspect results, review diff and update log with decisions and remaining work.
5. At task 3/6 boundaries, record branch/worktree, commit, completed criteria, exact results, interfaces, blockers and next-session startup prompt; stop.

## Validation Commands
```sh
just backend test -race ./internal/notification/... ./internal/handlers/resendemail ./internal/cmd/serve ./internal/services/email
```
Expected: exit 0; no real provider traffic. Record any environmental blockers explicitly.
