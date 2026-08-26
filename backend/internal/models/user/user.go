package user

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

// User represents a user entity
type User struct {
	ID           int        `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	PasswordHash string     `json:"-"` // Never expose in JSON
	LastLoginAt  *time.Time `json:"last_login_at"`
	Settings     any        `json:"settings"` // JSONB
	Metadata     any        `json:"metadata"` // JSONB
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// RBAC fields
	IsSuperadmin bool `json:"is_superadmin"`
	LastOrgID    *int `json:"last_org_id,omitempty"`
	// MustChangePassword gates the app behind the change-password screen
	// (TRA-1135). It is set on accounts an operator provisioned with a
	// bootstrap password and cleared by any password write.
	MustChangePassword bool `json:"must_change_password"`
}

// CreateUserRequest is gone with POST /api/v1/users (TRA-1103). It carried a
// `password_hash` the storage layer stored verbatim, so the value became the
// bcrypt hash and could never verify against itself. Users are created by signup
// and the org invitation flow, which also establish org membership.

// UpdateUserRequest for PUT /api/v1/users/:id
type UpdateUserRequest struct {
	Name  *string `json:"name" validate:"omitempty,min=1,max=255"`
	Email *string `json:"email" validate:"omitempty,email"`
	// MustChangePassword is the superadmin-only forced-rotation toggle
	// (TRA-1135) — the onsite operator's tool for an account they just
	// provisioned with a bootstrap password. Nil leaves the flag untouched;
	// this endpoint is a partial update, and an unrelated rename must never
	// let a flagged user back into the app.
	MustChangePassword *bool `json:"must_change_password"`
}

// UserListResponse for GET /api/v1/users
type UserListResponse struct {
	Data       []User            `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}
