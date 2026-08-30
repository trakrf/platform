# TRA-1201 Twilio SMS Boundary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a self-contained Twilio SMS boundary that can send through a Messaging Service and emit normalized, signature-verified callback events to injected consumers.

**Architecture:** TRA-1201 owns provider-neutral SMS interfaces plus the Twilio implementation. It does not depend on a database, queue, geofence, subscriber model, or frontend. TRA-1192 remains related because it will later consume these interfaces and provide durable implementations, but neither ticket blocks implementation of the other.

**Tech Stack:** Go 1.25.1, `github.com/twilio/twilio-go` v1.30.9, Chi, Prometheus, Testify.

**Spec:** Linear `TRA-1201`

## Global Constraints

- TrakRF is the centralized sender.
- Outbound calls use API Key SID + API Key Secret with Account SID as context.
- Auth Token is used only for callback signature validation.
- Sends use a Messaging Service SID and never configure a raw `From` number.
- Toll-free versus 10DLC is outside application code.
- All-empty configuration disables Twilio; partial configuration fails closed.
- Callback handlers validate signatures before producing domain events.
- Callback consumers are injected interfaces; this ticket supplies no persistence.
- STOP/START scope is not decided in the Twilio layer.
- No credentials, phone numbers, message bodies, delivery IDs, or organization IDs in logs or metric labels.
- Automated tests never contact Twilio.
- Tests assert observable results and public contracts: returned values and errors, HTTP responses, normalized callback events, signature rejection, and sensitive-data redaction.
- Tests do not assert private helper calls, internal call order, or implementation mechanics unless ordering is itself part of the public contract.
- Fakes are limited to the Twilio network boundary and injected callback-consumer boundary.
- Each task below is one review-sized MR and touches no more than five files.

---

## Context-window progress log

This section is the durable handoff record across fresh implementation contexts. Update it whenever a task starts, completes, changes scope, or discovers a cross-task constraint.

| Date (UTC) | Context / task | Result and decisions |
|---|---|---|
| 2026-08-30 | Planning | TRA-1201 moved to In Progress. Linear records the implementation scope, meaningful outcome-focused testing, and explicit exclusions for frontend and geofencing. |
| 2026-08-30 | Workspace setup | Created isolated feature worktree and branch `nicholusmuwonge/tra-1201-twilio-sms-integration` from current `origin/main`. Implementation has not started. |
| 2026-08-30 | Task 1 implementation and review | Defined provider-neutral SMS contracts using TDD. Review rejected fake-bookkeeping assertions; a fresh fix context replaced them with external-package public API contract checks. Re-review approved; targeted, race, vet, and diff checks pass. |
| 2026-08-30 | Task 2 implementation and review | Added fail-closed configuration using TDD. Review required canonical HTTPS origins and tidy dependency state; a fresh fix rejected userinfo/path/query/fragment forms and removed the then-unused SDK pin. Re-review approved; targeted, race, vet, module, notification-regression, and diff checks pass. |
| 2026-08-30 | Task 3 implementation and review | Documented configuration and ownership boundaries. Review caught a non-empty URL that made a copied template partially configured; a fresh fix made all six active values empty and retained only a commented example. Re-review approved; ADR 0010, static assignment, targeted config, and diff checks pass. |
| 2026-08-30 | Task 4 implementation and review | Classified legacy and V1 Twilio REST errors plus wrapped network failures using TDD. The structured result is redacted and does not retain raw provider chains. Task 4 legitimately imports and pins SDK v1.30.9. Independent review approved with all targeted, race, vet, module, regression, and diff checks passing. |
| 2026-08-30 | Task 5 implementation and review | Implemented the sender. Review rejected discarded cancellation, SDK-internal/fake-bookkeeping tests, and an untidy checksum set. A fresh fix added pre-submit cancellation and real SDK HTTP-contract tests for auth, account path, exact forms/no `From`, callbacks, response/error outcomes, and concurrency. Re-review approved; all targeted/race/vet/module/regression/diff checks pass. |
| 2026-08-30 | Task 6 implementation and review | Added bounded, signature-verified form parsing. Review found unsigned duplicate values and unsafe handler construction; a fresh fix rejects duplicate keys and makes construction fail closed for disabled/invalid config. Re-review approved; external URL/proxy/body-bound and all targeted checks pass. |

