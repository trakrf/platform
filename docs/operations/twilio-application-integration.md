# Twilio SMS application integration

The backend composes a Twilio SMS sender and signature-verified callback routes.
With all six settings empty, the integration is disabled and the application
boots normally. Complete configuration constructs the sender and mounts both
callback routes with a PostgreSQL consumer. Construction makes no Twilio request.

Apply migrations before enabling the integration. Migration
`000041_sms_callback_events.up.sql` creates the durable callback journal.
The configured sender is available as `notification.Runtime.Sender` for the
notification worker to consume. Geofence recipient selection, preferences,
and outbox workers remain part of the parent notification feature; this
integration does not generate messages by itself.

## Configuration

The six settings below are read during backend startup. Keep every credential on the
server; none is a frontend (`VITE_*`) setting.

| Variable | Purpose | Required when enabled |
| --- | --- | --- |
| `TWILIO_ACCOUNT_SID` | Twilio Account SID used as API request context | Yes |
| `TWILIO_API_KEY_SID` | API Key SID for outbound REST authentication | Yes |
| `TWILIO_API_KEY_SECRET` | Secret paired with the API Key SID | Yes |
| `TWILIO_AUTH_TOKEN` | Auth Token used to validate Twilio callback signatures | Yes |
| `TWILIO_MESSAGING_SERVICE_SID` | Central TrakRF Messaging Service sender | Yes |
| `TWILIO_PUBLIC_BASE_URL` | Canonical HTTPS origin visible to Twilio callbacks | Yes |

The canonical local template is `.env.local.example`; `.env.example` is
intentionally absent under [ADR 0010](../adr/0010-local-configuration-mirrors-the-deployed-shape.md)
so two environment templates cannot drift. The template contains this safe
disabled, copyable shape (all six active assignments are empty):

```dotenv
TWILIO_ACCOUNT_SID=
TWILIO_API_KEY_SID=
TWILIO_API_KEY_SECRET=
TWILIO_AUTH_TOKEN=
TWILIO_MESSAGING_SERVICE_SID=
TWILIO_PUBLIC_BASE_URL=
# Example only; replace with the externally reachable canonical HTTPS origin.
# TWILIO_PUBLIC_BASE_URL=https://api.example.com
```

To enable Twilio, set all six active assignments together. Do not leave only
the URL or any other subset configured.

`TWILIO_PUBLIC_BASE_URL` must be an HTTPS origin with a host and no userinfo,
path (including a trailing slash), query, or fragment. Replace the example
origin in the commented line with the externally reachable origin when
enabling the integration; do not put credentials in the URL.

## Fail-closed behavior

- If all six values are empty, Twilio is disabled and the application can run
  without an SMS provider.
- If any value is supplied but one or more values are missing, configuration
  loading fails closed. It must not create a partially enabled sender.
- A configured but non-canonical public URL is also rejected. Errors do not
  include API-key secrets or the Auth Token.

This boundary makes configuration state explicit. It does not provide a
fallback sender, queue, or retry/outbox implementation.

## Authentication and sender selection

Outbound REST calls use the API Key SID and API Key Secret, with the Account
SID supplied as context. The Auth Token has a separate role: it is used only to
validate the `X-Twilio-Signature` on callbacks and is not the outbound API
credential.

Outbound messages use the central TrakRF Twilio Messaging Service identified by
`TWILIO_MESSAGING_SERVICE_SID`. The application supplies a destination and
message body to that service; it does not select a phone number itself. There
is deliberately no `TWILIO_FROM_NUMBER` setting and no raw `From` number in
this integration. Sender-pool choice belongs to the Messaging Service.

## Credential provisioning and readiness

No interactive setup flow prompts for an API key. Offline development and tests
use fake credentials and local transports, including tests of the real SDK,
production router, and PostgreSQL callback journal. They verify application
behavior without establishing that a real account or sender is configured.

