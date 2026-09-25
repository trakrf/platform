# Feature: Notification email operations and acceptance evidence

## Metadata
**Workspace**: backend
**Type**: feature
**Parent issue**: TRA-1267 (issue URL not supplied)
**Child issue**: Not supplied
**Dependencies**: Tasks 7
**Session**: 3

## Outcome
Document configuration, domain verification, webhook setup, failure/ambiguity handling and operational checks.

## User Story
As a notification workflow developer, I want notification email operations and acceptance evidence so that email notifications have a testable, safe provider boundary.

## Context
**Current**: SMS integration is complete at db31d10917111ba66c3edc23040f5560ab6399a9. Transactional email lives in internal/services/email; notification email is separate.
**Desired**: Document configuration, domain verification, webhook setup, failure/ambiguity handling and operational checks. Exercise submission to signed callback to persistence and duplicate callback without real sends. Record evidence for all parent acceptance criteria.
**Examples**: internal/notification/sms and internal/notification/twilio.

## Technical Requirements
- Document configuration, domain verification, webhook setup, failure/ambiguity handling and operational checks. Exercise submission to signed callback to persistence and duplicate callback without real sends. Record evidence for all parent acceptance criteria.
- Preserve transactional email and SMS behavior.
- Exclude outgoing workers, suppression policy, recipient management, transactional-email migration, production activation and real customer sends.
- Use three separate implementation sessions: tasks 1–3, 4–6, 7–8. Stop at each boundary and record a copyable startup prompt if automatic session creation is unavailable.

## Validation Criteria
- [ ] Run repository-required validation and integrated fake-provider flow; record exact commands/results and environmental limitations.
- [ ] Exact command results recorded in log.md.

## Success Metrics
- [ ] All named behavioral tests pass without real sends.
- [ ] No regressions in relevant existing email/SMS tests.

## References
- Parent: TRA-1267 (user-provided plan; child links unavailable).
- Prerequisites: tasks 7 in adjacent TRA-1267 directories.
- [Plan](plan.md), [session log](log.md).
- Spec README/template and inventory reconciliation example read from starting revision 57443f7f; removed in SMS base revision.
