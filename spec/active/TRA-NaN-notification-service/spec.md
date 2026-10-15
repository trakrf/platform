# Notification service: eligibility and routing

## Metadata
**Workspace**: backend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-1277
**Parent**: TRA-1044
**Related**: TRA-1271 (rules), TRA-1192 (outbox, done), TRA-1274 (outbox follow-ups), TRA-1275 (subscriptions, done), TRA-1278 (suppressions)

## Outcome

A notification service receives an already-detected trigger event, finds every
recipient subscribed to the asset, decides which of them are eligible, renders
one message per eligible recipient × channel, and writes those deliveries to
the `notification_deliveries` outbox and the River queue in one transaction.

It sits between detection and delivery and owns only "who is told, and how".
It does not detect events, call providers, or retry sends.

```
Geofence Detection ──TriggerEvent──▶ routing.Service.Notify(ctx, ev)
  1. validate event
  2. entitlement check (org)
  3. load subscriptions + recipients for the asset
  4. eligibility: active, not deleted, not suppressed, has address for channel
  5. render DeliveryMessage per survivor (YAML templates)
  6. ONE tx: outbox rows + River jobs (idempotent)
  7. metrics + one summary log line
                                              │
                                              ▼
                           River worker → ChannelAdapter → Resend / Twilio  (out of scope)
```

## User Story

As an **operator**, I subscribe a contact (or myself, via "subscribe me") to an
asset with a chosen channel, so that when that asset triggers a notification the
contact receives an email or SMS.

As an **asset owner** subscribed to an asset, I receive one message per
triggered event on the channel I was subscribed with, and nothing once I am
unsubscribed, deactivated, or opted out.

As an **engineer**, I can tell from metrics and logs, for any event, exactly
which recipients were enqueued or skipped and why, without the logs containing
anyone's address or message text.

## Context

What exists on `main`:

| Piece | State | Where |
| -- | -- | -- |
| Recipients + asset subscriptions (channel per subscription, email default) | Done | migration 000045, `storage/notification_recipients.go`, `handlers/notificationrecipients` |
| Outbox audit table + River queue + worker + retry ladder | Done, not started in `serve.go` | migration 000044, `notification/outbox` |
| Resend email and Twilio SMS senders + signed callbacks | Done | `notification/resend`, `notification/twilio` |
| Asset event seam | `AssetMoved` only, webhook sink only | `assetevent/event.go` |

Gaps this spec closes:

* `notification_deliveries` records no recipient, asset, event, or message;
  `outbox.Command.Payload` is never persisted, so a worker cannot know where to
  send or what to say.
* `outbox.Enqueue` takes one command in its own transaction; routing needs N
  deliveries for one event, atomically and idempotently.
* No eligibility logic, message rendering, or opt-out check exists.

### Decisions (agreed 2026-10-09)

1. **Boundary.** Triggering and its durable handoff belong to the detection
   side. `Notify` takes an already-detected `TriggerEvent`.
2. **Subscription is the opt-in.** A subscription row exists only after someone
   clicks Subscribe on the asset. Unsubscribing switches it off
   (`asset_notification_recipients.is_active = false`, migration 000047); rows
   are never deleted. Routing reads only switched-on subscriptions, so a
   switched-off one is indistinguishable from none. `recipient.is_active = false`
   pauses all of a recipient's alerts.
3. **Recipients only.** Delivery targets are always `notification_recipients`.
   The UI will offer both "subscribe a contact" and "subscribe me" (creates or
   reuses a contact for the logged-in user). Routing does not change for either.
4. **No event-type filter in v1.** A subscription receives every event type
   sent to the service for its asset.
5. **No per-contact rate cap here.** Repeated-scan dedup is detection's job.
   Suppressing a real exit is the worst failure for this feature.
6. **Message snapshot on the outbox row.** The rendered `DeliveryMessage` is
   stored in `notification_deliveries.payload`. River job args stay
   `{org_id, notification_delivery_id}`. River cleans up jobs; the audit row is
   retained three months and is the record of what was sent.
7. **Templates in YAML.** Embedded `templates.yaml` keyed
   `<event_type>.<channel>`, with a required `generic.<channel>` fallback.
   Editing wording means editing YAML, no Go change. Validated at boot.
8. **Idempotent.** `delivery_id = hex(sha256(event_id|recipient_id|channel))`,
   unique per org. A duplicate insert is skipped, not an error.
9. **Atomic per event.** All of an event's deliveries commit together or not at all.
10. **Opt-outs.** Eligibility consults `SuppressionChecker` backed by
    `notification_suppressions`. Populating it from callbacks is TRA-1278.

### Delivery phases

* **Phase 1 — delivery schema and batch outbox** (this plan): migration 000048,
  delivery model/storage columns, suppression storage, `ListSubscribersForAsset`,
  `outbox.EnqueueBatchTx` + `BatchEnqueuer`. Nothing calls it yet.
* **Phase 2 — routing service**: the `routing/` package, templates, `Notify`,
  routing metrics/logs, and construction in `serve.go`.

