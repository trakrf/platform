package email

import (
	"fmt"
	"os"

	"github.com/resend/resend-go/v2"
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

// SendPasswordResetEmail sends a password reset email with a link containing the token
func (c *Client) SendPasswordResetEmail(toEmail, token string) error {
	resetURL := fmt.Sprintf("https://app.trakrf.id/#reset-password?token=%s", token)

	_, err := c.client.Emails.Send(&resend.SendEmailRequest{
		From:    "TrakRF <noreply@trakrf.id>",
		To:      []string{toEmail},
		Subject: "Reset your TrakRF password",
		Html: fmt.Sprintf(`
			<h2>Reset your password</h2>
			<p>Click the link below to reset your TrakRF password. This link expires in 24 hours.</p>
			<p><a href="%s">Reset Password</a></p>
			<p>If you didn't request this, you can safely ignore this email.</p>
		`, resetURL),
	})

	if err != nil {
		return fmt.Errorf("failed to send password reset email: %w", err)
	}

	return nil
}
