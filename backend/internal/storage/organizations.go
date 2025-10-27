package storage

import (
	"context"
	"fmt"

	"github.com/trakrf/platform/backend/internal/models/organization"
)

// TODO(TRA-94): Implement full CRUD operations for organizations
// These storage methods are not used by auth endpoints (which query directly).
// Proper implementation deferred to follow-up task.

// ListOrganizations retrieves a paginated list of active organizations ordered by creation date.
func (s *Storage) ListOrganizations(ctx context.Context, limit, offset int) ([]organization.Organization, int, error) {
	// TODO: Implement with new schema fields (id, name, domain, metadata, valid_from, valid_to, is_active, created_at, updated_at, deleted_at)
	return nil, 0, fmt.Errorf("not implemented: organization list requires schema migration")
}

// GetOrganizationByID retrieves a single organization by its ID.
func (s *Storage) GetOrganizationByID(ctx context.Context, id int) (*organization.Organization, error) {
	// TODO: Implement with new schema fields
	return nil, fmt.Errorf("not implemented: organization get requires schema migration")
}

// CreateOrganization inserts a new organization with the provided details.
func (s *Storage) CreateOrganization(ctx context.Context, request organization.CreateOrganizationRequest) (*organization.Organization, error) {
	// TODO: Implement with new schema (only name and domain, no billing/subscription fields)
	return nil, fmt.Errorf("not implemented: organization create requires schema migration")
}

// UpdateOrganization updates an organization with the provided partial fields.
func (s *Storage) UpdateOrganization(ctx context.Context, id int, request organization.UpdateOrganizationRequest) (*organization.Organization, error) {
	// TODO: Implement with new schema fields
	return nil, fmt.Errorf("not implemented: organization update requires schema migration")
}

// SoftDeleteOrganization marks an organization as deleted by setting deleted_at timestamp.
func (s *Storage) SoftDeleteOrganization(ctx context.Context, id int) error {
	// TODO: Implement
	return fmt.Errorf("not implemented: organization delete requires schema migration")
}