### Current handoff

- Next task: Task 7, delivery-status callback parsing.
- Implementation rule: use a fresh subagent context for every task, followed by an independent review context.
- Not implementable in this ticket: frontend and geofence-event generation/integration.

---

### Task 1: Define provider-neutral SMS contracts

**MR file count:** 2

**Files:**
- Create: `backend/internal/notification/sms/contracts.go`
- Create: `backend/internal/notification/sms/contracts_test.go`

**Produces:** `sms.Command`, `sms.Submission`, `sms.ProviderError`, `sms.Sender`, `sms.ProviderStatus`, `sms.InboundKeyword`, and `sms.CallbackConsumer`.

- [x] **Step 1: Write compile-time contract tests**

Define these public shapes:

```go
type Command struct {
    DeliveryID string
    ToE164     string
    Body       string
}

type Submission struct {
    ProviderMessageID string
    Status            string
}

type ErrorKind string

const (
    ErrorTransient ErrorKind = "transient"
    ErrorPermanent ErrorKind = "permanent"
    ErrorRejected  ErrorKind = "rejected"
)

type ProviderError struct {
    Kind       ErrorKind
    Code       string
    HTTPStatus int
}

type Sender interface {
    SendSMS(context.Context, Command) (Submission, error)
}

type ProviderStatus struct {
    ProviderMessageID string
    Status            string
    ErrorCode         string
    OccurredAt        time.Time
}

type InboundKeyword struct {
    ProviderMessageID string
    FromE164          string
    ToE164            string
    Keyword           string
    ReceivedAt        time.Time
}

type CallbackConsumer interface {
    HandleStatus(context.Context, ProviderStatus) error
    HandleKeyword(context.Context, InboundKeyword) error
}
```

- [x] **Step 2: Run the tests**

Run: `cd backend && go test ./internal/notification/sms -count=1`

Expected: PASS with no Twilio or storage import.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/notification/sms/contracts.go backend/internal/notification/sms/contracts_test.go
git commit -m "feat(TRA-1201): define SMS provider contracts"
```

---

### Task 2: Add Twilio configuration

**MR file count:** 2

**Files:**
- Create: `backend/internal/notification/twilio/config.go`
- Create: `backend/internal/notification/twilio/config_test.go`

**Consumes:** environment variables.

**Produces:** `twilio.Config`, `twilio.ConfigFromEnv() (Config, error)`, and `Config.Enabled() bool`.

- [x] **Step 1: Write failing configuration tests**

```go
type Config struct {
    AccountSID          string
    APIKeySID           string
    APIKeySecret        string
    AuthToken           string
    MessagingServiceSID string
    PublicBaseURL       string
}
```

Test all-empty disabled, complete enabled, partial rejected, secrets absent from errors, and public URLs that are not canonical HTTPS origins rejected outside tests. A trailing slash is rejected so Task 5 can append its callback path without producing a double slash.

- [x] **Step 2: Verify failure**

Run: `cd backend && go test ./internal/notification/twilio -run TestConfigFromEnv -count=1`

Expected: FAIL because the package does not exist.

- [x] **Step 3: Implement the loader**

Read `TWILIO_ACCOUNT_SID`, `TWILIO_API_KEY_SID`, `TWILIO_API_KEY_SECRET`, `TWILIO_AUTH_TOKEN`, `TWILIO_MESSAGING_SERVICE_SID`, and `TWILIO_PUBLIC_BASE_URL`. Do not read a sender-number variable.

- [x] **Step 4: Verify**

Run: `cd backend && go test ./internal/notification/twilio -run TestConfigFromEnv -count=1 && go mod tidy -diff && git diff --check`

- [x] **Step 5: Commit**

```bash
git add backend/internal/notification/twilio/config.go backend/internal/notification/twilio/config_test.go
git commit -m "feat(TRA-1201): configure Twilio client"
```

---

### Task 3: Document application environment settings

**MR file count:** 2 (the repository intentionally has no `.env.example`; see ADR 0010)

**Files:**
- Modify: `.env.local.example`
- Create: `docs/operations/twilio-application-integration.md`

**Consumes:** configuration names from Task 2.

**Produces:** developer-facing configuration documentation only.

- [x] **Step 1: Add empty environment examples**

The six settings are present in the canonical `.env.local.example`. The
planned `.env.example` copy is deliberately not created because ADR 0010
requires one local environment template.

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

- [x] **Step 2: Document boundaries**

Explain API-key authentication, Auth Token signature validation, Messaging Service sending, disabled/partial behavior, callback paths, and the deliberate absence of `TWILIO_FROM_NUMBER`. Exclude number purchase, compliance registration, staging, and production rollout.

- [ ] **Step 3: Verify and commit**

The verification portion passed. No commit was created because this context
was instructed to leave integration to the parent agent.

Run: `git diff --check`

```bash
git add .env.example .env.local.example docs/operations/twilio-application-integration.md
git commit -m "docs(TRA-1201): document Twilio application settings"
```

---

### Task 4: Classify Twilio provider errors

**MR file count:** 4

**Files:**
- Create: `backend/internal/notification/twilio/errors.go`
- Create: `backend/internal/notification/twilio/errors_test.go`
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`

