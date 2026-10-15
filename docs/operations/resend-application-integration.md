# Notification email application integration

The backend exposes `notification.EmailRuntime.Sender` as the provider-neutral
`email.Sender`. It uses the pinned Resend Go SDK v2.28.0 and mounts a verified
callback receiver with `Storage.EmailCallbackConsumer()`. Construction makes no
provider request. A successful submission returns a provider message ID and means
**accepted**, not delivered. Only callbacks report delivery outcomes.

## Configuration and startup

Notification email is disabled by default, including when `RESEND_API_KEY` is
already configured for transactional email. Settings are server-side only.

| Variable | Behavior |
| --- | --- |
| `NOTIFICATION_EMAIL_ENABLED` | Unset or `false`: no sender, callback route or email metrics. Set `true` explicitly to enable. Invalid boolean fails startup. |
| `RESEND_API_KEY` | Required when enabled; existing transactional-email key remains supported. |
| `NOTIFICATION_EMAIL_FROM` | Required single mailbox, optionally with display name, on a verified sending domain. |
| `RESEND_WEBHOOK_SECRET` | Required endpoint signing secret: `whsec_` followed by base64. Separate from the API key. |
| `NOTIFICATION_EMAIL_TIMEOUT` | Positive Go duration (for example `10s`); default `10s`. Bounds submission with the caller's earlier deadline taking precedence. |

The safe local shape is in [`.env.local.example`](../../.env.local.example).
Disabled mode ignores inactive provider settings. Enabled missing or malformed
configuration fails before storage startup, with errors that omit values.
Validation checks syntax; it cannot prove provider credentials or domain readiness.
The SDK API target is fixed to `https://api.resend.com`; notification submissions
do not honor `RESEND_BASE_URL`. Redirects and automatic retries are disabled.

Apply the existing `000042_email_callback_events` migration before enabling
callbacks. Use the migration role; the server uses the restricted application
role and does not apply migrations in its `serve` subcommand. Check `/health` for
`schema.applied == schema.expected` and `database: "connected"` before rollout.

Existing transactional email under `internal/services/email` retains its sender
configuration and behavior. SMS retains its separate configuration and routes.
No outgoing worker, recipient management, suppression policy, or transactional
migration is included. Enabling this boundary does not generate messages.

## Sender domain and credentials

