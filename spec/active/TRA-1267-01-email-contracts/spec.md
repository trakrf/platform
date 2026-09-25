# Feature: Provider-neutral email contracts

## Metadata
**Workspace**: backend
**Type**: feature
**Parent issue**: TRA-1267 (issue URL not supplied)
**Child issue**: Not supplied
**Dependencies**: Tasks None
**Session**: 1

## Outcome
Define sender commands, accepted submission results, safe failure categories and callback events/consumer.

## User Story
As a notification workflow developer, I want provider-neutral email contracts so that email notifications have a testable, safe provider boundary.

## Context
**Current**: SMS integration is complete at db31d10917111ba66c3edc23040f5560ab6399a9. Transactional email lives in internal/services/email; notification email is separate.
**Desired**: Define sender commands, accepted submission results, safe failure categories and callback events/consumer. Keep SDK types outside the contracts. A provider acceptance is not delivery. Callbacks carry provider/event/message identity and occurrence/receipt times; no outgoing delivery record is required.
**Examples**: internal/notification/sms and internal/notification/twilio.

## Technical Requirements
- Define sender commands, accepted submission results, safe failure categories and callback events/consumer. Keep SDK types outside the contracts. A provider acceptance is not delivery. Callbacks carry provider/event/message identity and occurrence/receipt times; no outgoing delivery record is required.
- Preserve transactional email and SMS behavior.
- Exclude outgoing workers, suppression policy, recipient management, transactional-email migration, production activation and real customer sends.
- Use three separate implementation sessions: tasks 1–3, 4–6, 7–8. Stop at each boundary and record a copyable startup prompt if automatic session creation is unavailable.

## Validation Criteria
- [ ] Compile independent sender/consumer test doubles without Resend imports; verify safe errors and acceptance semantics.
- [ ] Exact command results recorded in log.md.

## Success Metrics
- [ ] All named behavioral tests pass without real sends.
- [ ] No regressions in relevant existing email/SMS tests.

## References
- Parent: TRA-1267 (user-provided plan; child links unavailable).
- Prerequisites: tasks None in adjacent TRA-1267 directories.
- [Plan](plan.md), [session log](log.md).
- Spec README/template and inventory reconciliation example read from starting revision 57443f7f; removed in SMS base revision.
