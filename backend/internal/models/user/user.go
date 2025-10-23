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
}

// CreateUserRequest for POST /api/v1/users
type CreateUserRequest struct {
	Email        string `json:"email" validate:"required,email"`
	Name         string `json:"name" validate:"required,min=1,max=255"`
	PasswordHash string `json:"password_hash" validate:"required,min=8"` // Temporary, Phase 5 will hash
}

// UpdateUserRequest for PUT /api/v1/users/:id
type UpdateUserRequest struct {
	Name  *string `json:"name" validate:"omitempty,min=1,max=255"`
	Email *string `json:"email" validate:"omitempty,email"`
}

// UserListResponse for GET /api/v1/users
type UserListResponse struct {
	Data       []User            `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}
