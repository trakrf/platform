# Feature: Backend Logging — Production-Grade Structured Logging

## Metadata

**Workspace**: backend
**Type**: feature

---

## Outcome

Every backend service emits structured, JSON-formatted logs with contextual fields (request, user, environment, trace) to `stdout` for collection by Docker logging drivers or sidecar agents (Fluentbit / Loki). Logs are safe (no secrets), consistent, and useful for production debugging and tracing.

---

## User Story

As a backend or SRE engineer,
I want all backend services to produce structured, correlated, and redacted logs,
So that I can easily trace issues, correlate events, and debug production incidents without leaking sensitive data.

---

## Context

**Current**:

* Logs are inconsistent and not structured.
* Errors are printed directly to console.
* No request correlation or trace context.
* Secrets or user data may accidentally appear in logs.

**Desired**:

* Unified structured logging format across all backend services.
* Standard request correlation using `request_id`, `trace_id`, `user_id`, and `account_id` where available.
* Automatic inclusion of contextual metadata (service name, environment, version).
* Logs written to `stdout` (for Docker / Kubernetes aggregation).
* Optional pretty-print output in development.
* Built-in sanitization and field masking for sensitive data.

---

## Implementation Strategy

### Phase 1: Development Environment Setup

**Goal**: Get developers logging locally with human-readable output

1. Install `zerolog` and create logger initialization
2. Configure console format with colors and caller info
3. Add basic HTTP middleware for request ID injection
4. Set default log level to `debug`
5. Test with `docker compose up` and verify `docker logs` output

**Success**: Developers see colorful, readable logs when running locally

### Phase 2: Add Environment Detection

**Goal**: Logger automatically adapts to environment

1. Implement environment detection logic (env var → K8s labels → default)
2. Create configuration loader with environment-specific defaults
3. Add format switching (console for dev, JSON for staging/prod)
4. Implement log level adjustment per environment
5. Add unit tests for each environment configuration

**Success**: Same codebase produces different log formats based on `ENVIRONMENT`

### Phase 3: Sanitization & Security

**Goal**: Protect sensitive data in logs

1. Build sanitization middleware with regex/rules for secrets
2. Add environment-aware PII masking (strict in staging/prod, relaxed in dev)
3. Implement request/response body logging controls
4. Add validation tests to detect redaction violations
5. Document which fields are safe to log in which environments

**Success**: No secrets or PII leaked in staging/production logs

### Phase 4: Performance & Sampling

**Goal**: Minimize overhead and control log volume

1. Implement sampling logic (environment-aware rates)
2. Add benchmarks to measure logging overhead
3. Optimize for zero-allocation where possible
4. Add backpressure handling (drop low-priority logs if blocked)
5. Run load tests to verify <3% overhead in production

**Success**: Logging meets performance targets in all environments

### Phase 5: Aggregation & Observability

**Goal**: Centralize logs for production monitoring

1. Configure Docker log drivers for staging/production
2. Deploy Fluentbit/Loki sidecar for log collection
3. Set up log retention policies per environment
4. Create audit log separation for sensitive actions
5. Configure alerts on error rate thresholds

**Success**: Logs flow to central system within SLA (1-2 minutes)

---

## Technical Requirements

### Environment-Specific Configuration

The logging system must adapt based on the deployment environment:

| Aspect | Development (`dev`) | Staging (`staging`) | Production (`prod`) |
|--------|---------------------|---------------------|---------------------|
| **Log Format** | Console (pretty-print) | JSON | JSON |
| **Default Log Level** | `debug` | `info` | `warn` |
| **Include Stack Traces** | Always (on errors) | On errors only | Never (unless `LOG_INCLUDE_STACK=true`) |
| **Sanitization** | Relaxed (emails visible) | Full (strict PII masking) | Full (strict PII masking) |
| **Sampling** | Disabled | Light (10% for debug) | Aggressive (50% for info, 10% for debug) |
| **Request Body Logging** | Full body (truncated) | Sanitized only | Never (unless whitelisted endpoints) |
| **Performance Overhead** | <10% acceptable | <5% | <3% |
| **Log Aggregation** | Optional (local Docker logs) | Required (Fluentbit/Loki) | Required (Fluentbit/Loki) |
| **Colored Output** | Yes (terminal colors) | No | No |
| **Caller Location** | Yes (file:line) | No | No |

