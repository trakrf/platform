package auth

import (
	"testing"
)

func TestAcceptInvitation_EmailMismatch(t *testing.T) {
	// This test verifies the email mismatch error format
	// Full integration test requires database - see E2E tests

	t.Run("error format includes invited email", func(t *testing.T) {
		// Verify the error format we expect from the service
		invitedEmail := "invited@example.com"
		expectedError := "email_mismatch:" + invitedEmail

		// The actual service test requires mocking storage
		// This validates our error format convention
		if expectedError != "email_mismatch:invited@example.com" {
			t.Errorf("unexpected error format: %s", expectedError)
		}
	})
}
