package location

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/org"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

type Location struct {
	ID                 int         `json:"id"`
	Name               string      `json:"name"`
	OrgID              int         `json:"org_id"`
	Org                *org.Org    `json:"org,omitempty"`
	Identifier         string      `json:"identifier" validate:"required,min=1,max=255"`
	Path               string      `json:"path"`
	Depth              int         `json:"depth"`
	ParentLocationID   *int        `json:"parent_location_id"`
	Parent             *Location   `json:"parent,omitempty"`
	Children           []Location  `json:"children,omitempty"`
	Ancestors          []Location  `json:"ancestors,omitempty"`
	ValidFrom          time.Time   `json:"valid_from"`
	ValidTo            *time.Time  `json:"valid_to,omitempty"`
	IsActive           bool        `json:"is_active"`
	Description        string      `json:"description"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          *time.Time  `json:"updated_at,omitempty"`
	DeletedAt          *time.Time  `json:"deleted_at,omitempty"`
}

type LocationWithRelations struct {
	Location
	Children  []Location `json:"children"`
	Ancestors []Location `json:"ancestors"`
}

type CreateLocationRequest struct {
	OrgID              int        `json:"org_id" validate:"omitempty,min=1"`
	Name               string     `json:"name" validate:"required,min=1,max=255"`
	Identifier         string     `json:"identifier" validate:"required,min=1,max=255"`
	ParentLocationID   *int       `json:"parent_location_id" validate:"omitempty,min=1"`
	Description        string     `json:"description" validate:"omitempty,max=1024"`
	ValidFrom          time.Time  `json:"valid_from"`
	ValidTo            *time.Time `json:"valid_to,omitempty"`
	IsActive           bool       `json:"is_active"`
}

type UpdateLocationRequest struct {
	OrgID              *int       `json:"org_id" validate:"omitempty,min=1"`
	Name               *string    `json:"name" validate:"omitempty,min=1,max=255"`
	Identifier         *string    `json:"identifier" validate:"omitempty,min=1,max=255"`
	ParentLocationID   *int       `json:"parent_location_id" validate:"omitempty,min=1"`
	Description        *string    `json:"description" validate:"omitempty,max=1024"`
	ValidFrom          *time.Time `json:"valid_from"`
	ValidTo            *time.Time `json:"valid_to"`
	IsActive           *bool      `json:"is_active"`
}

type LocationListResponse struct {
	Data       []Location          `json:"data"`
	Pagination shared.Pagination   `json:"pagination"`
}