### Logging Library

* Use a **zero-allocation structured logger** (example: `zerolog`).
* Format determined by `ENVIRONMENT` or `LOG_FORMAT` override:
  * **dev**: Console format with colors and human-readable timestamps
  * **staging/prod**: JSON format for machine parsing
* Log output always to `stdout` to integrate with Docker's logging driver.

### Log Schema (Fields)

Every log line must contain:

* **timestamp** – RFC3339 format.
* **level** – one of `debug`, `info`, `warn`, `error`, `fatal`.
* **service** – service name (e.g., `asset-api`).
* **env** – environment (`dev`, `staging`, `prod`).
* **request_id** – from HTTP middleware or generated UUID.
* **trace_id** / **span_id** – if tracing enabled.
* **user_id**, **account_id** – when available (IDs only).
* **operation** – logical action (e.g., `asset.bulk_upload`).
* **message** – short human-readable message.
* **duration_ms** – optional, for request timing.
* **error** – structured sub-object containing `type`, `message`, and optionally `stack`.
* **host** / **container_id** – identify running instance.
* **version** – git commit or build version.

Optional:

* **meta** – key/value object for additional info (IDs, parameters, etc.).

### Middleware Responsibilities

* Injects `request_id` into each request if not present.
* Adds logger into the request context so all handlers can log consistently.
* On request completion, logs:

  * HTTP method and path
  * status code
  * latency (ms)
  * remote IP
  * any captured errors or panics

### Log Sanitization

All logged data must pass through a **sanitizer** with environment-aware rules:

**All Environments (dev, staging, prod)**:
* Passwords, tokens, OAuth credentials, refresh tokens → `<redacted>`
* Authorization headers → `Bearer <redacted>`
* Credit card numbers → `****-****-****-1234` (last 4 digits only)
* API keys, secrets, private keys → `<redacted>`

**Staging & Production Only**:
* Email addresses → `us***@ex***.com` (partial masking)
* Phone numbers → `***-***-1234` (last 4 digits only)
* Full names → `J*** D***` (first letter only)
* IP addresses → `192.168.*.*` (partial masking)
* Sensitive request/response bodies → removed or sanitized

**Development Only**:
* Email addresses → visible (for debugging)
* Request/response bodies → truncated to 1000 chars (visible but limited)
* User IDs and account IDs → visible with clear labels

### Error Logging

* Errors must be logged with context: `operation`, `error.type`, and `error.message`.
* Stack traces only in development or when explicitly allowed in production.
* Fatal errors trigger immediate service exit; other errors continue operation.

### Audit Logging

Audit logs are **separate** from application logs and track security-sensitive actions:

**Actions to Audit (All Environments)**:
* User authentication (login, logout, failed attempts)
* Authorization changes (role assignments, permission grants)
* Data access (PII views, exports, bulk downloads)
* Configuration changes (settings updates, feature flags)
* Data mutations (create, update, delete of sensitive resources)

**Audit Log Schema**:
```json
{
  "timestamp": "2025-11-02T14:32:45Z",
  "audit_id": "aud_abc123",
  "service": "asset-api",
  "env": "prod",
  "actor_id": "user_456",
  "actor_type": "user",
  "actor_email": "us***@ex***.com",
  "action": "asset.export",
  "target_id": "asset_789",
  "target_type": "asset",
  "outcome": "success",
  "ip_address": "192.168.*.*",
  "user_agent": "Mozilla/5.0...",
  "metadata": {
    "export_format": "csv",
    "row_count": 1500
  }
}
```

**Environment-Specific Handling**:

| Aspect | Development | Staging | Production |
|--------|-------------|---------|------------|
| **Audit Stream** | Same as app logs | Separate stream | Separate stream (required) |
| **Retention** | 7 days | 90 days | 365 days (compliance) |
| **Access Control** | Open | Restricted to admins | Restricted to security team |
| **Immutability** | No | Yes (append-only) | Yes (append-only, signed) |
| **PII Masking** | No (emails visible) | Yes (partial masking) | Yes (full masking) |
| **Real-time Alerts** | No | Optional | Required (for critical actions) |

### Sampling and Backpressure

