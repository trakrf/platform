package email

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/resend/resend-go/v2"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// reservedTestDomains are RFC 2606 / RFC 6761 addresses reserved for documentation
// and testing. No real user can own one, so we never attempt to send to them —
// this prevents e2e fixtures from burning Resend quota.
var reservedTestDomains = map[string]struct{}{
	"example.com": {},
	"example.net": {},
	"example.org": {},
}

var reservedTestSuffixes = []string{".test", ".invalid", ".example"}

func isReservedTestRecipient(addr string) bool {
	at := strings.LastIndex(addr, "@")
	if at < 0 || at == len(addr)-1 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(addr[at+1:]))
	if _, ok := reservedTestDomains[domain]; ok {
		return true
	}
	for _, s := range reservedTestSuffixes {
		if strings.HasSuffix(domain, s) {
			return true
		}
	}
	for d := range reservedTestDomains {
		if strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
}

// Client wraps the Resend email client
type Client struct {
	client *resend.Client
}

// NewClient creates a new email client using the RESEND_API_KEY environment variable
func NewClient() *Client {
	apiKey := os.Getenv("RESEND_API_KEY")
	return &Client{
		client: resend.NewClient(apiKey),
	}
}

// getEmailPrefix returns the appropriate email subject prefix based on APP_ENV.
// Production/empty returns "[TrakRF]", non-prod returns "[TrakRF Preview]" etc.
func getEmailPrefix() string {
	env := os.Getenv("APP_ENV")
	if env == "" || env == "production" || env == "prod" {
		return "[TrakRF]"
	}
	// Title case the environment name
	caser := cases.Title(language.English)
	return fmt.Sprintf("[TrakRF %s]", caser.String(env))
}

// OrgNotifyOverride returns a single-recipient override list for org-lifecycle
// superadmin notifications (self-service signup, internal create, delete) when
// ORG_CREATE_NOTIFY_ADDR is set, or nil to keep the default all-superadmins
// fan-out. It exists so a non-prod environment (e.g. preview) can point the
// e2e-churn notification firehose at a single operator instead of paging every
// superadmin. Empty/whitespace is treated as unset.
func OrgNotifyOverride() []string {
	if addr := strings.TrimSpace(os.Getenv("ORG_CREATE_NOTIFY_ADDR")); addr != "" {
		return []string{addr}
	}
	return nil
}

// getEnvironmentNotice returns an HTML notice for non-production environments.
// Returns empty string for production/empty.
func getEnvironmentNotice() string {
	env := os.Getenv("APP_ENV")
	if env == "" || env == "production" || env == "prod" {
		return ""
	}
	caser := cases.Title(language.English)
	return fmt.Sprintf(`<p style="color: #6b7280; font-size: 12px;">This email was sent from the %s environment.</p>`, caser.String(env))
}

// SendPasswordResetEmail sends a password reset email with a link containing the token.
// resetURL should be the base URL for the reset page (e.g., "https://app.trakrf.id/#reset-password")
func (c *Client) SendPasswordResetEmail(toEmail, resetURL, token string) error {
	fullResetURL := fmt.Sprintf("%s?token=%s", resetURL, token)

	if isReservedTestRecipient(toEmail) {
		log.Info().
			Str("to", toEmail).
			Str("kind", "password_reset").
			Str("app_env", os.Getenv("APP_ENV")).
			Msg("email send stubbed: reserved test-fixture recipient")
		return nil
	}

	_, err := c.client.Emails.Send(&resend.SendEmailRequest{
		From:    "TrakRF <noreply@trakrf.id>",
		To:      []string{toEmail},
		Subject: fmt.Sprintf("%s Reset your password", getEmailPrefix()),
		Html: fmt.Sprintf(`
			<h2>Reset your password</h2>
			<p>Click the link below to reset your TrakRF password. This link expires in 24 hours.</p>
			<p><a href="%s">Reset Password</a></p>
			<p>If you didn't request this, you can safely ignore this email.</p>
			%s
		`, fullResetURL, getEnvironmentNotice()),
	})

	if err != nil {
		return fmt.Errorf("failed to send password reset email: %w", err)
	}

	return nil
}

// SendInvitationEmail sends an organization invitation email.
// baseURL should be the frontend origin (e.g., "https://app.trakrf.id")
func (c *Client) SendInvitationEmail(toEmail, orgName, inviterName, role, token, baseURL string) error {
	acceptURL := fmt.Sprintf("%s/#accept-invite?token=%s", baseURL, token)

	if isReservedTestRecipient(toEmail) {
		log.Info().
			Str("to", toEmail).
			Str("kind", "invitation").
			Str("org", orgName).
			Str("app_env", os.Getenv("APP_ENV")).
			Msg("email send stubbed: reserved test-fixture recipient")
		return nil
	}

	_, err := c.client.Emails.Send(&resend.SendEmailRequest{
		From:    "TrakRF <noreply@trakrf.id>",
		To:      []string{toEmail},
		Subject: fmt.Sprintf("%s You've been invited to join %s", getEmailPrefix(), orgName),
		Html: fmt.Sprintf(`
			<h2>You've been invited to %s</h2>
			<p>%s has invited you to join %s as a %s on TrakRF.</p>
			<p><a href="%s">Accept Invitation</a></p>
			<p>This invitation expires in 7 days.</p>
			<p>If you don't have a TrakRF account yet, you'll be prompted to create one.</p>
			%s
		`, orgName, inviterName, orgName, role, acceptURL, getEnvironmentNotice()),
	})

	if err != nil {
		return fmt.Errorf("failed to send invitation email: %w", err)
	}

	return nil
}

