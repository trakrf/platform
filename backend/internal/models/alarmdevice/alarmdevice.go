// Package alarmdevice defines the alarm output device model (TRA-903): a
// physical device (demo: Shelly Gen4 relay) the geofence engine fires when a
// registered asset trips a boundary scan point.
package alarmdevice

import "time"

// TypeShellyGen4 is the only supported alarm device type today.
const TypeShellyGen4 = "shelly_gen4"

// AlarmDevice is an alarm output device row.
type AlarmDevice struct {
	ID         int        `json:"id"`
	OrgID      int        `json:"org_id"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	BaseURL    string     `json:"base_url"`
	SwitchID   int        `json:"switch_id"`
	LocationID *int       `json:"location_id,omitempty"`
	IsActive   bool       `json:"is_active"`
	Metadata   any        `json:"metadata"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}

// CreateAlarmDeviceRequest is the create payload. type/switch_id/is_active/
// metadata default server-side when omitted.
type CreateAlarmDeviceRequest struct {
	Name       string         `json:"name" validate:"required,min=1,max=255"`
	Type       string         `json:"type,omitempty" validate:"omitempty,oneof=shelly_gen4"`
	BaseURL    string         `json:"base_url" validate:"required,url,max=255"`
	SwitchID   *int           `json:"switch_id,omitempty" validate:"omitempty,min=0"`
	LocationID *int           `json:"location_id,omitempty"`
	IsActive   *bool          `json:"is_active,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// UpdateAlarmDeviceRequest is a partial update; nil fields are left unchanged.
type UpdateAlarmDeviceRequest struct {
	Name       *string         `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Type       *string         `json:"type,omitempty" validate:"omitempty,oneof=shelly_gen4"`
	BaseURL    *string         `json:"base_url,omitempty" validate:"omitempty,url,max=255"`
	SwitchID   *int            `json:"switch_id,omitempty" validate:"omitempty,min=0"`
	LocationID *int            `json:"location_id,omitempty"`
	IsActive   *bool           `json:"is_active,omitempty"`
	Metadata   *map[string]any `json:"metadata,omitempty"`
}

// AlarmDeviceResponse is the single-resource envelope.
type AlarmDeviceResponse struct {
	Data AlarmDevice `json:"data"`
}
