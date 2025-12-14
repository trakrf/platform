package orgs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/models/organization"
)

const invitationExpiryDays = 7

// CreateInvitation creates an invitation and sends an email
// baseURL is the frontend origin for building the accept link (e.g., "https://app.trakrf.id")
func (s *Service) CreateInvitation(ctx context.Context, orgID int, req organization.CreateInvitationRequest, inviterUserID int, baseURL string) (*organization.CreateInvitationResponse, error) {
	// Check if email is already a member
	isMember, err := s.storage.IsEmailMember(ctx, orgID, req.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to check membership: %w", err)
	}
	if isMember {
		return nil, fmt.Errorf("already_member")
	}

	// Check for pending invitation
	hasPending, err := s.storage.HasPendingInvitation(ctx, orgID, req.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to check pending invitations: %w", err)
	}
	if hasPending {
		return nil, fmt.Errorf("already_pending")
	}

	// Generate token (32 random bytes -> 64-char hex)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	rawToken := hex.EncodeToString(tokenBytes)

	// Hash token for storage
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	expiresAt := time.Now().Add(invitationExpiryDays * 24 * time.Hour)

	// Store invitation
	role := models.OrgRole(req.Role)
	inviteID, err := s.storage.CreateInvitation(ctx, orgID, req.Email, role, tokenHash, inviterUserID, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create invitation: %w", err)
	}

	// Get org and inviter info for email
	org, err := s.storage.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	inviter, err := s.storage.GetUserByID(ctx, inviterUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get inviter: %w", err)
	}

	// Send invitation email (with raw token, not hash)
	if s.emailClient != nil {
		if err := s.emailClient.SendInvitationEmail(req.Email, org.Name, inviter.Name, req.Role, rawToken, baseURL); err != nil {
			// Log error but don't fail the invitation creation
			// The admin can resend if needed
			fmt.Printf("warning: failed to send invitation email: %v\n", err)
		}
	}

	return &organization.CreateInvitationResponse{
		ID:        inviteID,
		Email:     req.Email,
		Role:      req.Role,
		ExpiresAt: expiresAt,
	}, nil
}

// ListPendingInvitations returns all pending invitations for an org
func (s *Service) ListPendingInvitations(ctx context.Context, orgID int) ([]organization.Invitation, error) {
	return s.storage.ListPendingInvitations(ctx, orgID)
}

// CancelInvitation cancels a pending invitation
func (s *Service) CancelInvitation(ctx context.Context, inviteID int) error {
	return s.storage.CancelInvitation(ctx, inviteID)
}

// ResendInvitation generates a new token and resends the email, returns new expiry
// baseURL is the frontend origin for building the accept link (e.g., "https://app.trakrf.id")
func (s *Service) ResendInvitation(ctx context.Context, inviteID, orgID int, baseURL string) (time.Time, error) {
	// Get the invitation
	inv, err := s.storage.GetInvitationByID(ctx, inviteID)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get invitation: %w", err)
	}
	if inv == nil {
		return time.Time{}, fmt.Errorf("invitation not found")
	}

	// Generate new token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return time.Time{}, fmt.Errorf("failed to generate token: %w", err)
	}
	rawToken := hex.EncodeToString(tokenBytes)

	// Hash token for storage
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	newExpiry := time.Now().Add(invitationExpiryDays * 24 * time.Hour)

	// Update token and expiry
	if err := s.storage.UpdateInvitationToken(ctx, inviteID, tokenHash, newExpiry); err != nil {
		return time.Time{}, err
	}

	// Get org info for email
	org, err := s.storage.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get organization: %w", err)
	}

	// Get inviter info - use original inviter if available, else use "Team Admin"
	inviterName := "Team Admin"
	if inv.InvitedBy != nil {
		inviterName = inv.InvitedBy.Name
	}

	// Send email with new token
	// Log error but don't fail - admin can retry if needed (matches CreateInvitation behavior)
	if s.emailClient != nil {
		if err := s.emailClient.SendInvitationEmail(inv.Email, org.Name, inviterName, inv.Role, rawToken, baseURL); err != nil {
			fmt.Printf("warning: failed to send invitation email: %v\n", err)
		}
	}

	return newExpiry, nil
}

// GetInvitationOrgID returns the org_id for an invitation (for authorization)
func (s *Service) GetInvitationOrgID(ctx context.Context, inviteID int) (int, error) {
	return s.storage.GetInvitationOrgID(ctx, inviteID)
}