## Technical Requirements

### Package layout

```
backend/internal/notification/routing/
├── service.go       Service, NewService, Notify
├── trigger.go       TriggerEvent + Validate
├── eligibility.go   pure: []SubscriberRow + suppression → survivors + skips
├── message.go       DeliveryMessage + DeliveryID
├── render.go        Renderer (text/template over templates.yaml)
├── templates.yaml   go:embed
├── outcome.go       Outcome enum, Result
└── metrics.go       Prometheus counters
```

Each file stays under 500 lines. Each dependency is a narrow interface defined
in this package so unit tests use fakes.

### Input contract

```go
type TriggerEvent struct {
    EventID    string    // stable per detected event; idempotency key
    EventType  string    // e.g. "asset.boundary", "asset.moved"
    OrgID      int
    OccurredAt time.Time
    Asset      assetevent.Asset     // ID, ExternalKey, Name
    Location   *assetevent.Location // where it was seen; nil if unknown
    Direction  string               // "in" | "out" | "unknown"
}
```

`Validate` rejects an empty `EventID` or `EventType`, a non-positive `OrgID` or
`Asset.ID`, a zero `OccurredAt`, and a `Direction` outside the three values
(empty is normalized to `unknown`).

### Output contract

```go
type DeliveryMessage struct {
    EventID     string                      `json:"event_id"`
    EventType   string                      `json:"event_type"`
    Channel     notificationdelivery.Channel `json:"channel"`
    RecipientID int                         `json:"recipient_id"`
    To          string                      `json:"to"`      // email or E.164 phone
    Subject     string                      `json:"subject,omitempty"` // email only
    Body        string                      `json:"body"`
    AssetID     int                         `json:"asset_id"`
    OccurredAt  time.Time                   `json:"occurred_at"`
}

type Result struct {
    EventID  string
    Outcomes []RecipientOutcome // one per subscription considered
}
```

### Dependencies

```go
type SubscriptionReader interface {
    // One org-scoped query: subscriptions for the asset joined with the
    // recipient's name, email, phone, is_active, deleted_at.
    ListSubscribersForAsset(ctx context.Context, orgID, assetID int) ([]SubscriberRow, error)
}
type SuppressionChecker interface {
    IsSuppressed(ctx context.Context, orgID int, ch notificationdelivery.Channel, address string) (bool, error)
}
type Enqueuer interface {
    EnqueueBatch(ctx context.Context, orgID int, cmds []outbox.Command) ([]outbox.BatchResult, error)
}
// EntitlementChecker: reuse outbox.EntitlementChecker.
```

### Eligibility rules (in order; first match wins)

| Check | Outcome |
| -- | -- |
| Recipient `deleted_at` set | `skipped_inactive` |
| Recipient `is_active = false` | `skipped_inactive` |
| No address for the subscription's channel | `skipped_no_address` |
| `SuppressionChecker` says suppressed | `skipped_opted_out` |
| Otherwise | eligible → render → enqueue |

### Templates

```yaml
# templates.yaml — keys are <event_type>.<channel>; generic.* is the required fallback.
generic:
  sms:
    body: 'TrakRF: "{{.AssetName}}" ({{.AssetKey}}) was seen at {{.LocationName}} on {{.OccurredAt}} UTC.'
  email:
    subject: "Asset activity: {{.AssetName}}"
    body: |
      {{.AssetName}} ({{.AssetKey}}) was seen at {{.LocationName}}.
      Time: {{.OccurredAt}} UTC
      {{if .DirectionKnown}}Direction: {{.Direction}}{{end}}
```

* Wording states what was observed ("seen at"), never "left", because a
  single-reader gate cannot tell in from out. `Direction` is shown only when
  known. Final wording is TRA-1271 Q6.
* Times are UTC: orgs have no timezone column today.
* Missing location renders as "an unknown location".
* Boot fails if the YAML does not parse, a template does not parse, the
  `generic.sms` / `generic.email` entries are missing, or a test render with
  sample data fails.

### Schema (migration 000048, then `just backend migrate-checksums`)

```sql
ALTER TABLE notification_deliveries
  ADD COLUMN event_id     TEXT,
  ADD COLUMN recipient_id BIGINT REFERENCES notification_recipients(id),
  ADD COLUMN asset_id     BIGINT REFERENCES assets(id),
  ADD COLUMN payload      JSONB;
-- indexes on (org_id, event_id), (org_id, recipient_id), (org_id, asset_id)

CREATE TABLE notification_suppressions (
  id BIGINT PRIMARY KEY,               -- generate_obfuscated_id trigger
  org_id BIGINT NOT NULL REFERENCES organizations(id),
  channel TEXT NOT NULL CHECK (channel IN ('email','sms')),
  address TEXT NOT NULL,
  reason TEXT NOT NULL,                -- 'sms_stop' | 'email_bounce' | 'email_complaint' | 'manual'
  source_event_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  cleared_at TIMESTAMPTZ
);
-- unique (org_id, channel, lower(address)) WHERE cleared_at IS NULL; RLS on org_id
```

