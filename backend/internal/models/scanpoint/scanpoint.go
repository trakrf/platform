// Package scanpoint models capture points (antennas / gateway zones) belonging
// to a scan device, and the requests for their internal CRUD endpoints.
// Every device has at least scan_point 1, uniformly (TRA-899).
package scanpoint

import "time"

type ScanPoint struct {
	ID           int        `json:"id"`
	OrgID        int        `json:"org_id"`
	ScanDeviceID int        `json:"scan_device_id"`
	LocationID   *int       `json:"location_id,omitempty"`
	Name         string     `json:"name"`
	AntennaPort  *int       `json:"antenna_port,omitempty"`
	Description  string     `json:"description"`
	Metadata     any        `json:"metadata"`
	ValidFrom    time.Time  `json:"valid_from"`
	ValidTo      *time.Time `json:"valid_to,omitempty"`
	IsActive     bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

type CreateScanPointRequest struct {
	Name       string `json:"name" validate:"required,min=1,max=255" example:"Antenna 1"`
	LocationID *int   `json:"location_id,omitempty" validate:"omitempty,min=1"`
	// AntennaPort is the per-antenna correlation key reads are matched on
	// (TRA-956); it defaults to 1 when omitted. Unique per device among live rows.
	AntennaPort *int           `json:"antenna_port,omitempty" validate:"omitempty,min=1" example:"1"`
	Description *string        `json:"description,omitempty" validate:"omitempty,max=1024"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
}

type UpdateScanPointRequest struct {
	Name        *string         `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	LocationID  *int            `json:"location_id,omitempty" validate:"omitempty,min=1"`
	AntennaPort *int            `json:"antenna_port,omitempty" validate:"omitempty,min=1"`
	Description *string         `json:"description,omitempty" validate:"omitempty,max=1024"`
	Metadata    *map[string]any `json:"metadata,omitempty"`
	IsActive    *bool           `json:"is_active,omitempty"`
	// ClearLocationID is set by the PATCH handler on an explicit JSON null for
	// location_id, requesting a column-clear (detach the zone). Not decoded directly.
	ClearLocationID bool `json:"-"`
}

type ScanPointResponse struct {
	Data ScanPoint `json:"data"`
}
