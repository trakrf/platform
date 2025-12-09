package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/models/organization"
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

// AddUserToOrg adds a user to an organization with the specified role.
func (s *Storage) AddUserToOrg(ctx context.Context, orgID, userID int, role models.OrgRole) error {
	query := `
		INSERT INTO trakrf.org_users (org_id, user_id, role)
		VALUES ($1, $2, $3)
	`
	_, err := s.pool.Exec(ctx, query, orgID, userID, role)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("user is already a member of this organization")
		}
		return fmt.Errorf("failed to add user to org: %w", err)
	}
	return nil
}

// ListOrgMembers returns all members of an organization with user details
func (s *Storage) ListOrgMembers(ctx context.Context, orgID int) ([]organization.OrgMember, error) {
	query := `
		SELECT ou.user_id, u.name, u.email, ou.role, ou.created_at
		FROM trakrf.org_users ou
		JOIN trakrf.users u ON u.id = ou.user_id
		WHERE ou.org_id = $1 AND ou.deleted_at IS NULL AND u.deleted_at IS NULL
		ORDER BY ou.created_at ASC
	`
	rows, err := s.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list org members: %w", err)
	}
	defer rows.Close()

	var members []organization.OrgMember
	for rows.Next() {
		var m organization.OrgMember
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}

// UpdateMemberRole updates a member's role in an organization
func (s *Storage) UpdateMemberRole(ctx context.Context, orgID, userID int, role models.OrgRole) error {
	query := `
		UPDATE trakrf.org_users
		SET role = $3, updated_at = NOW()
		WHERE org_id = $1 AND user_id = $2 AND deleted_at IS NULL
	`
	result, err := s.pool.Exec(ctx, query, orgID, userID, role)
	if err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrOrgUserNotFound
	}
	return nil
}

// RemoveMember removes a user from an organization (hard delete)
func (s *Storage) RemoveMember(ctx context.Context, orgID, userID int) error {
	query := `
		DELETE FROM trakrf.org_users
		WHERE org_id = $1 AND user_id = $2
	`
	result, err := s.pool.Exec(ctx, query, orgID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrOrgUserNotFound
	}
	return nil
}