* Always log `error` and `warn` levels.
* Optionally sample `info` and `debug` logs under load to reduce volume.
* If the log sink (stdout) blocks, drop low-priority logs and count dropped messages.

### Performance

* Logging should add negligible latency (<5% request time).
* Must never block main request handling (non-blocking writes).
* Logger should reuse buffers and avoid unnecessary allocations.

### Environment Variables / Config

Configuration via environment variables with smart defaults based on `ENVIRONMENT`:

| Variable | Type | Dev Default | Staging Default | Prod Default | Description |
|----------|------|-------------|-----------------|--------------|-------------|
| `ENVIRONMENT` | string | `dev` | `staging` | `prod` | Deployment environment identifier |
| `SERVICE_NAME` | string | **required** | **required** | **required** | Service name (e.g., `asset-api`) |
| `LOG_LEVEL` | string | `debug` | `info` | `warn` | Minimum log level to emit |
| `LOG_FORMAT` | string | `console` | `json` | `json` | Output format (`json` or `console`) |
| `LOG_INCLUDE_STACK` | boolean | `true` | `false` | `false` | Include stack traces on errors |
| `LOG_INCLUDE_CALLER` | boolean | `true` | `false` | `false` | Include file:line caller info |
| `LOG_SAMPLE_RATE_INFO` | integer | `0` (disabled) | `100` (no sampling) | `50` (50% sampling) | Sampling % for `info` logs |
| `LOG_SAMPLE_RATE_DEBUG` | integer | `0` (disabled) | `10` (10% sampling) | `10` (10% sampling) | Sampling % for `debug` logs |
| `LOG_COLOR` | boolean | `true` | `false` | `false` | Enable terminal colors (console format only) |
| `LOG_SANITIZE_EMAILS` | boolean | `false` | `true` | `true` | Mask email addresses in logs |
| `LOG_SANITIZE_IPS` | boolean | `false` | `true` | `true` | Mask IP addresses in logs |
| `LOG_MAX_BODY_SIZE` | integer | `1000` | `0` (disabled) | `0` (disabled) | Max bytes of request/response body to log |
| `VERSION` | string | `dev-build` | git SHA | git SHA | Build version (injected at build time) |

**Override Behavior**:
* Explicitly set env vars always override environment-based defaults
* Example: `LOG_LEVEL=debug` in production enables debug logs (use cautiously)

### Example Log Output by Environment

**Development (Console Format)**:
```
2025-11-02T14:32:45Z DBG asset-api/handlers/upload.go:42 > Starting bulk upload operation
  request_id=req_abc123 user_id=user_456 account_id=acc_789
  file_size=2048576 filename=assets.csv

2025-11-02T14:32:46Z INF Request completed method=POST path=/api/v1/assets/upload
  status=200 duration_ms=1234 request_id=req_abc123 remote_ip=192.168.1.100

2025-11-02T14:32:50Z ERR Failed to process row error="validation failed: invalid GPS coordinates"
  operation=asset.validate row=42 request_id=req_abc123
  Stack trace:
    github.com/yourorg/platform/backend/services/assets.(*Service).Validate
    /app/services/assets/validator.go:128
```

**Staging/Production (JSON Format)**:
```json
{"level":"debug","service":"asset-api","env":"staging","timestamp":"2025-11-02T14:32:45Z","request_id":"req_abc123","trace_id":"tr_xyz","user_id":"user_456","account_id":"acc_789","operation":"asset.bulk_upload","message":"Starting bulk upload operation","file_size":2048576,"filename":"assets.csv","version":"7d113df"}

{"level":"info","service":"asset-api","env":"staging","timestamp":"2025-11-02T14:32:46Z","request_id":"req_abc123","trace_id":"tr_xyz","method":"POST","path":"/api/v1/assets/upload","status":200,"duration_ms":1234,"remote_ip":"192.168.*.*","version":"7d113df"}

{"level":"error","service":"asset-api","env":"prod","timestamp":"2025-11-02T14:32:50Z","request_id":"req_abc123","trace_id":"tr_xyz","operation":"asset.validate","error":{"type":"ValidationError","message":"invalid GPS coordinates"},"row":42,"version":"7d113df"}
```