// SendTrialSignupNotification alerts a superadmin that a brand-new user
// self-service-signed up and was put on a 1-month trial (TRA-967). It carries
// the new org's name + identifier, the signing-up user's email, and the trial
// expiry so an operator can reach out and qualify the account. trialExpiresAt
// may be nil defensively.
func (c *Client) SendTrialSignupNotification(toEmail, orgName, orgIdentifier, signupEmail string, trialExpiresAt *time.Time) error {
	if isReservedTestRecipient(toEmail) {
		log.Info().
			Str("to", toEmail).
			Str("kind", "trial_signup_notification").
			Str("org", orgName).
			Str("app_env", os.Getenv("APP_ENV")).
			Msg("email send stubbed: reserved test-fixture recipient")
		return nil
	}

	trialExpiry := "unknown"
	if trialExpiresAt != nil {
		trialExpiry = trialExpiresAt.UTC().Format("2006-01-02 15:04 UTC")
	}

	_, err := c.client.Emails.Send(&resend.SendEmailRequest{
		From:    "TrakRF <noreply@trakrf.id>",
		To:      []string{toEmail},
		Subject: fmt.Sprintf("%s New trial signup: %s", getEmailPrefix(), orgName),
		Html: fmt.Sprintf(`
			<h2>New self-service trial signup</h2>
			<p>A new user signed up and started a 1-month trial. Reach out to qualify the account.</p>
			<ul>
				<li><strong>Organization:</strong> %s (%s)</li>
				<li><strong>User email:</strong> %s</li>
				<li><strong>Trial expires:</strong> %s</li>
			</ul>
			%s
		`, orgName, orgIdentifier, signupEmail, trialExpiry, getEnvironmentNotice()),
	})

	if err != nil {
		return fmt.Errorf("failed to send trial signup notification: %w", err)
	}

	return nil
}

// SendOrgCreatedNotification alerts a superadmin that a new org was created via
// an internal/admin create (POST /api/v1/orgs), not the self-service signup
// path (that has its own SendTrialSignupNotification). It carries the org name +
// identifier, the creating user's email, and the entitlement window: a non-nil
// trialExpiresAt means a trial expiry, nil means perpetual (the default for
// internal creates). Part of tracking what drives signups (TRA-977).
func (c *Client) SendOrgCreatedNotification(toEmail, orgName, orgIdentifier, creatorEmail string, trialExpiresAt *time.Time) error {
	if isReservedTestRecipient(toEmail) {
		log.Info().
			Str("to", toEmail).
			Str("kind", "org_created_notification").
			Str("org", orgName).
			Str("app_env", os.Getenv("APP_ENV")).
			Msg("email send stubbed: reserved test-fixture recipient")
		return nil
	}

	entitlement := "perpetual (no expiry)"
	if trialExpiresAt != nil {
		entitlement = "trial, expires " + trialExpiresAt.UTC().Format("2006-01-02 15:04 UTC")
	}

	_, err := c.client.Emails.Send(&resend.SendEmailRequest{
		From:    "TrakRF <noreply@trakrf.id>",
		To:      []string{toEmail},
		Subject: fmt.Sprintf("%s New org created: %s", getEmailPrefix(), orgName),
		Html: fmt.Sprintf(`
			<h2>New organization created</h2>
			<p>A new organization was created. Follow up to track what's driving signups.</p>
			<ul>
				<li><strong>Organization:</strong> %s (%s)</li>
				<li><strong>Created by:</strong> %s</li>
				<li><strong>Entitlement:</strong> %s</li>
			</ul>
			%s
		`, orgName, orgIdentifier, creatorEmail, entitlement, getEnvironmentNotice()),
	})

	if err != nil {
		return fmt.Errorf("failed to send org created notification: %w", err)
	}

	return nil
}

// SendOrgDeletedNotification alerts a superadmin that an org was soft-deleted, so
// an operator can follow up on churn and run a postmortem on why they quit
// (TRA-977). It carries the org name + identifier (pre-mangle), the user who
// deleted it, and when.
func (c *Client) SendOrgDeletedNotification(toEmail, orgName, orgIdentifier, actorEmail string, deletedAt time.Time) error {
	if isReservedTestRecipient(toEmail) {
		log.Info().
			Str("to", toEmail).
			Str("kind", "org_deleted_notification").
			Str("org", orgName).
			Str("app_env", os.Getenv("APP_ENV")).
			Msg("email send stubbed: reserved test-fixture recipient")
		return nil
	}

	_, err := c.client.Emails.Send(&resend.SendEmailRequest{
		From:    "TrakRF <noreply@trakrf.id>",
		To:      []string{toEmail},
		Subject: fmt.Sprintf("%s Org deleted: %s", getEmailPrefix(), orgName),
		Html: fmt.Sprintf(`
			<h2>Organization deleted</h2>
			<p>An organization was deleted. Follow up for a churn postmortem — find out why they quit.</p>
			<ul>
				<li><strong>Organization:</strong> %s (%s)</li>
				<li><strong>Deleted by:</strong> %s</li>
				<li><strong>Deleted at:</strong> %s</li>
			</ul>
			%s
		`, orgName, orgIdentifier, actorEmail, deletedAt.UTC().Format("2006-01-02 15:04 UTC"), getEnvironmentNotice()),
	})

	if err != nil {
		return fmt.Errorf("failed to send org deleted notification: %w", err)
	}

	return nil
}
