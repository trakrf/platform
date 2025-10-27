package organization

import "time"

// Organization represents an application customer identity and tenant root.
// This model matches the schema defined in database/migrations/000002_organizations.up.sql
type Organization struct {
	ID         int                    `json:"id"`
	Name       string                 `json:"name"`
	Identifier string                 `json:"identifier"`
	IsPersonal bool                   `json:"is_personal"`
	Metadata   map[string]interface{} `json:"metadata"`
	ValidFrom  time.Time              `json:"valid_from"`
	ValidTo    *time.Time             `json:"valid_to,omitempty"`
	IsActive   bool                   `json:"is_active"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	DeletedAt  *time.Time             `json:"deleted_at,omitempty"`
}

// TODO(TRA-94): Define proper request/response models for organization CRUD operations
// These request types are placeholders for the storage layer stubs

// CreateOrganizationRequest for POST /api/v1/organizations
type CreateOrganizationRequest struct {
	Name       string `json:"name" validate:"required,min=1,max=255"`
	Identifier string `json:"identifier" validate:"required"`
}

// UpdateOrganizationRequest for PUT /api/v1/organizations/:id
type UpdateOrganizationRequest struct {
	Name *string `json:"name" validate:"omitempty,min=1,max=255"`
}