**Key Differences**:
* Development shows file:line caller info, full IPs, colored output (not shown in markdown)
* Staging/Prod use JSON, mask IPs, no caller info
* Production omits stack traces unless explicitly enabled

### Deployment & Docker Integration

**All Environments**:
* Logs are written to **stdout** (never to files)
* Docker captures logs via default `json-file` or configured log driver
* No local file rotation or disk writes by the application
* Environment metadata injected at build time or runtime:
  * `SERVICE_NAME`: Required (e.g., `asset-api`)
  * `ENVIRONMENT`: Auto-detected or explicitly set (`dev`, `staging`, `prod`)
  * `VERSION`: Git SHA or build tag

**Development (Local Docker Compose)**:
* Use default `json-file` driver or no special configuration
* Logs viewable via `docker logs <container>` or `docker compose logs`
* Human-readable console format for easy debugging
* Optional: Mount logs to local volume for persistence

**Staging Environment**:
* Configure Docker log driver (e.g., `fluentd`, `loki`, or `json-file` with size limits)
* Deploy Fluentbit/Loki sidecar or centralized log collector
* Logs forwarded to aggregation system within 2 minutes
* Retention: 30 days typical

**Production Environment**:
* **Required**: Centralized log aggregation (Fluentbit → Loki/ELK)
* Docker log driver configured with:
  * Max size: 10MB per container
  * Max files: 3 rotations
  * Tag pattern: `{{.Name}}/{{.ID}}`
* Logs forwarded to aggregation system within 1 minute (SLA)
* Retention: 90 days minimum for compliance
* Alerts configured on error rate thresholds

**Environment Auto-Detection**:
The logger should detect `ENVIRONMENT` from:
1. Explicit `ENVIRONMENT` env var (highest priority)
2. Kubernetes namespace labels (`env=prod`, `env=staging`)
3. Docker compose service labels
4. Default to `dev` if unset (fail-safe)

---

## Testing Requirements

### Unit Tests (All Environments)

Test logging infrastructure with environment-specific configurations:

```go
func TestLoggerConfiguration(t *testing.T) {
  tests := []struct {
    env          string
    expectJSON   bool
    expectLevel  string
    expectCaller bool
  }{
    {"dev", false, "debug", true},
    {"staging", true, "info", false},
    {"prod", true, "warn", false},
  }

  for _, tt := range tests {
    // Test logger initialization with each environment
    // Verify format, level, and caller info match expectations
  }
}

func TestSanitization(t *testing.T) {
  tests := []struct {
    env      string
    input    string
    expected string
  }{
    {"dev", "user@example.com", "user@example.com"},      // Visible in dev
    {"staging", "user@example.com", "us***@ex***.com"},   // Masked in staging
    {"prod", "user@example.com", "us***@ex***.com"},      // Masked in prod
    {"prod", "password123", "<redacted>"},                // Redacted in all
  }
  // Verify sanitization behavior per environment
}
```

### Integration Tests

**Development**:
```bash
# Test console output format
ENVIRONMENT=dev go run main.go 2>&1 | grep -q "DBG.*>" && echo "✓ Console format works"

# Verify no sampling
ENVIRONMENT=dev go test ./... -v | grep "debug logs emitted: 100%"
```

**Staging**:
```bash
# Test JSON format
ENVIRONMENT=staging go run main.go 2>&1 | jq -r '.level' && echo "✓ JSON format works"

# Verify PII masking
docker compose -f docker-compose.staging.yml logs | jq '.actor_email' | grep -v "@" || echo "✓ Emails masked"

# Test log aggregation
curl -s http://loki:3100/api/v1/query?query={env="staging"} | jq '.data.result | length'
```

**Production**:
```bash
# Test strict redaction
docker logs asset-api 2>&1 | jq -r '.error' | grep -qv "password\|token" && echo "✓ No secrets leaked"

# Verify sampling rates
docker logs asset-api | jq -r 'select(.level=="info")' | wc -l  # Should be ~50% of total
docker logs asset-api | jq -r 'select(.level=="error")' | wc -l # Should be 100%

# Test aggregation SLA
# Emit log and verify it appears in Loki within 60 seconds
```

### Load Testing

