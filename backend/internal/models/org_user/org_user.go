package org_user

import "time"

// TODO(TRA-94): Define proper request/response models for org_user CRUD operations
// These models are not used by auth endpoints (which query directly).

// OrgUser represents a user-organization relationship
type OrgUser struct {
	OrgID     int        `json:"org_id"`
	UserID    int        `json:"user_id"`
	Role      string     `json:"role"`
	Status    string     `json:"status"`
	Settings  any        `json:"settings"` // JSONB
	Metadata  any        `json:"metadata"` // JSONB
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	UserEmail string     `json:"user_email"`
	UserName  string     `json:"user_name"`
}

// CreateOrgUserRequest for POST /api/v1/org_users
type CreateOrgUserRequest struct {
	OrgID  int    `json:"org_id" validate:"required"`
	UserID int    `json:"user_id" validate:"required"`
	Role   string `json:"role" validate:"required,oneof=owner admin member"`
}

// UpdateOrgUserRequest for PUT /api/v1/org_users/:orgId/:userId
type UpdateOrgUserRequest struct {
	Role   *string `json:"role" validate:"omitempty,oneof=owner admin member"`
	Status *string `json:"status" validate:"omitempty,oneof=active inactive"`
}
