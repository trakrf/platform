# Implementation Plan: Durable normalized email callbacks
Generated: 2026-09-25
Specification: spec.md

## Understanding
Persist normalized callbacks and deduplicate atomically by provider and provider event identity. Retain provider message IDs and timestamps. Accept events without an outgoing record, including out-of-order events; preserve all distinct events.

## Decisions
- Session 2; dependencies: 1. Follow the user-approved three-session boundary.
- Reuse SMS boundary conventions while keeping email and transactional email independent.
- Use the already pinned github.com/resend/resend-go/v2 v2.28.0; do not migrate transactional email.

## Relevant Files
- backend/internal/storage/email_callbacks.go; backend/migrations/; backend/internal/notification/email/contracts.go
- Reference: backend/internal/notification/sms/contracts.go and backend/internal/notification/twilio/.

## Architecture Impact
Backend notification email only. No production activation or live traffic. Provider-specific types stay outside the neutral contract. Later tasks introduce persistence/routes/runtime wiring.

## Task Breakdown
1. Read this spec/plan/log, prerequisite contracts and the preceding session handoff.
2. Write behavioral tests covering: Test concurrent duplicates, out-of-order callbacks, storage failures and uncorrelated events against persistent storage; update migration checksums.
3. Run tests and observe missing behavior, then implement the smallest change satisfying the technical requirements.
4. Run the validation commands, inspect results, review diff and update log with decisions and remaining work.
5. At task 3/6 boundaries, record branch/worktree, commit, completed criteria, exact results, interfaces, blockers and next-session startup prompt; stop.

## Validation Commands
```sh
just backend test ./internal/storage ./internal/notification/email; just backend test-integration ./internal/storage -run EmailCallback; just backend migrate-checksums
```
Expected: exit 0; no real provider traffic. Record any environmental blockers explicitly.
