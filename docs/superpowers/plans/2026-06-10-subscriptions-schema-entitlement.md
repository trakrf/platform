# Subscriptions schema + lite entitlement — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land one Stripe-aware schema (org entitlement columns + `subscription_plans` + `subscriptions` + `partners`) and ship working "lite" entitlement — a server-side `is_entitled` gate that returns 402 on paid mutations, plus entitlement in the session payload — closing TRA-947 and laying the TRA-198 foundation.

**Architecture:** Migration 000022 adds the schema and a single `trakrf.org_is_entitled(org_id)` SECURITY DEFINER function that encodes the whole entitlement formula (manual booleans OR active subscription) so it's callable from request middleware with no org context (same pattern as `list_active_scan_topics`). A `SubscriptionRequired` middleware calls it and rejects non-entitled mutations with a 402 envelope. `GetUserProfile` surfaces `is_entitled` + raw fields. The self-service signup path sets a 1-month trial; everything else stays perpetual.

**Tech Stack:** Go (chi router, pgx), TimescaleDB/Postgres, golang-migrate (embedded), RFC-7807-ish error envelope. Tests: `just backend test` (unit) and `just backend test-integration` (live Postgres, `integration` build tag).

**Spec:** `docs/superpowers/specs/2026-06-10-subscriptions-schema-entitlement-design.md`

**Worktree:** `.claude/worktrees/feat+tra-947-subscriptions-schema-entitlement` (branch `worktree-feat+tra-947-subscriptions-schema-entitlement`). Run all commands from the worktree root unless a step says `cd backend`.

---

## File map

- Create `backend/migrations/000022_subscriptions_schema.up.sql` — enums, partners, subscription_plans, subscriptions, org columns, backfill, seed, `org_is_entitled()`.
- Create `backend/migrations/000022_subscriptions_schema.down.sql` — reverse.
- Modify `backend/internal/models/organization/organization.go` — `Organization` fields + `UserOrgWithRole` fields.
- Modify `backend/internal/storage/organizations.go` — extend SELECTs; add `OrgIsEntitled`.
- Modify `backend/internal/models/errors/errors.go` — `ErrPaymentRequired` + title + enum tag.
- Modify `backend/internal/util/httputil/auth_error.go` — `Respond402PaymentRequired`.
- Create `backend/internal/middleware/subscription.go` — `SubscriptionRequired` + `EntitlementChecker`.
- Create `backend/internal/middleware/subscription_test.go` — unit tests.
- Modify `backend/internal/cmd/serve/router.go` — wire gate into public write group + paid internal mounts.
- Modify `backend/internal/handlers/scandevices/scandevices.go`, `scanpoints/scanpoints.go`, `outputdevices/outputdevices.go`, `assets/assets.go` — accept + apply the gate on paid mutating routes.
- Modify `backend/internal/services/orgs/service.go` — populate entitlement in `GetUserProfile`.
- Modify `backend/internal/services/auth/auth.go` — 1-month trial on self-service signup.
- Integration tests alongside the above (`*_integration_test.go`, build tag `integration`).

---

## Task 1: Migration 000022 — schema + entitlement function

**Files:**
- Create: `backend/migrations/000022_subscriptions_schema.up.sql`
- Create: `backend/migrations/000022_subscriptions_schema.down.sql`

- [ ] **Step 1: Write the up migration**

Create `backend/migrations/000022_subscriptions_schema.up.sql`:

```sql
-- TRA-947 / TRA-198 foundation — Stripe-aware subscriptions schema + "lite"
-- entitlement. Hybrid model: manual on/off booleans on the org (the lite gate)
-- OR an active subscription row (dormant until Stripe / TRA-135). Plans are
-- immutable price-points (pin + supersede grandfathering); custom plans set
-- owner_org_id. partners is the reseller / whitelabel / future-tenant home.
-- No in-migration GRANTs: the infra init-grants Job sets ALTER DEFAULT
-- PRIVILEGES for the migrate role; the integration harness grants CRUD to the
-- RLS role post-migrate (same as alarm_devices / asset_scans).
SET search_path = trakrf, public;

-- ── enums ────────────────────────────────────────────────────────────────
CREATE TYPE subscription_status AS ENUM ('active','trialing','past_due','canceled','incomplete');
CREATE TYPE payment_rail        AS ENUM ('stripe','invoice');

-- ── partners (reseller / whitelabel / future ThingsBoard-style tenant) ─────
CREATE TABLE partners (
    id                 BIGINT PRIMARY KEY,
    name               VARCHAR(255) NOT NULL,
    identifier         VARCHAR(255) UNIQUE,
    billing_email      VARCHAR(255),
    stripe_customer_id VARCHAR(255),
    metadata           JSONB NOT NULL DEFAULT '{}',
    is_active          BOOLEAN NOT NULL DEFAULT true,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at         TIMESTAMPTZ
);

CREATE TRIGGER generate_partner_id_trigger
    BEFORE INSERT ON partners
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();
CREATE TRIGGER update_partners_updated_at
    BEFORE UPDATE ON partners
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

COMMENT ON TABLE partners IS 'TRA-947: reseller / whitelabel billing entity; future home for ThingsBoard-style partner tenancy (System-Admin -> Partner -> Organization). Internal-only, no per-org RLS.';

-- ── subscription_plans (immutable price-points) ────────────────────────────
CREATE TABLE subscription_plans (
    id              BIGINT PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    owner_org_id    BIGINT REFERENCES organizations(id),  -- NULL = standard/shared; set = custom
    stripe_price_id VARCHAR(255),
    max_users       INT,
    max_assets      INT,
    max_locations   INT,
    features        JSONB NOT NULL DEFAULT '{}',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_subscription_plans_owner   ON subscription_plans(owner_org_id);
CREATE INDEX idx_subscription_plans_standard ON subscription_plans(id)
    WHERE owner_org_id IS NULL AND is_active;

CREATE TRIGGER generate_subscription_plan_id_trigger
    BEFORE INSERT ON subscription_plans
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();
CREATE TRIGGER update_subscription_plans_updated_at
    BEFORE UPDATE ON subscription_plans
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

-- RLS: standard rows (owner_org_id NULL) globally readable; custom rows scoped
-- to their org. missing_ok current_setting so a no-context read does not throw.
ALTER TABLE subscription_plans ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_subscription_plans ON subscription_plans
    USING (owner_org_id IS NULL
           OR owner_org_id = current_setting('app.current_org_id', true)::BIGINT);

COMMENT ON TABLE subscription_plans IS 'TRA-198: immutable plan price-points. Standard = owner_org_id NULL (shared); custom = owner_org_id set. Pin + supersede for grandfathering — never edit price/limits after pin; insert a new row and flip is_active.';

-- ── subscriptions (optional Stripe-managed deal; dormant until TRA-135) ─────
CREATE TABLE subscriptions (
    id                     BIGINT PRIMARY KEY,
    org_id                 BIGINT NOT NULL REFERENCES organizations(id),
    plan_id                BIGINT NOT NULL REFERENCES subscription_plans(id),
    status                 subscription_status NOT NULL,
    current_period_end     TIMESTAMPTZ,
    payment_rail           payment_rail NOT NULL DEFAULT 'stripe',
    reseller_id            BIGINT REFERENCES partners(id),  -- NULL = bill org directly
    stripe_subscription_id VARCHAR(255),
    external_billing_ref   TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    canceled_at            TIMESTAMPTZ
);

CREATE INDEX idx_subscriptions_org ON subscriptions(org_id);
-- At most one active subscription per org.
CREATE UNIQUE INDEX idx_subscriptions_one_active_per_org
    ON subscriptions(org_id) WHERE status = 'active';

CREATE TRIGGER generate_subscription_id_trigger
    BEFORE INSERT ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();
CREATE TRIGGER update_subscriptions_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE subscriptions ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_subscriptions ON subscriptions
    USING (org_id = current_setting('app.current_org_id', true)::BIGINT);

COMMENT ON TABLE subscriptions IS 'TRA-947/TRA-135: org subscription. Dormant until Stripe sync (no rows yet). plan_id pins an immutable subscription_plans row.';

-- ── organizations: entitlement + billing columns ──────────────────────────
-- NOTE: expires_at default is NULL (perpetual). There is deliberately NO blanket
-- 1-month trial default — that would silently expire trakrf-created customer
-- orgs. The trial is set explicitly on the self-service signup path only.
ALTER TABLE organizations
    ADD COLUMN subscription_enabled      BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN subscription_expires_at   TIMESTAMPTZ,
    ADD COLUMN stripe_customer_id        VARCHAR(255),
    ADD COLUMN default_payment_method_id VARCHAR(255),
    ADD COLUMN partner_id                BIGINT REFERENCES partners(id);

COMMENT ON COLUMN organizations.subscription_enabled    IS 'TRA-947: manual entitlement kill switch.';
COMMENT ON COLUMN organizations.subscription_expires_at IS 'TRA-947: manual entitlement expiry; NULL = perpetual (comp/partner/internal). Set to now()+1mo only on self-service signup.';
COMMENT ON COLUMN organizations.partner_id              IS 'TRA-947: dormant forward-hook for ThingsBoard-style partner tenancy / whitelabel. No behavior attached yet.';

-- All existing orgs entitled in perpetuity (defensive; ADD COLUMN already
-- backfilled enabled=true / expires_at NULL, but be explicit for intent).
UPDATE organizations SET subscription_enabled = true, subscription_expires_at = NULL;

-- ── seed standard tiers (shared rows; prices/limits land with TRA-337/Stripe) ─
INSERT INTO subscription_plans (name, owner_org_id, features) VALUES
    ('Free',         NULL, '{}'),
    ('Starter',      NULL, '{}'),
    ('Professional', NULL, '{}'),
    ('Enterprise',   NULL, '{}');

-- ── entitlement formula as a single SECURITY DEFINER function ──────────────
-- Callable from request middleware with NO org context set (mirrors
-- list_active_scan_topics). Encodes: manual override OR active subscription.
CREATE OR REPLACE FUNCTION trakrf.org_is_entitled(p_org_id BIGINT)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    SELECT
        COALESCE((
            SELECT o.subscription_enabled
                   AND (o.subscription_expires_at IS NULL OR now() < o.subscription_expires_at)
            FROM trakrf.organizations o
            WHERE o.id = p_org_id AND o.deleted_at IS NULL
        ), false)
        OR EXISTS (
            SELECT 1 FROM trakrf.subscriptions s
            WHERE s.org_id = p_org_id
              AND s.status = 'active'
              AND (s.current_period_end IS NULL OR now() < s.current_period_end)
        );
$$;

COMMENT ON FUNCTION trakrf.org_is_entitled(BIGINT) IS 'TRA-947: effective org entitlement (manual booleans OR active subscription). SECURITY DEFINER so the RLS-enforced app role can call it pre-org-context in middleware.';
```

- [ ] **Step 2: Write the down migration**

Create `backend/migrations/000022_subscriptions_schema.down.sql`:

```sql
SET search_path = trakrf, public;

DROP FUNCTION IF EXISTS trakrf.org_is_entitled(BIGINT);

ALTER TABLE organizations
    DROP COLUMN IF EXISTS partner_id,
    DROP COLUMN IF EXISTS default_payment_method_id,
    DROP COLUMN IF EXISTS stripe_customer_id,
    DROP COLUMN IF EXISTS subscription_expires_at,
    DROP COLUMN IF EXISTS subscription_enabled;

DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS subscription_plans;
DROP TABLE IF EXISTS partners;

DROP TYPE IF EXISTS payment_rail;
DROP TYPE IF EXISTS subscription_status;
```

- [ ] **Step 3: Apply the migration to the local DB**

Run: `just backend migrate`
Expected: ends with `✅ Migrations complete` and no error. (Requires `PG_URL_LOCAL` set and local TimescaleDB up; if `just backend migrate` reports no `PG_URL_LOCAL`, start the stack with `just dev-local` first or export `PG_URL_LOCAL`.)

- [ ] **Step 4: Verify schema + function with psql**

Run:
```bash
cd backend
psql "$PG_URL_LOCAL" -c "\d trakrf.organizations" -c "\d trakrf.subscriptions" \
  -c "SELECT name FROM trakrf.subscription_plans WHERE owner_org_id IS NULL ORDER BY name;" \
  -c "SELECT trakrf.org_is_entitled(id) FROM trakrf.organizations LIMIT 1;"
```
Expected: organizations shows the 5 new columns; subscriptions table exists; four plan names (Enterprise, Free, Professional, Starter); `org_is_entitled` returns `t` for an existing (entitled) org.

- [ ] **Step 5: Verify the down migration reverses cleanly**

Run: `cd backend && go run . migrate down 1 2>/dev/null || env PG_URL="$PG_URL_LOCAL" go run . migrate-down`
If the embedded runner has no `down` subcommand, instead verify by hand:
```bash
psql "$PG_URL_LOCAL" -f migrations/000022_subscriptions_schema.down.sql
psql "$PG_URL_LOCAL" -c "\d trakrf.subscriptions" 2>&1 | grep -q "Did not find" && echo "DOWN_OK"
```
Then re-apply: `just backend migrate`.
Expected: down drops everything; re-apply succeeds. (Re-applying is required so the DB is left migrated for later tasks.)

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/000022_subscriptions_schema.up.sql backend/migrations/000022_subscriptions_schema.down.sql
git commit -m "feat(tra-947): migration 000022 — subscriptions schema + org_is_entitled()

Adds partners, subscription_plans (immutable price-points), subscriptions
(dormant until Stripe), and org entitlement/billing columns. Seeds standard
tiers. org_is_entitled() SECURITY DEFINER encodes manual-OR-subscription
entitlement for pre-org-context middleware use.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Organization model + store reads

**Files:**
- Modify: `backend/internal/models/organization/organization.go`
- Modify: `backend/internal/storage/organizations.go`
- Test: `backend/internal/storage/organizations_integration_test.go` (create if absent)

- [ ] **Step 1: Write the failing integration test**

Create or append to `backend/internal/storage/organizations_integration_test.go`:

```go
//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestGetOrganizationByID_EntitlementFields(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

	// A fresh org from the harness is entitled (enabled=true, expires NULL).
	org, err := store.CreateOrganization(ctx, "Entitlement Co", "entitlement-co")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	got, err := store.GetOrganizationByID(ctx, org.ID)
	if err != nil {
		t.Fatalf("get org: %v", err)
	}
	if !got.SubscriptionEnabled {
		t.Errorf("SubscriptionEnabled = false, want true")
	}
	if got.SubscriptionExpiresAt != nil {
		t.Errorf("SubscriptionExpiresAt = %v, want nil", got.SubscriptionExpiresAt)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `just backend test-integration ./internal/storage/ -run TestGetOrganizationByID_EntitlementFields`
Expected: FAIL — compile error (`got.SubscriptionEnabled` undefined).

- [ ] **Step 3: Add fields to the Organization model**

In `backend/internal/models/organization/organization.go`, add to the `Organization` struct (after `IsActive`):

```go
	IsActive   bool                   `json:"is_active"`
	// TRA-947 lite entitlement (manual gate). Plan/billing refs are present in
	// schema but not surfaced on this struct until TRA-135/TRA-198 need them.
	SubscriptionEnabled   bool       `json:"subscription_enabled"`
	SubscriptionExpiresAt *time.Time `json:"subscription_expires_at,omitempty"`
```

- [ ] **Step 4: Extend the store SELECTs**

In `backend/internal/storage/organizations.go`, update the three read queries that scan an `Organization` (`GetOrganizationByID`, `GetOrganizationByIdentifier`, and the `RETURNING` in `CreateOrganization` and `UpdateOrganization`). For each, add the two columns to the column list and the matching `Scan` targets. Example for `GetOrganizationByID`:

```go
	query := `
		SELECT id, name, identifier, metadata,
		       valid_from, valid_to, is_active, created_at, updated_at,
		       subscription_enabled, subscription_expires_at
		FROM trakrf.organizations
		WHERE id = $1 AND deleted_at IS NULL
	`
	var org organization.Organization
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&org.ID, &org.Name, &org.Identifier, &org.Metadata,
		&org.ValidFrom, &org.ValidTo, &org.IsActive, &org.CreatedAt, &org.UpdatedAt,
		&org.SubscriptionEnabled, &org.SubscriptionExpiresAt)