New delivery columns are nullable so existing rows remain valid.

### Outbox change

`outbox.EnqueueBatchTx(ctx, tx, []Command) ([]BatchResult, error)`:

* Caller-joined: runs in the transaction it is given.
* Per command: `INSERT ... ON CONFLICT (org_id, delivery_id) DO NOTHING RETURNING id`.
  No row returned → `duplicate`, no River job. Otherwise `riverClient.InsertTx`
  and set `river_job_id`.
* Persists `Command.Payload` into `payload`, plus `event_id`, `recipient_id`, `asset_id`.
* Enqueue metrics are recorded by the caller after commit (also fixes TRA-1274 item 4).
* Supersedes TRA-1274's single-command `EnqueueTx` item. Existing `Enqueue` stays.

The routing `Enqueuer` wraps `store.WithOrgTx` + `EnqueueBatchTx`.

### Error handling, logging, metrics

Counter `trakrf_notification_routing_outcomes_total{outcome}`, histogram
`trakrf_notification_routing_duration_seconds`.

| Outcome | Log level | `Notify` returns |
| -- | -- | -- |
| `enqueued` | info | nil |
| `skipped_unentitled` | info | nil |
| `skipped_no_subscribers` | info | nil |
| `skipped_inactive` | info | nil |
| `skipped_opted_out` | warn | nil |
| `skipped_no_address` | warn (configuration error) | nil |
| `duplicate` | debug | nil |
| `failed`: subscription read, suppression lookup, render, or enqueue error | error | wrapped error; caller retries |
| invalid event | error | `ErrInvalidEvent`; caller does not retry |

* Any `failed` rolls back the whole event; no partial enqueue.
* A suppression lookup error fails the event: never send when opt-out state is unknown.
* Every log line carries `org_id`, `event_id`, `event_type`, `asset_id`, and,
  per recipient, `recipient_id`, `channel`, `delivery_id`.
* Never logged: email address, phone number, subject, body. Errors are wrapped
  with context (`fmt.Errorf("routing: enqueue event %s: %w", ...)`) without payload.
* One summary line per event after commit: counts per outcome and duration.

## Edge Cases

* **Asset with zero subscriptions:** `skipped_no_subscribers`, nil, no transaction opened.
* **Same recipient subscribed on email and SMS:** two deliveries with distinct IDs.
* **`Notify` retried after a timeout whose commit actually succeeded:** all
  deliveries come back `duplicate`; no new River jobs.
* **Two concurrent `Notify` calls for the same event:** the unique index
  serializes them; one enqueues, the other sees `duplicate`.
* **Recipient deactivated or opted out between event and send:** routing
  honors the state at routing time. Changes after enqueue are the worker's
  concern (out of scope).
* **Subscription row for an asset in another org:** never returned; RLS plus
  explicit `org_id` predicate.
* **Org unentitled:** skipped and counted; never buffered for replay.
* **Location nil / direction unknown:** template renders neutral wording.
* **Template references a field that is empty:** rendered as the documented
  fallback text, not `<no value>`; caught by the boot-time sample render.
* **Phone stored in non-E.164 form:** passed through as stored; normalization
  is the recipient API's job (note for the operator UI ticket).

## Validation Criteria

- [ ] Migration applies and rolls back cleanly; `just backend migrate-checksums` updated.
- [ ] Eligibility table tests cover active, inactive, deleted, suppressed, missing address, and one recipient on two channels.
- [ ] Renderer tests: per-type template, generic fallback, unknown direction, nil location; boot fails on missing `generic.*` or a broken template.
- [ ] Integration: N subscribers → N `notification_deliveries` rows with payload + N River jobs, one transaction.
- [ ] Integration: `Notify` called twice → second call all `duplicate`, River job count unchanged.
- [ ] Integration: injected River insert failure → zero delivery rows, error returned.
- [ ] Integration: unentitled org → zero rows, `skipped_unentitled` counted.
- [ ] Integration: cross-org asset/recipient → nothing enqueued.
- [ ] Log capture test: no email, phone, subject or body in any log line.
- [ ] `just backend lint` and `just backend test` pass; service constructed in `serve.go` with no caller.

## Out of scope

* Boundary/gate events, decoupling detection from relay actuation, and calling `Notify` from detection (TRA-1044 item 6).
* ChannelAdapters that read `payload` and call Resend/Twilio; starting the outbox runtime in `serve.go`.
* Populating `notification_suppressions` from STOP/START and bounce/complaint callbacks (TRA-1278).
* Operator UI (subscribe / subscribe me / unsubscribe), delivery-status API.
* Per-contact rate cap, event-type subscription filters, org timezone.

## References

* `backend/internal/notification/outbox/{contracts,outbox,worker}.go`
* `backend/internal/assetevent/event.go`
* `backend/internal/storage/notification_recipients.go`
* `backend/migrations/000044_notification_deliveries.up.sql`, `000045_notification_subscriptions.up.sql`
* `backend/internal/webhook/sink.go` (existing entitlement-skip precedent)