Before a live integration test, the account owner creates an application key
in the intended Twilio account and region. Use a Standard key, or a Restricted
key with message-creation permission; this sender does not require Main access.
Follow [Twilio's Console key creation guide](https://www.twilio.com/docs/iam/api-keys/keys-in-console)
and [key types reference](https://www.twilio.com/docs/iam/api-keys).
The current adapter uses the SDK's default endpoint and exposes no region setting;
create credentials for that endpoint's region.

Store the key SID and secret in `TWILIO_API_KEY_SID` and
`TWILIO_API_KEY_SECRET`. Supply the Account SID, callback Auth Token,
Messaging Service SID, and public HTTPS origin at the same time. Use the local
ignored `.env.local` file or the deployment secret mechanism; keep secrets out
of source, frontend configuration, and review comments.

Apply the migration, configure the Messaging Service's sender pool and inbound
webhook, and deploy with the complete configuration. Callback endpoints will
then be active. The parent notification worker must use `Runtime.Sender` to
submit messages; the runtime performs no automatic test send at startup.
Only an authorized live test can verify real account authentication, sender
registration, carrier delivery, and callback reachability through deployed
network infrastructure. These are operational verification steps, not reasons
to leave the application code or offline tests unfinished.

HTTP failure classification preserves the actual response status even for empty,
malformed, or inconsistent error bodies: 429 and 5xx are transient, and recognized
provider rejection/permanent codes remain available. The adapter bounds error
body reads and closes them. It does not retry automatically; the future worker
owns retry scheduling and the treatment of ambiguous submission outcomes.

## Outbound cancellation boundary

`SendSMS` honors a context that is already canceled or past its deadline before
submission: it returns without making an SDK request. Once `CreateMessage` has
started, Twilio Go v1.30.9 does not propagate cancellation or deadline changes
through its `CreateMessage` path; the SDK constructs that request from a
background context. Callers must therefore not assume that a cancellation or
deadline change interrupts an in-flight Twilio request. A context-aware SDK
transport or API path is future work if shorter in-flight deadlines become a
requirement.

## Callback paths

With complete configuration, `serve.setupRouter` mounts these form-encoded POST
endpoints outside session authentication and JSON-only middleware. Configure
Twilio to use these paths appended to the public base URL:

| Callback | Path |
| --- | --- |
| Delivery status | `/api/v1/notifications/twilio/status` |
| Inbound keyword | `/api/v1/notifications/twilio/inbound` |

Use this inbound webhook URL, substituting the configured public origin:

```text
https://api.example.com/api/v1/notifications/twilio/inbound#rc=3&rp=ct,rt,5xx
```

The sender appends the same retry fragment to its outbound `StatusCallback` URL.
This requests up to three retries for connection failures, read timeouts, and
HTTP 5xx responses, within Twilio's total time budget. The default retry policy
does not cover HTTP 5xx. Account webhook rules can override URL settings; ensure
any matching rule preserves equivalent retries. The fragment is consumed by
Twilio and is omitted from the callback request and signature computation.
See [Twilio connection overrides](https://www.twilio.com/docs/usage/webhooks/webhooks-connection-overrides).

Retries are bounded and cannot guarantee recovery from an extended outage.
Monitor `trakrf_twilio_callbacks_total{result="consumer_failure"}` and the Twilio
Debugger, and reconcile missing delivery outcomes before treating them as final.

The callback boundary validates the Twilio signature against the
externally visible URL (including the path and query, when present) before
emitting a normalized event to an injected consumer. The endpoints do not
require a TrakRF user session. With no configuration, both paths return 404.

The production consumer commits normalized events to `trakrf.sms_callback_events`
before the handler returns 204. Storage failure returns 500; persistence has a
five-second deadline. Duplicate provider events are deduplicated atomically
across processes, excluding local receipt timestamps from the event identity.
Distinct statuses and error codes are retained as separate events; receipt
order is not assumed to be provider lifecycle order.

The journal is platform-owned because callbacks do not carry authenticated
organization context. It is not exposed by a customer API. Entries include the
configured Account SID and Messaging Service SID. Signed `AccountSid` and
`MessagingServiceSid` fields, when supplied, must match that configuration;
conflicts return 400. Configure this endpoint only for the selected service,
including number-level callbacks that omit service metadata. Keyword payloads contain the
sender and destination phone numbers and the normalized keyword. Status payloads
contain status and error code. Arbitrary inbound bodies, credentials, and raw
provider error text are not persisted. Restrict database/operator access to the
journal. Retention must preserve events until downstream delivery/consent
consumers have processed them; that processing belongs to the parent feature.

After signature verification, `OptOutType=STOP|START` takes precedence over body
matching. `HELP` is acknowledged without a consent event. A present empty or
unsupported `OptOutType` returns 400. If the field is absent, standard English
keywords are normalized, including `YES` to `START`. A keyword event does not
prove that a carrier has cleared all blocking: toll-free reactivation requires
START/UNSTOP, and YES alone is insufficient. See
[Twilio Advanced Opt-Out](https://www.twilio.com/docs/messaging/tutorials/advanced-opt-out).

Metrics record bounded submission and callback outcomes and durations. The
startup log reports whether SMS is enabled without logging configuration values.
HTTP request logging redacts `X-Twilio-Signature`.

## Deliberate exclusions

This configuration and its documentation do not add frontend behavior,
geofence-event generation, subscriber management, an outbox/queue, or outgoing
delivery-attempt persistence. STOP/START keyword interpretation is handed to the injected
consumer; suppression scope is not defined here.

Number purchase, sender-pool configuration, toll-free or 10DLC compliance
registration, environment rollout, and production activation are operational
work outside this task. Nothing in this document indicates that those steps
are complete.

## Operational ownership

- **Backend/platform owners** maintain the six-value application contract,
  inject secrets through the deployment secret mechanism, keep the public base
  URL canonical, apply the callback-journal migration, and operate the
  configured callback endpoints. Logs and metrics must not contain
  credentials, message bodies, phone numbers, delivery IDs, or organization
  IDs.
- **Twilio account owners** create and rotate API keys and the Auth Token and
  own Messaging Service sender-pool and compliance configuration. Those
  resources are not provisioned or changed by application startup.
- **Release/operations owners** approve any future environment-specific
  rollout and verify callback reachability and delivery monitoring. No staging
  or production rollout is claimed here.
