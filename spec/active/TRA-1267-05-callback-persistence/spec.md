# Feature: Durable normalized email callbacks

## Metadata
**Workspace**: backend
**Type**: feature
**Parent issue**: TRA-1267 (issue URL not supplied)
**Child issue**: Not supplied
**Dependencies**: Tasks 1
**Session**: 2

## Outcome
Persist normalized callbacks and deduplicate atomically by provider and provider event identity.

## User Story
As a notification workflow developer, I want durable normalized email callbacks so that email notifications have a testable, safe provider boundary.

## Context
**Current**: SMS integration is complete at db31d10917111ba66c3edc23040f5560ab6399a9. Transactional email lives in internal/services/email; notification email is separate.
**Desired**: Persist normalized callbacks and deduplicate atomically by provider and provider event identity. Retain provider message IDs and timestamps. Accept events without an outgoing record, including out-of-order events; preserve all distinct events.
**Examples**: internal/notification/sms and internal/notification/twilio.

## Technical Requirements
- Persist normalized callbacks and deduplicate atomically by provider and provider event identity. Retain provider message IDs and timestamps. Accept events without an outgoing record, including out-of-order events; preserve all distinct events.
- Preserve transactional email and SMS behavior.
- Exclude outgoing workers, suppression policy, recipient management, transactional-email migration, production activation and real customer sends.
- Use three separate implementation sessions: tasks 1–3, 4–6, 7–8. Stop at each boundary and record a copyable startup prompt if automatic session creation is unavailable.

## Validation Criteria
- [ ] Test concurrent duplicates, out-of-order callbacks, storage failures and uncorrelated events against persistent storage; update migration checksums.
- [ ] Exact command results recorded in log.md.

## Success Metrics
- [ ] All named behavioral tests pass without real sends.
- [ ] No regressions in relevant existing email/SMS tests.

## References
- Parent: TRA-1267 (user-provided plan; child links unavailable).
- Prerequisites: tasks 1 in adjacent TRA-1267 directories.
- [Plan](plan.md), [session log](log.md).
- Spec README/template and inventory reconciliation example read from starting revision 57443f7f; removed in SMS base revision.
