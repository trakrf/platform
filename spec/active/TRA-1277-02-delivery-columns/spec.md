# Persist routing context on delivery rows

## Metadata
**Workspace**: backend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-1277
**Parent spec**: [../TRA-NaN-notification-service/spec.md](../TRA-NaN-notification-service/spec.md) (Phase 1, MR 02 of 06)
**Depends on**: MR 01
**Branch**: `feat/tra-1277-02-delivery-columns` from `feat/tra-1277-01-delivery-schema`

## ELI5 (open the PR description with this)
The new boxes from MR 01 can now be filled in. When we save a "message to send", we also
save who it's for, which asset and event it's about, and the message text. If the same message
is saved twice, the database politely says "I already have that one" instead of making a copy
or crashing.

## Outcome
`InsertNotificationDeliveryIfAbsentTx` writes the new columns with `ON CONFLICT DO NOTHING`; `outbox.Command` and the delivery model carry the fields; both getters read them back.

## Validation Criteria
- [x] Round-trip: every new field reads back equal through both getters (payload compared with `require.JSONEq`)
- [x] Second insert with the same `delivery_id` → `inserted=false`, id 0, row unchanged
- [x] nil (and empty) payload reads back nil; unset event/recipient/asset read back nil
- [x] Existing outbox and worker tests still pass unchanged (integration run of `./internal/notification/outbox/`)
- [x] `just backend lint` and `just backend test` pass (and `just validate` exits 0)

## Out of scope
Everything not listed above; see the parent spec's MR table. No caller is wired in this MR.

## References
- [Plan](plan.md)
