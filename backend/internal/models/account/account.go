package account

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

// Account represents an account entity
type Account struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	Domain           string    `json:"domain"`
	Status           string    `json:"status"`
	SubscriptionTier string    `json:"subscription_tier"`
	MaxUsers         int       `json:"max_users"`
	MaxStorageGB     int       `json:"max_storage_gb"`
	Settings         any       `json:"settings"` // JSONB
	Metadata         any       `json:"metadata"` // JSONB
	BillingEmail     string    `json:"billing_email"`
	TechnicalEmail   *string   `json:"technical_email"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// CreateAccountRequest for POST /api/v1/accounts
type CreateAccountRequest struct {
	Name             string  `json:"name" validate:"required,min=1,max=255"`
	Domain           string  `json:"domain" validate:"required,hostname"`
	BillingEmail     string  `json:"billing_email" validate:"required,email"`
	TechnicalEmail   *string `json:"technical_email" validate:"omitempty,email"`
	SubscriptionTier string  `json:"subscription_tier" validate:"omitempty,oneof=free basic premium god-mode"`
	MaxUsers         *int    `json:"max_users" validate:"omitempty,min=1"`
	MaxStorageGB     *int    `json:"max_storage_gb" validate:"omitempty,min=1"`
}

// UpdateAccountRequest for PUT /api/v1/accounts/:id
type UpdateAccountRequest struct {
	Name           *string `json:"name" validate:"omitempty,min=1,max=255"`
	BillingEmail   *string `json:"billing_email" validate:"omitempty,email"`
	TechnicalEmail *string `json:"technical_email" validate:"omitempty,email"`
	Status         *string `json:"status" validate:"omitempty,oneof=active inactive suspended"`
	MaxUsers       *int    `json:"max_users" validate:"omitempty,min=1"`
	MaxStorageGB   *int    `json:"max_storage_gb" validate:"omitempty,min=1"`
}

// AccountListResponse for GET /api/v1/accounts
type AccountListResponse struct {
	Data       []Account         `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}