**Consumes:** `sms.ProviderError` from Task 1 and Twilio REST errors.

**Produces:** `classifyError(error) error` returning normalized provider errors.

- [x] **Step 1: Write failing table-driven tests**

Assert:

```text
Permanent: 21211, 21408, 21610, 21612, 30034, unknown 4xx
Rejected: 30007, 30450
Transient: 429, 5xx, timeout, temporary network failure
```

Assert normalized errors exclude destination, body, and credentials.

- [x] **Step 2: Verify failure**

Run: `cd backend && go test ./internal/notification/twilio -run TestClassifyError -count=1`

Expected: FAIL because `classifyError` is undefined.

- [x] **Step 3: Implement classification**

Use Twilio error code and HTTP status when available. Preserve no raw provider error text that could contain request data.

- [x] **Step 4: Verify**

Run: `cd backend && go test -race ./internal/notification/twilio -run TestClassifyError -count=1 && go vet ./internal/notification/twilio && go mod tidy -diff && go mod verify && git diff --check`

The verification portion passed. No commit was created because this context was
instructed to leave integration to the parent agent.

```bash
git add backend/internal/notification/twilio/errors.go backend/internal/notification/twilio/errors_test.go backend/go.mod backend/go.sum
git commit -m "feat(TRA-1201): classify Twilio failures"
```

---

### Task 5: Implement outbound SMS sending

**MR file count:** 3

**Files:**
- Modify: `backend/internal/notification/twilio/sender.go`
- Modify: `backend/internal/notification/twilio/sender_test.go`
- Modify: `backend/go.sum`

**Consumes:** `sms.Command`, `sms.Submission`, `sms.Sender`, `twilio.Config`, and Task 4 classification.

**Produces:** `twilio.Sender`, satisfying `sms.Sender`.

- [x] **Step 1: Write failing sender tests**

Exercise the official SDK through a local HTTP transport. Verify the Messages POST
path, API Key SID/Secret Basic auth (not Auth Token), Account SID context, exact
form fields (`To`, `Body`, `MessagingServiceSid`, `StatusCallback` only), returned
SID/status, normalized/redacted HTTP provider errors, nil SDK responses,
pre-cancelled context behavior, and concurrent safety on the production HTTP path.

- [x] **Step 2: Verify failure**

The cancellation regression was observed RED before the implementation: a
pre-cancelled call returned a successful submission. The transport-suite rewrite
was also observed RED before adding the minimal client-construction seam.

- [x] **Step 3: Use the Twilio SDK pinned by Task 4**

Task 4 imports the official SDK's concrete REST error types, so it pins
`github.com/twilio/twilio-go` v1.30.9 and owns `backend/go.mod` and
`backend/go.sum`. Task 5 reuses that pin.

- [x] **Step 4: Implement sending**

Construct the official Twilio client with API Key SID, API Key Secret, and Account
SID. Before submission, return `context.Cause(ctx)` when the caller context is
already cancelled or expired. Otherwise set `To`, `Body`, `MessagingServiceSid`,
and `${PublicBaseURL}/api/v1/notifications/twilio/status` only.

- [x] **Step 5: Verify**

Run: `cd backend && go test -race ./internal/notification/twilio -run 'TestSender|TestSendSMS' -count=1 && go vet ./internal/notification/twilio && go mod tidy -diff && go mod verify && go test ./internal/notification/... -count=1 && git diff --check`