```

Apply the same two-column addition + two scan targets to `GetOrganizationByIdentifier`, `CreateOrganization` (its `RETURNING`), and `UpdateOrganization` (its `RETURNING`). Keep column order identical across all four so the scan targets line up.

- [ ] **Step 5: Run the test to verify it passes**

Run: `just backend test-integration ./internal/storage/ -run TestGetOrganizationByID_EntitlementFields`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models/organization/organization.go backend/internal/storage/organizations.go backend/internal/storage/organizations_integration_test.go
git commit -m "feat(tra-947): surface org entitlement columns on Organization reads

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Entitlement storage method `OrgIsEntitled`

**Files:**
- Modify: `backend/internal/storage/organizations.go`
- Test: `backend/internal/storage/organizations_integration_test.go`

- [ ] **Step 1: Write the failing truth-table test**

Append to `backend/internal/storage/organizations_integration_test.go`:

```go
func TestOrgIsEntitled_TruthTable(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool() // superuser pool for fixture mutation

	org, err := store.CreateOrganization(ctx, "Gate Co", "gate-co")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	cases := []struct {
		name    string
		enabled bool
		expires string // SQL expression for subscription_expires_at
		want    bool
	}{
		{"enabled, no expiry", true, "NULL", true},
		{"enabled, future expiry", true, "now() + interval '1 day'", true},
		{"enabled, past expiry (lapsed)", true, "now() - interval '1 day'", false},
		{"disabled", false, "NULL", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := pool.Exec(ctx,
				"UPDATE trakrf.organizations SET subscription_enabled=$1, subscription_expires_at="+c.expires+" WHERE id=$2",
				c.enabled, org.ID)
			if err != nil {
				t.Fatalf("update fixture: %v", err)
			}
			got, err := store.OrgIsEntitled(ctx, org.ID)
			if err != nil {
				t.Fatalf("OrgIsEntitled: %v", err)
			}
			if got != c.want {
				t.Errorf("OrgIsEntitled = %v, want %v", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `just backend test-integration ./internal/storage/ -run TestOrgIsEntitled_TruthTable`
Expected: FAIL — `store.OrgIsEntitled` undefined.

- [ ] **Step 3: Implement `OrgIsEntitled`**

Append to `backend/internal/storage/organizations.go`:

```go
// OrgIsEntitled reports whether the org may perform paid mutations. It calls
// the SECURITY DEFINER function trakrf.org_is_entitled, which encodes the full
// formula (manual booleans OR active subscription) and runs with no org context
// required — so this is safe to call from request middleware before WithOrgTx.
func (s *Storage) OrgIsEntitled(ctx context.Context, orgID int) (bool, error) {
	var entitled bool
	err := s.pool.QueryRow(ctx, `SELECT trakrf.org_is_entitled($1)`, orgID).Scan(&entitled)
	if err != nil {
		return false, fmt.Errorf("failed to check org entitlement: %w", err)
	}
	return entitled, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `just backend test-integration ./internal/storage/ -run TestOrgIsEntitled_TruthTable`
Expected: PASS (all four sub-cases).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/storage/organizations.go backend/internal/storage/organizations_integration_test.go
git commit -m "feat(tra-947): add Storage.OrgIsEntitled via org_is_entitled() SECURITY DEFINER fn

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: 402 error type + responder

**Files:**
- Modify: `backend/internal/models/errors/errors.go`
- Modify: `backend/internal/util/httputil/auth_error.go`
- Test: `backend/internal/util/httputil/auth_error_test.go` (create if absent)

- [ ] **Step 1: Write the failing test**

Create `backend/internal/util/httputil/auth_error_test.go` (or append):

```go
package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRespond402PaymentRequired(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/assets", nil)

	Respond402PaymentRequired(w, r, "Organization subscription is not active", "req-123")

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", w.Code)
	}
	var body struct {
		Error struct {
			Type   string `json:"type"`
			Title  string `json:"title"`
			Status int    `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Error.Type != "payment_required" {
		t.Errorf("type = %q, want payment_required", body.Error.Type)
	}
	if body.Error.Title != "Payment required" {
		t.Errorf("title = %q, want 'Payment required'", body.Error.Title)
	}
	if body.Error.Status != 402 {
		t.Errorf("status field = %d, want 402", body.Error.Status)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `just backend test ./internal/util/httputil/ -run TestRespond402PaymentRequired`
Expected: FAIL — `Respond402PaymentRequired` undefined.

- [ ] **Step 3: Add the error type + title + enum tag**

In `backend/internal/models/errors/errors.go`:

Add the constant to the `const (...)` block:
```go
	ErrMissingOrgContext ErrorType = "missing_org_context"
	ErrPaymentRequired   ErrorType = "payment_required"
```

Add the case to `TitleForType` (before the final `return "Error"`):
```go
	case ErrPaymentRequired:
		return "Payment required"
```

Add `payment_required` to the `enums:` list in the `ErrorEnvelope.Type` struct tag (append after `missing_org_context`):
```go
	Type      string       `json:"type" example:"validation_error" enums:"validation_error,bad_request,unauthorized,forbidden,not_found,conflict,rate_limited,internal_error,method_not_allowed,unsupported_media_type,missing_org_context,payment_required" extensions:"x-extensible-enum=true"`
```

- [ ] **Step 4: Add the responder**

In `backend/internal/util/httputil/auth_error.go`, append:

```go
// Respond402PaymentRequired writes a normalized 402 for a not-entitled org
// (TRA-947 subscriptions lite). Distinct type/title from 401 (auth) and 403
// (RBAC) so the frontend branches to a renew/contact prompt rather than a
// login or permission prompt.
func Respond402PaymentRequired(w http.ResponseWriter, r *http.Request, detail, requestID string) {
	WriteJSONError(w, r, http.StatusPaymentRequired, apierrors.ErrPaymentRequired,
		detail, requestID)
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `just backend test ./internal/util/httputil/ -run TestRespond402PaymentRequired`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models/errors/errors.go backend/internal/util/httputil/auth_error.go backend/internal/util/httputil/auth_error_test.go
git commit -m "feat(tra-947): add 402 payment_required error type + Respond402PaymentRequired

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: `SubscriptionRequired` middleware

**Files:**
- Create: `backend/internal/middleware/subscription.go`
- Test: `backend/internal/middleware/subscription_test.go`

- [ ] **Step 1: Write the failing unit test**

Create `backend/internal/middleware/subscription_test.go`:

```go
package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeChecker struct {
	entitled bool
	err      error
	called   bool
}

func (f *fakeChecker) OrgIsEntitled(ctx context.Context, orgID int) (bool, error) {
	f.called = true
	return f.entitled, f.err
}

// withAPIKeyOrg returns a request carrying an API-key principal for orgID so
// GetRequestOrgID resolves. Uses the same context key the auth chain sets.
func withAPIKeyOrg(r *http.Request, orgID int) *http.Request {
	return r.WithContext(SetAPIKeyPrincipal(r.Context(), &APIKeyPrincipal{OrgID: orgID}))
}

func TestSubscriptionRequired_GetAlwaysPasses(t *testing.T) {
	chk := &fakeChecker{entitled: false}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := SubscriptionRequired(chk)(next)

	r := withAPIKeyOrg(httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil), 42)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Errorf("GET status = %d, want 200 (reads stay open)", w.Code)
	}
	if chk.called {
		t.Errorf("entitlement checked on a GET; should short-circuit on method")
	}
}

func TestSubscriptionRequired_EntitledMutationPasses(t *testing.T) {
	chk := &fakeChecker{entitled: true}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) })
	h := SubscriptionRequired(chk)(next)

	r := withAPIKeyOrg(httptest.NewRequest(http.MethodPost, "/api/v1/assets", nil), 42)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != 201 {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestSubscriptionRequired_NotEntitledMutation402(t *testing.T) {
	chk := &fakeChecker{entitled: false}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) })
	h := SubscriptionRequired(chk)(next)

	r := withAPIKeyOrg(httptest.NewRequest(http.MethodPost, "/api/v1/assets", nil), 42)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want 402", w.Code)
	}
}

func TestSubscriptionRequired_NoOrgContextPassesThrough(t *testing.T) {
	chk := &fakeChecker{entitled: false}
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; w.WriteHeader(401) })
	h := SubscriptionRequired(chk)(next)

	// No principal/claims set -> GetRequestOrgID errors -> pass through so the
	// auth layer (not the gate) produces the 401.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/assets", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !reached {
		t.Errorf("handler not reached; gate should defer to auth layer when no org context")
	}
}
```

Note: confirm the exact API-key principal setter name (`SetAPIKeyPrincipal` + `APIKeyPrincipal`) by reading `internal/middleware/apikey.go`; adjust the test helper to the real exported symbols if they differ.

- [ ] **Step 2: Run the test to verify it fails**

Run: `just backend test ./internal/middleware/ -run TestSubscriptionRequired`
Expected: FAIL — `SubscriptionRequired` undefined.

- [ ] **Step 3: Implement the middleware**

Create `backend/internal/middleware/subscription.go`:

```go
package middleware

import (
	"context"
	"net/http"

	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// EntitlementChecker reports whether an org may perform paid mutations.
// Satisfied by *storage.Storage (OrgIsEntitled).
type EntitlementChecker interface {
	OrgIsEntitled(ctx context.Context, orgID int) (bool, error)
}

// isMutation reports whether the method writes. Reads stay open regardless of
// entitlement (TRA-946: lapsed orgs keep read-only visibility).
func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// SubscriptionRequired gates paid mutations behind org entitlement (TRA-947).
// Apply it to route groups / routes that carry paid mutations. It:
//   - passes through all non-mutating methods (GET/HEAD/OPTIONS),
//   - passes through when no org context is resolvable (lets the auth layer 401),
//   - rejects a not-entitled mutation with 402 before the handler runs.
func SubscriptionRequired(checker EntitlementChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutation(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			orgID, err := GetRequestOrgID(r)
			if err != nil {
				// No org context — defer to the auth layer's 401.
				next.ServeHTTP(w, r)
				return
			}
			entitled, err := checker.OrgIsEntitled(r.Context(), orgID)
			if err != nil {
				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					apierrors.ErrInternal, "Failed to verify subscription entitlement",
					GetRequestID(r.Context()))
				return
			}
			if !entitled {
				httputil.Respond402PaymentRequired(w, r,
					"Organization subscription is not active or has expired",
					GetRequestID(r.Context()))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `just backend test ./internal/middleware/ -run TestSubscriptionRequired`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/middleware/subscription.go backend/internal/middleware/subscription_test.go
git commit -m "feat(tra-947): SubscriptionRequired middleware (402 on not-entitled mutations)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Wire the gate into the router

The gate goes on (a) the public write group at group level, and (b) the paid internal mutating routes via a middleware passed into the relevant handlers' `RegisterRoutes`. Must-stay-open routes (auth, org/user/API-key management, current-org switch, invitations, and output-device test/reset) are NOT gated.

**Files:**
- Modify: `backend/internal/cmd/serve/router.go`
- Modify: `backend/internal/handlers/scandevices/scandevices.go`
- Modify: `backend/internal/handlers/scanpoints/scanpoints.go`
- Modify: `backend/internal/handlers/outputdevices/outputdevices.go`
- Modify: `backend/internal/handlers/assets/assets.go`
- Test: `backend/internal/cmd/serve/router_entitlement_integration_test.go` (create)

- [ ] **Step 1: Gate the public write group**

In `backend/internal/cmd/serve/router.go`, in the public write group (the `r.Group` starting ~line 220), add the gate after `WriteAudit` so a denied write is still audited:

```go
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.WriteAudit)
		r.Use(middleware.SubscriptionRequired(store)) // TRA-947: 402 on not-entitled paid mutation
		r.Use(middleware.RateLimit(rl, allowTestRateLimitBypass))
```

(`store` is the `*storage.Storage` already in scope and passed to `EitherAuth`; it now also satisfies `middleware.EntitlementChecker`.)

- [ ] **Step 2: Thread the gate into the paid internal handlers' RegisterRoutes**

The internal session group (the `r.Group` starting ~line 138) mounts paid handlers alongside must-stay-open ones, so gate per-handler rather than at group level. Build the gate once before that group and pass it in.

In `router.go`, just before the internal session `r.Group(func(r chi.Router) {` (~line 138), add:

```go
	// TRA-947: paid-mutation entitlement gate, threaded into the handlers that
	// register paid mutating routes in the internal session group.
	paidGate := middleware.SubscriptionRequired(store)
```

Then update the four paid handler mounts inside that group to pass `paidGate`:

```go
		assetsHandler.RegisterRoutes(r, paidGate)
		...
		scanDevicesHandler.RegisterRoutes(r, paidGate)
		scanPointsHandler.RegisterRoutes(r, paidGate)
		...
		outputDevicesHandler.RegisterRoutes(r, paidGate)
```

Leave `orgsHandler`, `usersHandler`, `inventoryHandler`, `reportsHandler`, `lookupHandler`, `readstreamHandler`, and `RegisterMeRoutes` unchanged (must-stay-open or read-only).

- [ ] **Step 3: Apply the gate to scan-devices paid routes**

In `backend/internal/handlers/scandevices/scandevices.go`, change `RegisterRoutes` to accept the gate and wrap only the mutating routes:

```go
func (h *Handler) RegisterRoutes(r chi.Router, paidGate func(http.Handler) http.Handler) {
	// reads — open
	r.Get("/api/v1/scan-devices", h.List)
	r.Get("/api/v1/scan-devices/{scan_device_id}", h.Get)
	// paid mutations — gated
	r.With(paidGate).Post("/api/v1/scan-devices", h.Create)
	r.With(paidGate).Patch("/api/v1/scan-devices/{scan_device_id}", h.Update)
	r.With(paidGate).Delete("/api/v1/scan-devices/{scan_device_id}", h.Delete)
	r.With(paidGate).Post("/api/v1/scan-devices/{scan_device_id}/scan-points", h.CreatePoint)
}
```

Match the existing route paths/handler names already in the file (read the current `RegisterRoutes` body first and keep every existing route; only add `.With(paidGate)` to the POST/PATCH/DELETE ones and add the `paidGate` param). Add `"net/http"` to imports if not present.

- [ ] **Step 4: Apply the gate to scan-points paid routes**

In `backend/internal/handlers/scanpoints/scanpoints.go`, same transformation:

```go
func (h *Handler) RegisterRoutes(r chi.Router, paidGate func(http.Handler) http.Handler) {
	// keep existing GET routes as-is
	r.With(paidGate).Patch("/api/v1/scan-points/{scan_point_id}", h.Update)
	r.With(paidGate).Delete("/api/v1/scan-points/{scan_point_id}", h.Delete)
}
```

- [ ] **Step 5: Apply the gate to output-devices paid routes (NOT test/reset)**

In `backend/internal/handlers/outputdevices/outputdevices.go`, gate Create/Update/Delete but leave Test/Reset open (operational, per the route map):

```go
func (h *Handler) RegisterRoutes(r chi.Router, paidGate func(http.Handler) http.Handler) {
	// keep existing GET routes as-is
	r.With(paidGate).Post("/api/v1/output-devices", h.Create)
	r.With(paidGate).Patch("/api/v1/output-devices/{output_device_id}", h.Update)
	r.With(paidGate).Delete("/api/v1/output-devices/{output_device_id}", h.Delete)
	// Test / Reset stay ungated (operational, must-stay-open)
	r.Post("/api/v1/output-devices/{output_device_id}/test", h.Test)
	r.Post("/api/v1/output-devices/{output_device_id}/reset", h.Reset)
}
```

- [ ] **Step 6: Apply the gate to the assets bulk route**

In `backend/internal/handlers/assets/assets.go`, `RegisterRoutes` (~line 1002) registers the internal asset routes (bulk upload + GETs; CRUD lives in the public write group). Add the gate param and wrap only the bulk POST:

```go
func (handler *Handler) RegisterRoutes(r chi.Router, paidGate func(http.Handler) http.Handler) {
	// keep existing GET routes as-is
	r.With(paidGate).Post("/api/v1/assets/bulk", handler.UploadCSV)
}
```

Keep every existing route in the current body; only add the param and `.With(paidGate)` on the bulk POST.

- [ ] **Step 7: Build to catch signature mismatches**

Run: `cd backend && go build ./... 2>&1 | grep -v "openapi.internal.json\|frontend/dist" ; echo done`
Expected: no errors other than the two known embed warnings. Fix any `RegisterRoutes` call-site arity errors the compiler reports (only the four paid handlers changed; all call sites are in `router.go`).

- [ ] **Step 8: Write the failing enforcement integration test**

Create `backend/internal/cmd/serve/router_entitlement_integration_test.go`. Model it on an existing router/handler integration test (read one first, e.g. an existing `*_integration_test.go` under `internal/handlers/` or `internal/cmd/serve/`, to reuse the harness that builds the router with a real store and mints session/api-key auth). The test must assert, against a built router with a real DB:

```
// pseudocode of required assertions — implement with the existing test harness
// 1. Lapsed org (subscription_enabled=false): 
//      POST /api/v1/assets            -> 402, body.error.type == "payment_required"
//      GET  /api/v1/assets            -> NOT 402 (200/empty)
//      POST /api/v1/output-devices/{id}/test -> NOT 402 (operational, ungated)
//      POST /api/v1/users/me/current-org     -> NOT 402 (must-stay-open)
// 2. Entitled org (default):
//      POST /api/v1/assets            -> NOT 402 (201/400-validation, anything but 402)
```

Use `store.Pool().Exec(ctx, "UPDATE trakrf.organizations SET subscription_enabled=false WHERE id=$1", orgID)` to lapse the fixture org. Assert on status codes and `error.type`.

- [ ] **Step 9: Run the enforcement test to verify it fails, then passes**

Run: `just backend test-integration ./internal/cmd/serve/ -run Entitlement`
Expected: after Steps 1–6 it should PASS. If it fails, the gate placement or a route classification is wrong — fix before committing. (Write the test first against the unmodified expectation if practicing strict TDD; given the gate is already wired by this point, treat a failure as a wiring bug.)

- [ ] **Step 10: Commit**

```bash
git add backend/internal/cmd/serve/router.go backend/internal/handlers/scandevices/scandevices.go backend/internal/handlers/scanpoints/scanpoints.go backend/internal/handlers/outputdevices/outputdevices.go backend/internal/handlers/assets/assets.go backend/internal/cmd/serve/router_entitlement_integration_test.go
git commit -m "feat(tra-947): enforce entitlement 402 on paid mutations (public + internal)

Gates the public write group and the paid internal scan/output/bulk routes;
leaves auth, org/user/API-key mgmt, current-org switch, and output test/reset open.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Entitlement in the session payload

**Files:**
- Modify: `backend/internal/models/organization/organization.go`
- Modify: `backend/internal/services/orgs/service.go`
- Test: `backend/internal/services/orgs/profile_integration_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/services/orgs/profile_integration_test.go` (read the file first to reuse its existing setup helpers and the pattern for creating a user + org + membership):

```go
func TestGetUserProfile_IncludesEntitlement(t *testing.T) {
	// ... use the file's existing helper to create a user, an org, and admin
	// membership, returning (svc *Service, userID int, orgID int) ...
	profile, err := svc.GetUserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserProfile: %v", err)
	}
	if profile.CurrentOrg == nil {
		t.Fatalf("CurrentOrg is nil")
	}
	if !profile.CurrentOrg.IsEntitled {
		t.Errorf("IsEntitled = false, want true for a fresh org")
	}
	if !profile.CurrentOrg.SubscriptionEnabled {
		t.Errorf("SubscriptionEnabled = false, want true")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `just backend test-integration ./internal/services/orgs/ -run TestGetUserProfile_IncludesEntitlement`
Expected: FAIL — `IsEntitled` undefined on `UserOrgWithRole`.

- [ ] **Step 3: Add fields to `UserOrgWithRole`**

In `backend/internal/models/organization/organization.go`, add to `UserOrgWithRole` (after `Role`):

```go
	Role       string `json:"role"`
	// TRA-947 entitlement: is_entitled is computed server-side; the raw fields
	// are surfaced for display (renew prompts / trial countdown).
	IsEntitled            bool       `json:"is_entitled"`
	SubscriptionEnabled   bool       `json:"subscription_enabled"`
	SubscriptionExpiresAt *time.Time `json:"subscription_expires_at,omitempty"`
```

- [ ] **Step 4: Populate them in `GetUserProfile`**

In `backend/internal/services/orgs/service.go`, inside `GetUserProfile`, where `cur := &organization.UserOrgWithRole{...}` is built (the block that also sets `cur.Identifier` from `GetOrganizationByID`), reuse that same `full` org lookup and add the entitlement call:

```go
				cur := &organization.UserOrgWithRole{
					ID:   org.ID,
					Name: org.Name,
					Role: string(role),
				}
				if full, ferr := s.storage.GetOrganizationByID(ctx, currentOrgID); ferr == nil && full != nil {
					cur.Identifier = full.Identifier
					cur.SubscriptionEnabled = full.SubscriptionEnabled
					cur.SubscriptionExpiresAt = full.SubscriptionExpiresAt
				}
				if entitled, eerr := s.storage.OrgIsEntitled(ctx, currentOrgID); eerr == nil {
					cur.IsEntitled = entitled
				}
				profile.CurrentOrg = cur
```

(Best-effort, matching the existing `Identifier` pattern — a lookup miss leaves zero values rather than failing `/me`.)

- [ ] **Step 5: Run the test to verify it passes**

Run: `just backend test-integration ./internal/services/orgs/ -run TestGetUserProfile_IncludesEntitlement`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/models/organization/organization.go backend/internal/services/orgs/service.go backend/internal/services/orgs/profile_integration_test.go
git commit -m "feat(tra-947): include is_entitled + raw subscription fields in session payload

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: 1-month trial on self-service signup only

**Files:**
- Modify: `backend/internal/services/auth/auth.go`
- Test: `backend/internal/services/auth/auth_integration_test.go` (append; read the file for existing signup-test helpers)

- [ ] **Step 1: Write the failing test**

Append to the auth integration test file. Two assertions: self-service signup sets a ~1-month expiry; invitation signup leaves it NULL.

```go
func TestSignup_SelfService_SetsOneMonthTrial(t *testing.T) {
	// ... build the auth Service with a real store (reuse existing helper) ...
	resp, err := svc.Signup(ctx, auth.SignupRequest{
		Email: "wild@example.com", Password: "password123", OrgName: "Wild Co",
	}, "ua", "1.2.3.4", hashStub, jwtStub)
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	// Look up the created org's expiry via the store's superuser pool.
	var expires *time.Time
	err = store.Pool().QueryRow(ctx,
		`SELECT subscription_expires_at FROM trakrf.organizations
		 WHERE id = (SELECT org_id FROM trakrf.org_users WHERE user_id=$1 LIMIT 1)`,
		resp.User.ID).Scan(&expires)
	if err != nil {
		t.Fatalf("query expiry: %v", err)
	}
	if expires == nil {
		t.Fatalf("self-service signup expiry is NULL, want ~1 month out")
	}
	d := time.Until(*expires)
	if d < 27*24*time.Hour || d > 32*24*time.Hour {
		t.Errorf("expiry in %v, want ~1 month", d)
	}
}
```

(Use the same `hashStub`/`jwtStub` signatures the existing signup tests use; read them from the file.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `just backend test-integration ./internal/services/auth/ -run TestSignup_SelfService_SetsOneMonthTrial`
Expected: FAIL — expiry is NULL (no trial set yet).

- [ ] **Step 3: Set the trial on the standard signup org INSERT**

In `backend/internal/services/auth/auth.go`, in `Signup`'s standard branch, change the org INSERT (currently `INSERT INTO trakrf.organizations (name, identifier) VALUES ($1, $2)`) to set the trial expiry:

```go
	orgQuery := `
		INSERT INTO trakrf.organizations (name, identifier, subscription_expires_at)
		VALUES ($1, $2, now() + interval '1 month')
		RETURNING id, name, identifier, metadata, valid_from, valid_to, is_active, created_at, updated_at
	`
```

Leave `signupWithInvitation` and `orgs.CreateOrgWithAdmin` untouched — invitation joins create no org, and explicit/internal creates stay perpetual (superadmin sets entitlement via TRA-949).

- [ ] **Step 4: Run the test to verify it passes**

Run: `just backend test-integration ./internal/services/auth/ -run TestSignup_SelfService_SetsOneMonthTrial`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/services/auth/auth.go backend/internal/services/auth/auth_integration_test.go
git commit -m "feat(tra-947): self-service signup starts a 1-month trial (internal creates stay perpetual)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Regenerate spec, full validation

**Files:**
- Modify: generated OpenAPI artifacts (via `just backend api-spec`)

- [ ] **Step 1: Regenerate the OpenAPI spec**

The 402 enum addition and any new annotations change the generated spec.
Run: `just backend api-spec`
Expected: regenerates `openapi.*` artifacts; `git status` shows the spec files changed (or no change if annotations didn't alter output).

- [ ] **Step 2: Run the RLS guard**

Run: `just backend check-rls-guard`
Expected: `✓ check-rls-guard: clean`. (No new offenders — `OrgIsEntitled` lives in `organizations.go`, which is not in the guard's RLS file list, and it intentionally uses the no-context SECURITY DEFINER function.)

- [ ] **Step 3: Run unit + integration suites**

Run: `just backend test ./internal/...`
Then: `just backend test-integration ./internal/...`
Expected: all pass. Investigate any failure before proceeding (do not claim success on partial runs).

- [ ] **Step 4: Build**

Run: `just backend build`
Expected: builds clean (this target generates the frontend/spec embeds, so the two earlier embed warnings should be gone).

- [ ] **Step 5: Commit any generated changes**

```bash
git add -A
git commit -m "chore(tra-947): regenerate OpenAPI spec for 402 payment_required

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

(If `git status` is clean after Step 1, skip this commit.)

---

## Done criteria

- Migration 000022 applies and reverses; all existing orgs entitled; standard tiers seeded; `org_is_entitled()` returns correct truth table.
- Paid mutations (assets/locations/inventory + scan-device/point/output config + asset bulk) return **402 `payment_required`** for a lapsed org on both public-API and SPA/internal routes; GETs and must-stay-open POSTs (auth, org/user/API-key mgmt, current-org switch, output test/reset) are unaffected.
- `/api/v1/users/me` carries `current_org.is_entitled` + raw `subscription_enabled` / `subscription_expires_at`.
- Self-service signup org expires ~1 month out; invitation and explicit/internal creates stay perpetual (NULL).
- `just backend validate`-level checks (api-lint, lint, test, build) green.

## Deferred (not this plan)

- Stripe sync / webhooks / checkout → TRA-135 (also wires the dormant subscription branch of `org_is_entitled`).
- Plan-limit enforcement + get-my-plan endpoint → TRA-198 remainder.
- Superadmin entitlement controls + create-time trial/paid choice → TRA-949.
- Three-state gating UX → TRA-948.
- Superadmin notification on self-service trial signup → TRA-967.