**Performance Benchmarks**:
```bash
# Development: < 10% overhead acceptable
ENVIRONMENT=dev go test -bench=. -benchmem ./internal/logger
# BenchmarkLogging-8  500000  3245 ns/op  <-- Target

# Staging: < 5% overhead
ENVIRONMENT=staging go test -bench=. -benchmem ./internal/logger

# Production: < 3% overhead
ENVIRONMENT=prod go test -bench=. -benchmem ./internal/logger
```

---

## Validation Criteria

### All Environments

* [ ] Every log line contains `timestamp`, `service`, `env`, and `level`
* [ ] No logs contain plaintext passwords, tokens, or API keys (redacted in all envs)
* [ ] Request logs include `request_id` and latency for all HTTP calls
* [ ] Error logs contain structured fields (`error.type`, `error.message`)
* [ ] Service runs in Docker and logs appear in `docker logs` output
* [ ] Log level and format configurable via environment variables
* [ ] Logger initializes correctly with defaults when optional env vars are missing

### Development Environment

* [ ] Console output is human-readable with colors and pretty-printing
* [ ] Stack traces appear on all errors without configuration
* [ ] Caller information (file:line) appears in logs
* [ ] Email addresses visible in logs (for debugging)
* [ ] Request/response bodies logged (truncated to 1000 chars)
* [ ] No sampling applied (all logs emitted)
* [ ] Logging overhead < 10% verified in local benchmarks

### Staging Environment

* [ ] Logs output in valid JSON format (parsable by `jq`)
* [ ] Email addresses masked (`us***@ex***.com` format)
* [ ] IP addresses masked (`192.168.*.*` format)
* [ ] Stack traces only on errors, not info/debug logs
* [ ] Sampling enabled (10% for debug logs)
* [ ] Request/response bodies sanitized (no PII)
* [ ] Logs aggregated to centralized system (Fluentbit/Loki)
* [ ] Logging overhead < 5% verified under load testing

### Production Environment

* [ ] Logs output in valid JSON format (parsable by `jq`)
* [ ] No logs contain plaintext emails, phone numbers, or PII
* [ ] No request/response bodies logged unless explicitly whitelisted
* [ ] Stack traces disabled by default (unless `LOG_INCLUDE_STACK=true`)
* [ ] Aggressive sampling enabled (50% info, 10% debug)
* [ ] No caller location info (file:line) in logs
* [ ] Logs aggregated to centralized system within 1 minute
* [ ] Logging overhead < 3% verified under production-like load
* [ ] 100% of error/warn logs captured (no sampling)
* [ ] Redaction violations < 0.01% validated via automated scanning

---

## Success Metrics

### Development Environment

* [ ] 100% of logs emitted (no sampling) for full debugging visibility
* [ ] All errors include stack traces and caller location
* [ ] Log format switches automatically based on `ENVIRONMENT=dev`
* [ ] Colored console output renders correctly in terminal

### Staging Environment

* [ ] 100% of request logs are correlated by `request_id`
* [ ] < 0.1% of logs contain PII redaction violations
* [ ] Logs accessible from centralized aggregator within 2 minutes
* [ ] 95th percentile ingestion delay under 5 seconds
* [ ] < 5% performance overhead verified under load testing
* [ ] 90% of debug logs sampled out (10% captured)

### Production Environment

* [ ] 100% of request logs are correlated by `request_id`
* [ ] < 0.01% of logs contain redaction violations (strict compliance)
* [ ] Logs accessible from centralized aggregator within 1 minute
* [ ] 95th percentile ingestion delay under 2 seconds
* [ ] < 3% performance overhead verified under production load
* [ ] 100% of error/warn logs captured with full context
* [ ] 50% of info logs sampled out (volume control)
* [ ] 90% of debug logs sampled out (minimal debug in prod)

---

## References

* **Logging Library:** [rs/zerolog](https://github.com/rs/zerolog)
* **OpenTelemetry for Go:** [https://opentelemetry.io/docs/instrumentation/go/](https://opentelemetry.io/docs/instrumentation/go/)
* **Docker Logging Drivers:** [https://docs.docker.com/config/containers/logging/configure/](https://docs.docker.com/config/containers/logging/configure/)
* **Fluent Bit / Loki:** Recommended setup for centralized log aggregation.
* **OWASP Logging Guidelines:** Redaction and privacy best practices.
