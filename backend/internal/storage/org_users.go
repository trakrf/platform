package storage

import (
	"context"
	"fmt"

	"github.com/trakrf/platform/backend/internal/models/org_user"
)

// TODO(TRA-94): Implement full CRUD operations for org_users
// These storage methods are not used by auth endpoints (which query directly).
// The auth service queries trakrf.org_users directly for user-org relationships.
// Proper implementation of these CRUD methods deferred to follow-up task.

// ListOrgUsers retrieves a paginated list of users in an organization.
func (s *Storage) ListOrgUsers(ctx context.Context, orgID int, limit, offset int) ([]org_user.OrgUser, int, error) {
	// TODO: Implement with new schema (org_id instead of account_id, no status field)
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
