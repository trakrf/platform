# Implementation Plan: Verified Resend email webhooks
Generated: 2026-09-25
Specification: spec.md

## Understanding
Verify signatures over raw bytes before normalization. Normalize delivery/failure/bounce/complaint events, preserving authenticated event identity. Persist before acknowledging, safely acknowledge duplicates and unsupported events; storage failures return retryable errors.

## Decisions
- Session 2; dependencies: 2, 5. Follow the user-approved three-session boundary.
- Reuse SMS boundary conventions while keeping email and transactional email independent.
- Use the already pinned github.com/resend/resend-go/v2 v2.28.0; do not migrate transactional email.

## Relevant Files
- backend/internal/handlers/resendemail/; backend/internal/notification/email/contracts.go
- Reference: backend/internal/notification/sms/contracts.go and backend/internal/notification/twilio/.

## Architecture Impact
Backend notification email only. No production activation or live traffic. Provider-specific types stay outside the neutral contract. Later tasks introduce persistence/routes/runtime wiring.

## Task Breakdown
1. Read this spec/plan/log, prerequisite contracts and the preceding session handoff.
2. Write behavioral tests covering: Test signed, invalid, expired, malformed and unsupported fixtures, consumer failures, and duplicates without live callbacks.
3. Run tests and observe missing behavior, then implement the smallest change satisfying the technical requirements.
4. Run the validation commands, inspect results, review diff and update log with decisions and remaining work.
5. At task 3/6 boundaries, record branch/worktree, commit, completed criteria, exact results, interfaces, blockers and next-session startup prompt; stop.

## Validation Commands
```sh
just backend test -race ./internal/handlers/resendemail ./internal/notification/...
```
Expected: exit 0; no real provider traffic. Record any environmental blockers explicitly.
