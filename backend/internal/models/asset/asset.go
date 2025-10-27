package asset

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/org"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

type Asset struct {
	ID          int        `json:"id"`
	OrgID       int        `json:"org_id"`
	Org         *org.Org   `json:"org"`
	Identifier  string     `json:"identifier"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Description string     `json:"description"`
	ValidFrom   time.Time  `json:"valid_from"`
	ValidTo     time.Time  `json:"valid_to"`
	Metadata    any        `json:"metadata"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at"`
}

type CreateAssetRequest struct {
	OrgID       int       `json:"org_id" validate:"omitempty,min=1"`
	Identifier  string    `json:"identifier" validate:"required,min=1,max=255"`
	Name        string    `json:"name" validate:"required,min=1,max=255"`
	Type        string    `json:"type" validate:"oneof=person device asset inventory other"`
	Description string    `json:"description" validate:"omitempty,max=1024"`
	ValidFrom   time.Time `json:"valid_from"`
	ValidTo     time.Time `json:"valid_to"`
	Metadata    any       `json:"metadata"`
	IsActive    bool      `json:"is_active"`
}

type UpdateAssetRequest struct {
	OrgID       *int       `json:"org_id" validate:"omitempty,min=1"`
	Identifier  *string    `json:"identifier" validate:"omitempty,min=1,max=255"`
	Name        *string    `json:"name" validate:"omitempty,min=1,max=255"`
	Type        *string    `json:"type" validate:"omitempty,oneof=person device asset inventory other"`
	Description *string    `json:"description" validate:"omitempty,max=1024"`
	ValidFrom   *time.Time `json:"valid_from"`
	ValidTo     *time.Time `json:"valid_to"`
	Metadata    *any       `json:"metadata"`
	IsActive    *bool      `json:"is_active"`
}

type AssetListResponse struct {
	Data       []Asset           `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}
