# Implementation Plan: Notification email operations and acceptance evidence
Generated: 2026-09-25
Specification: spec.md

## Understanding
Document configuration, domain verification, webhook setup, failure/ambiguity handling and operational checks. Exercise submission to signed callback to persistence and duplicate callback without real sends. Record evidence for all parent acceptance criteria.

## Decisions
- Session 3; dependencies: 7. Follow the user-approved three-session boundary.
- Reuse SMS boundary conventions while keeping email and transactional email independent.
- Use the already pinned github.com/resend/resend-go/v2 v2.28.0; do not migrate transactional email.

## Relevant Files
- docs/operations/resend-application-integration.md; backend/internal/notification/resend/integration_test.go
- Reference: backend/internal/notification/sms/contracts.go and backend/internal/notification/twilio/.

## Architecture Impact
Backend notification email only. No production activation or live traffic. Provider-specific types stay outside the neutral contract. Later tasks introduce persistence/routes/runtime wiring.

## Task Breakdown
1. Read this spec/plan/log, prerequisite contracts and the preceding session handoff.
2. Write behavioral tests covering: Run repository-required validation and integrated fake-provider flow; record exact commands/results and environmental limitations.
3. Run tests and observe missing behavior, then implement the smallest change satisfying the technical requirements.
4. Run the validation commands, inspect results, review diff and update log with decisions and remaining work.
5. At task 3/6 boundaries, record branch/worktree, commit, completed criteria, exact results, interfaces, blockers and next-session startup prompt; stop.

## Validation Commands
```sh
just validate; just backend test-integration ./internal/storage -run EmailCallback
```
Expected: exit 0; no real provider traffic. Record any environmental blockers explicitly.
