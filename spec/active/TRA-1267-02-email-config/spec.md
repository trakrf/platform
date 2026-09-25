# Feature: Explicit notification email configuration

## Metadata
**Workspace**: backend
**Type**: feature
**Parent issue**: TRA-1267 (issue URL not supplied)
**Child issue**: Not supplied
**Dependencies**: Tasks 1
**Session**: 1

## Outcome
Add explicit enablement, sender, webhook secret and positive timeout configuration alongside the existing RESEND_API_KEY.

## User Story
As a notification workflow developer, I want explicit notification email configuration so that email notifications have a testable, safe provider boundary.

## Context
**Current**: SMS integration is complete at db31d10917111ba66c3edc23040f5560ab6399a9. Transactional email lives in internal/services/email; notification email is separate.
**Desired**: Add explicit enablement, sender, webhook secret and positive timeout configuration alongside the existing RESEND_API_KEY. Disabled mode requires no credentials and never activates from the key alone. Enabled incomplete/invalid configuration fails safely. Preserve transactional email configuration.
**Examples**: internal/notification/sms and internal/notification/twilio.

## Technical Requirements
- Add explicit enablement, sender, webhook secret and positive timeout configuration alongside the existing RESEND_API_KEY. Disabled mode requires no credentials and never activates from the key alone. Enabled incomplete/invalid configuration fails safely. Preserve transactional email configuration.
- Preserve transactional email and SMS behavior.
- Exclude outgoing workers, suppression policy, recipient management, transactional-email migration, production activation and real customer sends.
- Use three separate implementation sessions: tasks 1–3, 4–6, 7–8. Stop at each boundary and record a copyable startup prompt if automatic session creation is unavailable.

## Validation Criteria
- [ ] Test disabled and enabled environment combinations, malformed values, safe errors and transactional compatibility.
- [ ] Exact command results recorded in log.md.

## Success Metrics
- [ ] All named behavioral tests pass without real sends.
- [ ] No regressions in relevant existing email/SMS tests.

## References
- Parent: TRA-1267 (user-provided plan; child links unavailable).
- Prerequisites: tasks 1 in adjacent TRA-1267 directories.
- [Plan](plan.md), [session log](log.md).
- Spec README/template and inventory reconciliation example read from starting revision 57443f7f; removed in SMS base revision.
