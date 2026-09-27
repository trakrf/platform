//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestGetOrganizationByID_EntitlementFields(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

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

func TestOrgIsEntitled_TruthTable(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)

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
		{"enabled, within default grace", true, "now() - interval '2 days'", true},
		{"enabled, past default grace", true, "now() - interval '4 days'", false},
		{"disabled", false, "NULL", false},
		{"disabled during grace is immediate cutoff", false, "now() - interval '2 days'", false},
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

// An active trakrf.subscriptions row entitles an org on its own, independent of
// the manual subscription_enabled flag (TRA-947's OR contract, which Stripe —
// TRA-135 — will rely on). Grace applies to current_period_end the same way it
// applies to the manual expiry.
func TestOrgIsEntitled_ActiveSubscriptionRow(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)

	org, err := store.CreateOrganization(ctx, "Stripe Co", "stripe-co")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE trakrf.organizations SET subscription_enabled=false WHERE id=$1`, org.ID); err != nil {
		t.Fatalf("disable manual entitlement: %v", err)
	}
	var planID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO trakrf.subscription_plans (name) VALUES ('Test Plan') RETURNING id`).Scan(&planID); err != nil {
		t.Fatalf("insert plan: %v", err)
	}

	cases := []struct {
		name      string
		status    string
		periodEnd string // SQL expression for current_period_end
		want      bool
	}{
		{"active, open-ended", "active", "NULL", true},
		{"active, future period end", "active", "now() + interval '10 days'", true},
		{"active, period end within grace", "active", "now() - interval '2 days'", true},
		{"active, period end past grace", "active", "now() - interval '4 days'", false},
		{"canceled", "canceled", "now() + interval '10 days'", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, `DELETE FROM trakrf.subscriptions WHERE org_id=$1`, org.ID); err != nil {
				t.Fatalf("clear subscriptions: %v", err)
			}
			if _, err := pool.Exec(ctx,
				`INSERT INTO trakrf.subscriptions (org_id, plan_id, status, current_period_end)
				 VALUES ($1, $2, $3::trakrf.subscription_status, `+c.periodEnd+`)`,
				org.ID, planID, c.status); err != nil {
				t.Fatalf("insert subscription: %v", err)
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

// app.subscription_grace_period is an operator-set GUC. A valid value is
// honoured; a malformed one must fall back to the default rather than make every
// entitlement check (and the MQTT topic list) error out.
func TestOrgIsEntitled_GracePeriodSetting(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)

	org, err := store.CreateOrganization(ctx, "Grace Co", "grace-co")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE trakrf.organizations SET subscription_expires_at = now() - interval '2 days' WHERE id=$1`,
		org.ID); err != nil {
		t.Fatalf("set expiry: %v", err)
	}

	cases := []struct {
		name    string
		setting string
		want    bool
	}{
		{"shorter grace is honoured", "1 day", false},
		{"longer grace is honoured", "5 days", true},
		{"malformed falls back to 3 days", "not-an-interval", true},
		{"negative is treated as no grace", "-1 day", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if _, err := tx.Exec(ctx, `SELECT set_config('app.subscription_grace_period', $1, true)`, c.setting); err != nil {
				t.Fatalf("set grace: %v", err)
			}
			var got bool
			if err := tx.QueryRow(ctx, `SELECT trakrf.org_is_entitled($1)`, org.ID).Scan(&got); err != nil {
				t.Fatalf("org_is_entitled with %q: %v", c.setting, err)
			}
			if got != c.want {
				t.Errorf("org_is_entitled with grace %q = %v, want %v", c.setting, got, c.want)
			}
		})
	}
}
