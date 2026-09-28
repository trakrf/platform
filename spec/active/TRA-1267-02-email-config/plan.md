# Implementation Plan: Explicit notification email configuration
Generated: 2026-09-25
Specification: spec.md

## Understanding
Add explicit enablement, sender, webhook secret and positive timeout configuration alongside the existing RESEND_API_KEY. Disabled mode requires no credentials and never activates from the key alone. Enabled incomplete/invalid configuration fails safely. Preserve transactional email configuration.

## Decisions
- Session 1; dependencies: 1. Follow the user-approved three-session boundary.
- Reuse SMS boundary conventions while keeping email and transactional email independent.
- Use the already pinned github.com/resend/resend-go/v2 v2.28.0; do not migrate transactional email.

## Relevant Files
- backend/internal/notification/resend/config.go; .env.local.example
- Reference: backend/internal/notification/sms/contracts.go and backend/internal/notification/twilio/.

## Architecture Impact
Backend notification email only. No production activation or live traffic. Provider-specific types stay outside the neutral contract. Later tasks introduce persistence/routes/runtime wiring.

## Task Breakdown
1. Read this spec/plan/log, prerequisite contracts and the preceding session handoff.
2. Write behavioral tests covering: Test disabled and enabled environment combinations, malformed values, safe errors and transactional compatibility.
3. Run tests and observe missing behavior, then implement the smallest change satisfying the technical requirements.
4. Run the validation commands, inspect results, review diff and update log with decisions and remaining work.
5. At task 3/6 boundaries, record branch/worktree, commit, completed criteria, exact results, interfaces, blockers and next-session startup prompt; stop.

## Validation Commands
```sh
just backend test ./internal/notification/resend ./internal/services/email
```
Expected: exit 0; no real provider traffic. Record any environmental blockers explicitly.
