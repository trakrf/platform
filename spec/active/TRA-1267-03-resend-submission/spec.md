# Feature: Resend SDK submission adapter

## Metadata
**Workspace**: backend
**Type**: feature
**Parent issue**: TRA-1267 (issue URL not supplied)
**Child issue**: Not supplied
**Dependencies**: Tasks 1, 2
**Session**: 1

## Outcome
Use the pinned Resend Go SDK for submissions; validate delivery ID, single recipient, subject and text/HTML; propagate context and bounded timeout; return provider message ID only on acceptance.

## User Story
As a notification workflow developer, I want resend sdk submission adapter so that email notifications have a testable, safe provider boundary.

## Context
**Current**: SMS integration is complete at db31d10917111ba66c3edc23040f5560ab6399a9. Transactional email lives in internal/services/email; notification email is separate.
**Desired**: Use the pinned Resend Go SDK for submissions; validate delivery ID, single recipient, subject and text/HTML; propagate context and bounded timeout; return provider message ID only on acceptance. Classify failures without retaining response text, addresses, bodies or secrets. Do not retry.
**Examples**: internal/notification/sms and internal/notification/twilio.

## Technical Requirements
- Use the pinned Resend Go SDK for submissions; validate delivery ID, single recipient, subject and text/HTML; propagate context and bounded timeout; return provider message ID only on acceptance. Classify failures without retaining response text, addresses, bodies or secrets. Do not retry.
- Preserve transactional email and SMS behavior.
- Exclude outgoing workers, suppression policy, recipient management, transactional-email migration, production activation and real customer sends.
- Use three separate implementation sessions: tasks 1–3, 4–6, 7–8. Stop at each boundary and record a copyable startup prompt if automatic session creation is unavailable.

## Validation Criteria
- [x] Fake transport exercises payload mapping, success, malformed success, HTTP failures, network ambiguity, cancellation, timeouts, disabled mode and concurrent isolation without real sends.
- [x] Exact command results recorded in log.md.

## Success Metrics
- [x] All named behavioral tests pass without real sends.
- [x] No regressions in relevant existing email/SMS tests.

## References
- Task pull request: https://github.com/trakrf/platform/pull/684
- Parent: TRA-1267 (user-provided plan; child links unavailable).
- Prerequisites: tasks 1, 2 in adjacent TRA-1267 directories.
- [Plan](plan.md), [session log](log.md).
- Spec README/template and inventory reconciliation example read from starting revision 57443f7f; removed in SMS base revision.
