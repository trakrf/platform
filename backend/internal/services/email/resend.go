package email

import (
	"fmt"
	"os"

	"github.com/resend/resend-go/v2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

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