The Resend account owner adds a domain they control (a dedicated notification
subdomain is supported), publishes the exact DKIM/SPF records shown by Resend,
and waits for the domain to verify. Configure DMARC with the domain owner and
set `NOTIFICATION_EMAIL_FROM` on that verified domain. Each subdomain requires
its own verification. Follow [Resend's domain setup](https://resend.com/docs/add-a-domain)
for the current DNS records; do not copy records from another environment.

The account owner provisions an API key permitted to send from that domain.
Platform owners inject credentials through the environment's secret mechanism,
or ignored `.env.local` for authorized local use. Coordinate rotation of the
shared `RESEND_API_KEY` with transactional email owners. Do not put credentials
in frontend variables, source control, logs, or review comments.

## Webhook setup and authentication

In the intended Resend account, create a webhook for the externally reachable
HTTPS URL:

```text
https://api.example.com/api/v1/notifications/resend/events
```

Select `email.delivered`, `email.failed`, `email.bounced`, and `email.complained`.
Copy that endpoint's signing secret to `RESEND_WEBHOOK_SECRET`. See
[webhook setup](https://resend.com/docs/webhooks/create-webhook) and
[signature verification](https://resend.com/docs/webhooks/verify-webhooks-requests).
The endpoint is account-scoped; it can receive events for transactional or other
messages from that account, including messages without a local outgoing record.

The route accepts POST without a user session. The proxy must preserve the raw
body and `svix-id`, `svix-timestamp`, and `svix-signature` headers. The handler
limits bodies to 256 KiB and verifies raw bytes before JSON normalization using
the pinned SDK. Its signature timestamp tolerance is five minutes in either
direction; keep server clocks synchronized. Only `v1` signature entries are
accepted, including multiple entries during rotation. One signing secret is
configured per process; coordinate restart and the provider's rotation overlap.

| Response | Meaning and operational action |
| --- | --- |
| 204 | Supported event durably inserted or already present; authenticated unsupported types are ignored. |
| 400 | Unreadable/malformed payload, or supported event missing message ID or occurrence time. Inspect provider schema without logging raw payloads. |
| 403 | Signature/header/timestamp verification failed. Check endpoint secret, clock, and proxy byte preservation. |
| 404 | Notification email is disabled, or the URL/path is incorrect. |
| 405 | Use POST; `Allow: POST` is returned. |
| 413 | Body exceeds 256 KiB. Inspect provider event size. |
| 503 | Durable storage failed. Repair database connectivity/permissions/contention and replay. |

The receiver uses 204 as its successful 2xx acknowledgment. Svix documents
[2xx acknowledgment semantics](https://www.svix.com/guides/receiving/receive-webhooks-with-svix-cli/).
Resend retries failed deliveries with bounded backoff and supports dashboard
replays of both failed and successful messages. Check the current
[retry and replay policy](https://resend.com/docs/webhooks/retries-and-replays)
and the endpoint's delivery history; a persistently failing endpoint may be
automatically disabled and require re-enabling after repair. Do not rely on
unlimited retries or replay a captured stale signature by hand.

## Persistence and event interpretation

`trakrf.email_callback_events` is a platform inbox with a unique key on
`(provider, provider_event_id)`. The authenticated `svix-id` is the event ID;
`data.email_id` is the provider message ID. They are distinct identities.
Records preserve event type, provider occurrence time, first local receipt time,
and insertion time. The first committed receipt wins, including concurrent
replays across replicas. Separate events for the same message remain separate,
even when received out of order. No outgoing foreign key is required.

The handler waits for an autocommitted insert before acknowledgment; writes have
a five-second deadline. Lost acknowledgments are safe to replay. Unsupported
signed types are acknowledged without persistence. Recipient addresses, subject,
body, arbitrary failure reasons, and raw webhook payloads are discarded.

This inbox is not exposed through a tenant API. Restrict operator access and
retain events until downstream processing/retention ownership is established.
Future consumers must resolve message ownership before tenant actions. A bounce
or complaint is recorded here; it does not automatically change suppression or
recipient policy. Occurrence time, rather than receipt order, describes the
provider timeline. Missing callbacks never prove successful delivery.

## Submission failures, ambiguity, and idempotency

Commands require a globally unique, stable `DeliveryID`, one bare recipient,
a nonempty subject without line breaks, and text or HTML. The adapter makes one
SDK call. It propagates cancellation/deadlines and never schedules a retry.

| Failure | Interpretation |
| --- | --- |
| `invalid` | Local validation failed; no request made. |
| `canceled` / `timeout` | Before submission, no request made; after submission starts, `OutcomeUnknown=true`. |
| `transient` | 429 is a rejected attempt; 408, 5xx and network failure may already have been accepted. |
| `permanent` | Other HTTP failures require correction; examine `OutcomeUnknown` for conflicts. |
| `unknown` | Missing/malformed success or unclassified error may already have been accepted. |

The actual SDK header is `notification-email/v1/` plus the lowercase hexadecimal
SHA-256 of the exact `DeliveryID`. It is independent of payload and attempt.
Resend retains idempotency keys for **24 hours**; this is not permanent exactly-once
delivery. An authorized caller retry must retain the same delivery identity and
payload, including configured From. Do not change the key to evade a conflict,
or blindly retry an ambiguous result after the retention window.

For HTTP 409, `invalid_idempotent_request` is permanent for the changed request;
`concurrent_idempotent_requests` is transient with an unknown original outcome.
Other/malformed conflicts are permanent with an unknown outcome. Reconcile
ambiguity with provider history and any callbacks before deciding on a new
attempt. See [Resend idempotency behavior](https://resend.com/docs/dashboard/emails/idempotency-keys).

## Monitoring and checks

Startup logs report only the enabled boolean. Debug boundary logs use bounded
outcomes; request logging redacts all `svix-*` headers. No boundary log or metric
contains recipient, message/delivery/event ID, body, signature, or raw error text.
The process exposes these families at `/metrics` when enabled:

| Metric | Labels and interpretation |
| --- | --- |
| `trakrf_resend_submissions_total` | `result`: accepted, invalid, disabled, permanent, transient, canceled, timeout, unknown; `outcome_unknown`: true/false. Acceptance is not delivery. |
| `trakrf_resend_callbacks_total` | `result`: persisted, ignored, invalid_signature, malformed, too_large, method_not_allowed, consumer_failure, unknown. Persisted includes duplicates; it is not a unique-delivery count. |
| `trakrf_resend_request_duration_seconds` | Submission duration histogram, including local rejection; no labels. |

Counter series appear after their first observation. Alert on consumer failures,
signature failures, ambiguous submissions, and unexpected callback silence;
compare with provider delivery history. After repair, replay a known event and
verify one stored row for its provider/event identity and unchanged first receipt.
An operator can inspect aggregate event counts without exposing recipient data:

```sql
SELECT provider, event_type, count(*) AS events, max(received_at) AS last_receipt
FROM trakrf.email_callback_events
GROUP BY provider, event_type;
```

Offline validation uses fake provider transports and signed fixtures with real
PostgreSQL; it sends no email. From the repository root, bootstrap first, then:

```sh
just backend test -race ./internal/notification/... ./internal/handlers/resendemail ./internal/cmd/serve ./internal/services/email ./internal/handlers/twiliosms
just backend test-integration -race ./internal/storage ./internal/handlers/resendemail ./internal/cmd/serve -run "'EmailCallback|WebhookDurable|EmailIntegration|SMSIntegration|SMSCallbacks'"
just validate
```

The integration harness recreates `trakrf_test` and requires a superuser
`PG_ADMIN_URL`; it exercises writes through the restricted test application role.
Run it serially with other users of that test database. For binary smoke checks,
use a free `BACKEND_PORT` and an isolated migrated database; the existing backend
smoke recipe assumes port 8080 and the local development database.

Setting `NOTIFICATION_EMAIL_ENABLED=false` and restarting removes both the sender
and callback route. Existing journal rows remain. Before disabling an active
integration, account for outstanding callbacks and arrange provider replay after
re-enabling; a 404 does not durably hand off an event. Preserve the journal during
application rollback. There is no independent outgoing-worker switch here.

Platform owners maintain configuration, migrations, callback availability, and
monitoring. Resend/domain owners maintain credentials, DNS and webhook setup.
Release owners separately authorize live reachability/delivery checks and rollout.
Offline tests do not establish real account readiness, DNS verification, deployed
proxy behavior, or inbox delivery. Production activation and customer sends remain
separate operational work.
