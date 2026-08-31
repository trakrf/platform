# Twilio SMS application integration

This document describes the application-side configuration boundary for the
TRA-1201 Twilio SMS integration. It is a developer-facing contract for loading
settings and constructing callback URLs; it does not activate Twilio traffic or
provision an account resource.

> **Activation warning:** TRA-1201 defines and tests `Handler.RegisterRoutes`,
> but `serve.setupRouter` does **not** mount these callbacks. The paths below
> are not reachable production endpoints today. They are intended future public,
> signature-protected endpoints. Do not configure or activate Twilio callback
> URLs until a durable callback consumer and production root-router wiring exist.

## Configuration

The six settings below are read by the backend. Keep every credential on the
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

## Outbound cancellation boundary

`SendSMS` honors a context that is already canceled or past its deadline before
submission: it returns without making an SDK request. Once `CreateMessage` has
started, Twilio Go v1.30.9 does not propagate cancellation or deadline changes
through its `CreateMessage` path; the SDK constructs that request from a
background context. Callers must therefore not assume that a cancellation or
deadline change interrupts an in-flight Twilio request. A context-aware SDK
transport or API path is future work if shorter in-flight deadlines become a
requirement.

## Callback paths (future public endpoints)

When production root wiring and a durable callback consumer are available,
Twilio will send form-encoded POST callbacks to these intended public paths,
appended to the configured canonical base URL:

| Callback | Path |
| --- | --- |
| Delivery status | `/api/v1/notifications/twilio/status` |
| Inbound keyword | `/api/v1/notifications/twilio/inbound` |

When mounted, the callback boundary validates the Twilio signature against the
externally visible URL (including the path and query, when present) before
emitting a normalized event to an injected consumer. The endpoints do not
require a TrakRF user session. Callback consumers are the seam for later
application integration; this ticket does not store callback events.

## Deliberate exclusions

This configuration and its documentation do not add frontend behavior,
geofence-event generation, subscriber management, an outbox/queue, or database
persistence. STOP/START keyword interpretation is handed to the injected
consumer; suppression scope is not defined here.

Number purchase, sender-pool configuration, toll-free or 10DLC compliance
registration, environment rollout, and production activation are operational
work outside this task. Nothing in this document indicates that those steps
are complete.

## Operational ownership

- **Backend/platform owners** maintain the six-value application contract,
  inject secrets through the deployment secret mechanism, keep the public base
  URL canonical, and—after durable consumer and root-router wiring exist—operate
  the mounted callback endpoints. Logs and metrics must not contain
  credentials, message bodies, phone numbers, delivery IDs, or organization
  IDs.
- **Twilio account owners** create and rotate API keys and the Auth Token and
  own Messaging Service sender-pool and compliance configuration. Those
  resources are not provisioned or changed by this documentation task.
- **Release/operations owners** approve any future environment-specific
  rollout and verify callback reachability and delivery monitoring. No staging
  or production rollout is claimed here.
