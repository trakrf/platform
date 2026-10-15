# Feature: Backend notification email wiring

## Metadata
**Workspace**: backend
**Type**: feature
**Parent issue**: TRA-1267 (issue URL not supplied)
**Child issue**: Not supplied
**Dependencies**: Tasks 4, 6
**Session**: 3

## Outcome
Construct/expose sender interface and mount verified callbacks with persistent consumer only when explicitly enabled.

## User Story
As a notification workflow developer, I want backend notification email wiring so that email notifications have a testable, safe provider boundary.

## Context
**Current**: SMS integration is complete at db31d10917111ba66c3edc23040f5560ab6399a9. Transactional email lives in internal/services/email; notification email is separate.
**Desired**: Construct/expose sender interface and mount verified callbacks with persistent consumer only when explicitly enabled. Add bounded safe logs/metrics. Preserve SMS and existing transactional email.
**Examples**: internal/notification/sms and internal/notification/twilio.

## Technical Requirements
- Construct/expose sender interface and mount verified callbacks with persistent consumer only when explicitly enabled. Add bounded safe logs/metrics. Preserve SMS and existing transactional email.
- Preserve transactional email and SMS behavior.
- Exclude outgoing workers, suppression policy, recipient management, transactional-email migration, production activation and real customer sends.
- Use three separate implementation sessions: tasks 1–3, 4–6, 7–8. Stop at each boundary and record a copyable startup prompt if automatic session creation is unavailable.

## Validation Criteria
- [ ] Test startup/config failures, disabled and enabled routes, persistent consumer wiring, and transactional/SMS regressions.
- [ ] Exact command results recorded in log.md.

## Success Metrics
- [ ] All named behavioral tests pass without real sends.
- [ ] No regressions in relevant existing email/SMS tests.

## References
- Parent: TRA-1267 (user-provided plan; child links unavailable).
- Prerequisites: tasks 4, 6 in adjacent TRA-1267 directories.
- [Plan](plan.md), [session log](log.md).
- Spec README/template and inventory reconciliation example read from starting revision 57443f7f; removed in SMS base revision.