All commands pass. `go mod tidy` added the two required
`github.com/localtunnel/go-localtunnel` checksums to `backend/go.sum`; `go.mod`
is unchanged. No commit was created because this context must leave integration
to the parent agent.

```bash
git add backend/internal/notification/twilio/sender.go backend/internal/notification/twilio/sender_test.go backend/go.sum
git commit -m "feat(TRA-1201): send SMS through Twilio"
```

---

### Task 6: Build callback signature validation

**MR file count:** 3

**Files:**
- Create: `backend/internal/handlers/twiliosms/handler.go`
- Create: `backend/internal/handlers/twiliosms/signature.go`
- Create: `backend/internal/handlers/twiliosms/signature_test.go`

**Consumes:** Auth Token, Public Base URL, and `sms.CallbackConsumer`.

**Produces:** `twiliosms.Handler` and verified form parsing shared by both callback types.

- [x] **Step 1: Write failing signature tests**

Cover valid form signatures, invalid signatures, missing signatures, path/query inclusion, and canonical public URL reconstruction behind a proxy.

- [x] **Step 2: Verify failure**

Run: `cd backend && go test ./internal/handlers/twiliosms -run TestSignature -count=1`

Expected: FAIL because the handler package does not exist.

- [x] **Step 3: Implement validation**

Use `client.NewRequestValidator`. Validate `X-Twilio-Signature` against configured public origin plus request path and raw query before producing callback events.

- [x] **Step 4: Verify and commit**

Run: `cd backend && go test ./internal/handlers/twiliosms -run TestSignature -count=1`

```bash
git add backend/internal/handlers/twiliosms/handler.go backend/internal/handlers/twiliosms/signature.go backend/internal/handlers/twiliosms/signature_test.go
git commit -m "feat(TRA-1201): validate Twilio callback signatures"
```

---

### Task 7: Parse delivery-status callbacks

**MR file count:** 2

**Files:**
- Create: `backend/internal/handlers/twiliosms/status.go`
- Create: `backend/internal/handlers/twiliosms/status_test.go`

**Consumes:** verified callback helper and `sms.CallbackConsumer`.

**Produces:** `Handler.Status(http.ResponseWriter, *http.Request)`.

- [ ] **Step 1: Write failing handler tests**

Cover `queued`, `sent`, `delivered`, `undelivered`, and `failed`; optional `ErrorCode`; required Message SID/status; invalid signature; consumer failure; and repeated callback delivery.

- [ ] **Step 2: Verify failure**

Run: `cd backend && go test ./internal/handlers/twiliosms -run TestStatus -count=1`

Expected: FAIL because `Status` is undefined.

- [ ] **Step 3: Implement the thin receiver**

After signature validation, emit one `sms.ProviderStatus`. Return `204` on success, `400` for malformed signed input, `403` for invalid signatures, and `500` when the consumer fails.

- [ ] **Step 4: Verify and commit**

Run: `cd backend && go test ./internal/handlers/twiliosms -run TestStatus -count=1`

```bash
git add backend/internal/handlers/twiliosms/status.go backend/internal/handlers/twiliosms/status_test.go
git commit -m "feat(TRA-1201): parse Twilio delivery callbacks"
```

---

### Task 8: Parse inbound consent keywords

**MR file count:** 2

**Files:**
- Create: `backend/internal/handlers/twiliosms/inbound.go`
- Create: `backend/internal/handlers/twiliosms/inbound_test.go`

**Consumes:** verified callback helper and `sms.CallbackConsumer`.

**Produces:** `Handler.Inbound(http.ResponseWriter, *http.Request)`.

- [ ] **Step 1: Write failing handler tests**

Cover case-insensitive, trimmed `STOP`, `START`, `CANCEL`, `UNSUBSCRIBE`, `END`, and `QUIT`; invalid signatures; repeated Message SID; consumer failure; and unrelated text.

- [ ] **Step 2: Verify failure**

Run: `cd backend && go test ./internal/handlers/twiliosms -run TestInbound -count=1`

Expected: FAIL because `Inbound` is undefined.

- [ ] **Step 3: Implement keyword handoff**

Normalize opt-out synonyms to `STOP`, retain `START`, and emit `sms.InboundKeyword`. Acknowledge unrelated text without storing its body. Do not implement suppression scope.

- [ ] **Step 4: Verify and commit**

