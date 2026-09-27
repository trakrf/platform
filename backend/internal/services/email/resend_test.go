package email

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestOrgNotifyOverride(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want []string
	}{
		{"unset", "", nil},
		{"whitespace only", "   ", nil},
		{"single address", "ops@trakrf.id", []string{"ops@trakrf.id"}},
		{"trimmed", "  ops@trakrf.id  ", []string{"ops@trakrf.id"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.addr == "" {
				os.Unsetenv("ORG_CREATE_NOTIFY_ADDR")
			} else {
				t.Setenv("ORG_CREATE_NOTIFY_ADDR", tt.addr)
			}
			got := OrgNotifyOverride()
			if len(got) != len(tt.want) {
				t.Fatalf("OrgNotifyOverride() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("OrgNotifyOverride()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGetEmailPrefix(t *testing.T) {
	tests := []struct {
		name     string
		appEnv   string
		expected string
	}{
		{"empty env", "", "[TrakRF]"},
		{"production", "production", "[TrakRF]"},
		{"prod", "prod", "[TrakRF]"},
		{"preview", "preview", "[TrakRF Preview]"},
		{"staging", "staging", "[TrakRF Staging]"},
		{"dev", "dev", "[TrakRF Dev]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("APP_ENV", tt.appEnv)
			defer os.Unsetenv("APP_ENV")

			result := getEmailPrefix()
			if result != tt.expected {
				t.Errorf("getEmailPrefix() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetEnvironmentNotice(t *testing.T) {
	tests := []struct {
		name          string
		appEnv        string
		shouldBeEmpty bool
		contains      string
	}{
		{"empty env", "", true, ""},
		{"production", "production", true, ""},
		{"prod", "prod", true, ""},
		{"preview", "preview", false, "Preview environment"},
		{"staging", "staging", false, "Staging environment"},
		{"dev", "dev", false, "Dev environment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("APP_ENV", tt.appEnv)
			defer os.Unsetenv("APP_ENV")

			result := getEnvironmentNotice()
			if tt.shouldBeEmpty && result != "" {
				t.Errorf("getEnvironmentNotice() = %q, want empty", result)
			}
			if !tt.shouldBeEmpty && result == "" {
				t.Errorf("getEnvironmentNotice() = empty, want non-empty containing %q", tt.contains)
			}
			if !tt.shouldBeEmpty && !strings.Contains(result, tt.contains) {
				t.Errorf("getEnvironmentNotice() = %q, want containing %q", result, tt.contains)
			}
		})
	}
}

func TestIsReservedTestRecipient(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected bool
	}{
		{"example.com", "fixture@example.com", true},
		{"example.net", "fixture@example.net", true},
		{"example.org", "fixture@example.org", true},
		{".test TLD", "fixture@foo.test", true},
		{".invalid TLD", "fixture@foo.invalid", true},
		{".example TLD", "fixture@foo.example", true},
		{"case-insensitive domain", "Fixture@Example.COM", true},
		{"subdomain of example.com", "fixture@mail.example.com", true},
		{"trakrf.id", "user@trakrf.id", false},
		{"gmail", "user@gmail.com", false},
		{"gmail alias", "miks2u+t2@gmail.com", false},
		{"customer domain", "user@acme.co", false},
		{"empty", "", false},
		{"no at-sign", "not-an-email", false},
		{"trailing whitespace", "fixture@example.com  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReservedTestRecipient(tt.addr); got != tt.expected {
				t.Errorf("isReservedTestRecipient(%q) = %v, want %v", tt.addr, got, tt.expected)
			}
		})
	}
}

func TestSendInvitationEmail_StubsReservedDomain(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "invalid-key-should-never-be-used")
	c := NewClient()

	if err := c.SendInvitationEmail(
		"fixture@example.com",
		"Test Org",
		"Inviter Name",
		"member",
		"token-xyz",
		"https://app.preview.trakrf.id",
	); err != nil {
		t.Fatalf("expected nil error for reserved recipient, got %v", err)
	}
}

func TestSendPasswordResetEmail_StubsReservedDomain(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "invalid-key-should-never-be-used")
	c := NewClient()

	if err := c.SendPasswordResetEmail(
		"fixture@example.com",
		"https://app.preview.trakrf.id/#reset-password",
		"token-xyz",
	); err != nil {
		t.Fatalf("expected nil error for reserved recipient, got %v", err)
	}
}

// TRA-967: the superadmin trial-signup notification must stub reserved test
// recipients (so e2e/integration runs never burn Resend quota), and must
// tolerate a nil trial expiry without panicking.
func TestSendTrialSignupNotification_StubsReservedDomain(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "invalid-key-should-never-be-used")
	c := NewClient()

	expires := time.Now().Add(30 * 24 * time.Hour)
	if err := c.SendTrialSignupNotification(
		"admin@example.com",
		"Acme Co",
		"acme-co",
		"newuser@example.com",
		&expires,
	); err != nil {
		t.Fatalf("expected nil error for reserved recipient, got %v", err)
	}

	// A nil expiry (defensive) must not panic.
	if err := c.SendTrialSignupNotification(
		"admin@example.com",
		"Acme Co",
		"acme-co",
		"newuser@example.com",
		nil,
	); err != nil {
		t.Fatalf("expected nil error for nil expiry, got %v", err)
	}
}

// TRA-977: the generic org-created notification must stub reserved recipients
// and handle both a perpetual org (nil expiry) and a trial org (non-nil expiry).
func TestSendOrgCreatedNotification_StubsReservedDomain(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "invalid-key-should-never-be-used")
	c := NewClient()

	// Perpetual (internal create) — nil expiry.
	if err := c.SendOrgCreatedNotification(
		"admin@example.com",
		"Acme Co",
		"acme-co",
		"creator@example.com",
		nil,
	); err != nil {
		t.Fatalf("expected nil error for perpetual org, got %v", err)
	}

	// Trial — non-nil expiry.
	expires := time.Now().Add(30 * 24 * time.Hour)
	if err := c.SendOrgCreatedNotification(
		"admin@example.com",
		"Acme Co",
		"acme-co",
		"creator@example.com",
		&expires,
	); err != nil {
		t.Fatalf("expected nil error for trial org, got %v", err)
	}
}

// TRA-977: the org-deleted (churn) notification must stub reserved recipients.
func TestSendOrgDeletedNotification_StubsReservedDomain(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "invalid-key-should-never-be-used")
	c := NewClient()

	if err := c.SendOrgDeletedNotification(
		"admin@example.com",
		"Acme Co",
		"acme-co",
		"actor@example.com",
		time.Now(),
	); err != nil {
		t.Fatalf("expected nil error for reserved recipient, got %v", err)
	}
}

