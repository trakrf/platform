package org_user

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models"
)

// OrgUser represents a user-organization relationship
type OrgUser struct {
	OrgID     int            `json:"org_id"`
	UserID    int            `json:"user_id"`
	Role      models.OrgRole `json:"role"`
	Status    string         `json:"status"`
	Settings  any            `json:"settings"` // JSONB
	Metadata  any            `json:"metadata"` // JSONB
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt *time.Time     `json:"deleted_at,omitempty"`
	UserEmail string         `json:"user_email"`
	UserName  string         `json:"user_name"`
}

// CreateOrgUserRequest for POST /api/v1/org_users
type CreateOrgUserRequest struct {
	OrgID  int            `json:"org_id" validate:"required"`
	UserID int            `json:"user_id" validate:"required"`
	Role   models.OrgRole `json:"role" validate:"required"`
}

// UpdateOrgUserRequest for PUT /api/v1/org_users/:orgId/:userId
type UpdateOrgUserRequest struct {
	Role   *models.OrgRole `json:"role" validate:"omitempty"`
	Status *string         `json:"status" validate:"omitempty,oneof=active inactive"`
}