Run: `cd backend && go test ./internal/handlers/twiliosms -run TestInbound -count=1`

```bash
git add backend/internal/handlers/twiliosms/inbound.go backend/internal/handlers/twiliosms/inbound_test.go
git commit -m "feat(TRA-1201): parse Twilio consent callbacks"
```

---

### Task 9: Define public callback routes

**MR file count:** 2

**Files:**
- Create: `backend/internal/handlers/twiliosms/routes.go`
- Create: `backend/internal/handlers/twiliosms/routes_test.go`

**Consumes:** `Handler.Status` and `Handler.Inbound`.

**Produces:** `Handler.RegisterRoutes(chi.Router)`.

- [ ] **Step 1: Write failing route tests**

Verify public form-encoded POST routes at `/api/v1/notifications/twilio/status` and `/api/v1/notifications/twilio/inbound`, no TrakRF authentication dependency, signature rejection at the handler, and `405` with `Allow: POST` for GET.

- [ ] **Step 2: Verify failure**

Run: `cd backend && go test ./internal/handlers/twiliosms -run TestRoutes -count=1`

Expected: FAIL because `RegisterRoutes` is undefined.

- [ ] **Step 3: Implement route registration**

Register only the two form-encoded POST endpoints. Production attachment to the root router occurs in the later integration work that supplies a real callback consumer.

- [ ] **Step 4: Verify and commit**

Run: `cd backend && go test ./internal/handlers/twiliosms -run TestRoutes -count=1`

```bash
git add backend/internal/handlers/twiliosms/routes.go backend/internal/handlers/twiliosms/routes_test.go
git commit -m "feat(TRA-1201): define Twilio callback routes"
```

---

### Task 10: Add bounded Twilio metrics

**MR file count:** 2

**Files:**
- Create: `backend/internal/notification/twilio/metrics.go`
- Create: `backend/internal/notification/twilio/metrics_test.go`

**Consumes:** sender outcomes and callback outcomes.

**Produces:** bounded provider metrics.

- [ ] **Step 1: Write failing metric tests**

Define:

```text
trakrf_twilio_submissions_total{result}
trakrf_twilio_callbacks_total{type,result}
trakrf_twilio_request_duration_seconds
```

Allow only fixed result values. Assert metric descriptors contain no organization, delivery, phone, error-message, or message-body labels.

- [ ] **Step 2: Verify failure**

Run: `cd backend && go test ./internal/notification/twilio -run TestMetrics -count=1`

Expected: FAIL because metrics are undefined.

- [ ] **Step 3: Implement instrumentation**

Instrument accepted, transient error, permanent error, rejected, invalid signature, malformed callback, and consumer failure paths.

- [ ] **Step 4: Verify and commit**

Run: `cd backend && go test ./internal/notification/twilio ./internal/handlers/twiliosms -count=1`

```bash
git add backend/internal/notification/twilio/metrics.go backend/internal/notification/twilio/metrics_test.go
git commit -m "feat(TRA-1201): instrument Twilio boundary"
```

---

### Task 11: Verify the complete independent boundary

**MR file count:** 2

**Files:**
- Create: `backend/internal/notification/twilio/integration_test.go`
- Create: `backend/internal/handlers/twiliosms/handler_integration_test.go`

**Consumes:** all prior TRA-1201 components.

**Produces:** integration evidence without storage, workers, or external requests.

- [ ] **Step 1: Add sender integration coverage**

Using a fake Twilio client, verify Messaging Service sending, returned Message SID/status, error classification, concurrent safety, and redacted logs.

- [ ] **Step 2: Add callback integration coverage**

Using signed fixtures and an in-memory fake consumer, verify status and STOP/START handoff, invalid-signature rejection, repeated callback delivery, and absence of arbitrary inbound body persistence.

- [ ] **Step 3: Run full verification**

```bash
cd backend
go test ./internal/notification/sms ./internal/notification/twilio ./internal/handlers/twiliosms -count=1
go test -race ./internal/notification/sms ./internal/notification/twilio ./internal/handlers/twiliosms -count=1
go test ./... -count=1
go vet ./...
```

Expected: all commands pass and no external Twilio request occurs.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/notification/twilio/integration_test.go backend/internal/handlers/twiliosms/handler_integration_test.go
git commit -m "test(TRA-1201): verify independent Twilio boundary"
```