// TRA-958: the "your email address changed" notice goes to the OLD address —
// the one the account holder can still read if the new one was a typo, and the
// one an attacker does not control after a session hijack. Like every other
// send, a reserved test recipient must stub out rather than burn Resend quota.
func TestSendEmailChangedNotification_StubsReservedDomain(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "invalid-key-should-never-be-used")
	c := NewClient()

	if err := c.SendEmailChangedNotification(
		"fixture@example.com",
		"new-address@example.com",
	); err != nil {
		t.Fatalf("expected nil error for reserved recipient, got %v", err)
	}
}

func TestSubscriptionNoticeContent(t *testing.T) {
	expires := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	cutoff := expires.Add(72 * time.Hour)
	t.Setenv("APP_ENV", "production")

	cases := []struct {
		kind        string
		wantSubject string
		wantBody    string
	}{
		{"t_minus_14", "[TrakRF] Your TrakRF subscription for Acme <&> expires in 14 days", "Oct 10, 2026"},
		{"t_minus_3", "[TrakRF] Your TrakRF subscription for Acme <&> expires in 3 days", "Oct 10, 2026"},
		{"expired", "[TrakRF] Your TrakRF subscription for Acme <&> has expired", "Oct 13, 2026"},
		{"cutoff", "[TrakRF] TrakRF service for Acme <&> has stopped", "not being recorded"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			subject, html, err := subscriptionNoticeContent(c.kind, "Acme <&>", expires, cutoff)
			if err != nil {
				t.Fatalf("content: %v", err)
			}
			if subject != c.wantSubject {
				t.Errorf("subject = %q, want %q", subject, c.wantSubject)
			}
			if !strings.Contains(html, c.wantBody) {
				t.Errorf("body missing %q:\n%s", c.wantBody, html)
			}
			if strings.Contains(html, "Acme <&>") || !strings.Contains(html, "Acme &lt;&amp;&gt;") {
				t.Errorf("org name must be HTML-escaped in body:\n%s", html)
			}
		})
	}

	if _, _, err := subscriptionNoticeContent("bogus", "Acme", expires, cutoff); err == nil {
		t.Error("unknown kind must error")
	}
}

func TestSendSubscriptionNotice_StubsReservedDomain(t *testing.T) {
	c := &Client{} // nil resend client: a real send would panic
	if err := c.SendSubscriptionNotice("admin@example.com", "t_minus_3", "Acme",
		time.Now(), time.Now().Add(72*time.Hour)); err != nil {
		t.Fatalf("reserved recipient must be stubbed, got %v", err)
	}
}
