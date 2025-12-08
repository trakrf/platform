package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/models/org_user"
)

// ErrOrgUserNotFound is returned when a user is not a member of an org
var ErrOrgUserNotFound = fmt.Errorf("user is not a member of this organization")

// GetUserOrgRole returns the user's role in the specified organization
func (s *Storage) GetUserOrgRole(ctx context.Context, userID, orgID int) (models.OrgRole, error) {
	query := `
		SELECT role
		FROM trakrf.org_users
		WHERE user_id = $1 AND org_id = $2 AND deleted_at IS NULL
	`
	var role models.OrgRole
	err := s.pool.QueryRow(ctx, query, userID, orgID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrOrgUserNotFound
		}
		return "", fmt.Errorf("failed to get user org role: %w", err)
	}
	return role, nil
}

// IsUserSuperadmin checks if the user has the superadmin flag set
func (s *Storage) IsUserSuperadmin(ctx context.Context, userID int) (bool, error) {
	query := `
		SELECT is_superadmin
		FROM trakrf.users
		WHERE id = $1 AND deleted_at IS NULL
	`
	var isSuperadmin bool
	err := s.pool.QueryRow(ctx, query, userID).Scan(&isSuperadmin)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check superadmin status: %w", err)
	}
	return isSuperadmin, nil
}

// CountOrgAdmins returns the number of admins in an organization
func (s *Storage) CountOrgAdmins(ctx context.Context, orgID int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM trakrf.org_users
		WHERE org_id = $1 AND role = 'admin' AND deleted_at IS NULL
	`
	var count int
	err := s.pool.QueryRow(ctx, query, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count org admins: %w", err)
	}
	return count, nil
}

// ListOrgUsers retrieves a paginated list of users in an organization.
func (s *Storage) ListOrgUsers(ctx context.Context, orgID int, limit, offset int) ([]org_user.OrgUser, int, error) {
	// TODO: Implement with new schema
	return nil, 0, fmt.Errorf("not implemented: org_user list requires schema migration")
}

// GetOrgUser retrieves a single org-user relationship.
func (s *Storage) GetOrgUser(ctx context.Context, orgID, userID int) (*org_user.OrgUser, error) {
	// TODO: Implement
	return nil, fmt.Errorf("not implemented: org_user get requires schema migration")
}

// CreateOrgUser creates a new org-user relationship.
func (s *Storage) CreateOrgUser(ctx context.Context, request org_user.CreateOrgUserRequest) (*org_user.OrgUser, error) {
	// TODO: Implement
	return nil, fmt.Errorf("not implemented: org_user create requires schema migration")
}

// UpdateOrgUser updates an org-user relationship.
func (s *Storage) UpdateOrgUser(ctx context.Context, orgID, userID int, request org_user.UpdateOrgUserRequest) (*org_user.OrgUser, error) {
	// TODO: Implement
	return nil, fmt.Errorf("not implemented: org_user update requires schema migration")
}

// SoftDeleteOrgUser marks an org-user relationship as deleted.
func (s *Storage) SoftDeleteOrgUser(ctx context.Context, orgID, userID int) error {
	// TODO: Implement
	return fmt.Errorf("not implemented: org_user delete requires schema migration")
}
