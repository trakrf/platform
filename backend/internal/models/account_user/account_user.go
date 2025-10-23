package account_user

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

// AccountUser represents an account_users junction table entry
type AccountUser struct {
	AccountID   int        `json:"account_id"`
	UserID      int        `json:"user_id"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at"`
	Settings    any        `json:"settings"` // JSONB
	Metadata    any        `json:"metadata"` // JSONB
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// User details from JOIN
	UserEmail string `json:"user_email,omitempty"`
	UserName  string `json:"user_name,omitempty"`
}

// AddUserToAccountRequest for POST /api/v1/accounts/:account_id/users
type AddUserToAccountRequest struct {
	UserID int    `json:"user_id" validate:"required"`
	Role   string `json:"role" validate:"required,oneof=owner admin member readonly"`
	Status string `json:"status" validate:"omitempty,oneof=active inactive suspended invited"`
}

// UpdateAccountUserRequest for PUT /api/v1/accounts/:account_id/users/:user_id
type UpdateAccountUserRequest struct {
	Role   *string `json:"role" validate:"omitempty,oneof=owner admin member readonly"`
	Status *string `json:"status" validate:"omitempty,oneof=active inactive suspended invited"`
}

// AccountUserListResponse for GET /api/v1/accounts/:account_id/users
type AccountUserListResponse struct {
	Data       []AccountUser     `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}
