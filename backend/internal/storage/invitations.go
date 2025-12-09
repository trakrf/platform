package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/models/organization"
)

// CreateInvitation creates a new invitation with hashed token
func (s *Storage) CreateInvitation(ctx context.Context, orgID int, email string, role models.OrgRole, tokenHash string, invitedBy int, expiresAt time.Time) (int, error) {
	query := `
		INSERT INTO trakrf.org_invitations (org_id, email, role, token, invited_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	var id int
	err := s.pool.QueryRow(ctx, query, orgID, email, role, tokenHash, invitedBy, expiresAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create invitation: %w", err)
	}
	return id, nil
}

// ListPendingInvitations returns non-expired, non-cancelled, non-accepted invitations
func (s *Storage) ListPendingInvitations(ctx context.Context, orgID int) ([]organization.Invitation, error) {
	query := `
		SELECT i.id, i.email, i.role, i.expires_at, i.created_at,
		       u.id, u.name
		FROM trakrf.org_invitations i
		LEFT JOIN trakrf.users u ON u.id = i.invited_by
		WHERE i.org_id = $1
		  AND i.expires_at > NOW()
		  AND i.cancelled_at IS NULL
		  AND i.accepted_at IS NULL
		ORDER BY i.created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list invitations: %w", err)
	}
	defer rows.Close()

	var invitations []organization.Invitation
	for rows.Next() {
		var inv organization.Invitation
		var inviterID *int
		var inviterName *string
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedAt,
			&inviterID, &inviterName); err != nil {
			return nil, fmt.Errorf("failed to scan invitation: %w", err)
		}
		if inviterID != nil && inviterName != nil {
			inv.InvitedBy = &organization.InvitedByUser{ID: *inviterID, Name: *inviterName}
		}
		invitations = append(invitations, inv)
	}
	return invitations, nil
}

// GetInvitationByID returns an invitation by ID
func (s *Storage) GetInvitationByID(ctx context.Context, inviteID int) (*organization.Invitation, error) {
	query := `
		SELECT id, org_id, email, role, expires_at, cancelled_at, accepted_at, created_at
		FROM trakrf.org_invitations
		WHERE id = $1
	`
	var inv organization.Invitation
	var orgID int
	var cancelledAt, acceptedAt *time.Time
	err := s.pool.QueryRow(ctx, query, inviteID).Scan(
		&inv.ID, &orgID, &inv.Email, &inv.Role, &inv.ExpiresAt, &cancelledAt, &acceptedAt, &inv.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get invitation: %w", err)
	}
	return &inv, nil
}

// CancelInvitation sets cancelled_at timestamp
func (s *Storage) CancelInvitation(ctx context.Context, inviteID int) error {
	query := `
		UPDATE trakrf.org_invitations
		SET cancelled_at = NOW()
		WHERE id = $1 AND cancelled_at IS NULL AND accepted_at IS NULL
	`
	result, err := s.pool.Exec(ctx, query, inviteID)
	if err != nil {
		return fmt.Errorf("failed to cancel invitation: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("invitation not found or already cancelled/accepted")
	}
	return nil
}

// UpdateInvitationToken updates token and expiry for resend
func (s *Storage) UpdateInvitationToken(ctx context.Context, inviteID int, newTokenHash string, newExpiry time.Time) error {
	query := `
		UPDATE trakrf.org_invitations
		SET token = $2, expires_at = $3
		WHERE id = $1 AND cancelled_at IS NULL AND accepted_at IS NULL
	`
	result, err := s.pool.Exec(ctx, query, inviteID, newTokenHash, newExpiry)
	if err != nil {
		return fmt.Errorf("failed to update invitation token: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("invitation not found or already cancelled/accepted")
	}
	return nil
}

// IsEmailMember checks if email is already a member of org
func (s *Storage) IsEmailMember(ctx context.Context, orgID int, email string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM trakrf.org_users ou
			JOIN trakrf.users u ON u.id = ou.user_id
			WHERE ou.org_id = $1 AND u.email = $2 AND ou.deleted_at IS NULL AND u.deleted_at IS NULL
		)
	`
	var exists bool
	err := s.pool.QueryRow(ctx, query, orgID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check email membership: %w", err)
	}
	return exists, nil
}

// HasPendingInvitation checks if there's an active invitation for email
func (s *Storage) HasPendingInvitation(ctx context.Context, orgID int, email string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM trakrf.org_invitations
			WHERE org_id = $1 AND email = $2
			  AND expires_at > NOW()
			  AND cancelled_at IS NULL
			  AND accepted_at IS NULL
		)
	`
	var exists bool
	err := s.pool.QueryRow(ctx, query, orgID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check pending invitation: %w", err)
	}
	return exists, nil
}

// GetInvitationOrgID returns the org_id for an invitation (for authorization)
func (s *Storage) GetInvitationOrgID(ctx context.Context, inviteID int) (int, error) {
	query := `SELECT org_id FROM trakrf.org_invitations WHERE id = $1`
	var orgID int
	err := s.pool.QueryRow(ctx, query, inviteID).Scan(&orgID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, fmt.Errorf("invitation not found")
		}
		return 0, fmt.Errorf("failed to get invitation org: %w", err)
	}
	return orgID, nil
}
