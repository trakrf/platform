// Package outputdevice defines the output device model (TRA-903): a
// physical device (demo: Shelly Gen4 relay) the geofence engine fires when a
// registered asset trips a boundary scan point.
package outputdevice

import (
	"encoding/json"
	"time"
)

// TypeShellyGen4 is the only supported output device type today.
const TypeShellyGen4 = "shelly_gen4"

// Fire-path transports (TRA-906).
const (
	TransportHTTP = "http" // local edge HTTP RPC (default)
	TransportMQTT = "mqtt" // publish to the shared broker (firewall-friendly)
)

// OutputDevice is an output device row.
type OutputDevice struct {
	ID           int        `json:"id"`
	OrgID        int        `json:"org_id"`
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	Transport    string     `json:"transport"`
	BaseURL      string     `json:"base_url"`
	SwitchID     int        `json:"switch_id"`
	CommandTopic *string    `json:"command_topic,omitempty"`
	LocationID   *int       `json:"location_id,omitempty"`
	IsActive     bool       `json:"is_active"`
	Metadata     any        `json:"metadata"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

// AutoOffSeconds returns the per-device auto-off duration in seconds from
// metadata.auto_off_seconds, or 0 when unset, zero, negative, or non-numeric.
// 0 means "stay on until manual reset" (the latch default). This mirrors the
// per-scan-point metadata tuning used for rssi_threshold (geofence engine).
// Metadata arrives as map[string]any from jsonb, so numbers are float64.
func (d OutputDevice) AutoOffSeconds() int {
	m, ok := d.Metadata.(map[string]any)
	if !ok {
		return 0
	}
	var sec int
	switch n := m["auto_off_seconds"].(type) {
	case float64:
		sec = int(n)
	case int:
		sec = n
	case int64:
		sec = int(n)
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0
		}
		sec = int(i)
	default:
		return 0
	}
	if sec < 0 {
		return 0
	}
	return sec
}

// CreateOutputDeviceRequest is the create payload. type/switch_id/is_active/
// metadata default server-side when omitted.
type CreateOutputDeviceRequest struct {
	Name      string `json:"name" validate:"required,min=1,max=255"`
	Type      string `json:"type,omitempty" validate:"omitempty,oneof=shelly_gen4"`
	Transport string `json:"transport,omitempty" validate:"omitempty,oneof=http mqtt"`
	// base_url is only meaningful for http transport; its required/URL-format
	// validation is transport-aware in the handler (TRA-928), not a struct tag,
	// so an mqtt device can omit or blank it without tripping a url validator.
	BaseURL      string         `json:"base_url,omitempty" validate:"omitempty,max=255"`
	SwitchID     *int           `json:"switch_id,omitempty" validate:"omitempty,min=0"`
	CommandTopic *string        `json:"command_topic,omitempty" validate:"omitempty,max=255"`
	LocationID   *int           `json:"location_id,omitempty"`
	IsActive     *bool          `json:"is_active,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// UpdateOutputDeviceRequest is a partial update; nil fields are left unchanged.
type UpdateOutputDeviceRequest struct {
	Name      *string `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Type      *string `json:"type,omitempty" validate:"omitempty,oneof=shelly_gen4"`
	Transport *string `json:"transport,omitempty" validate:"omitempty,oneof=http mqtt"`
	// base_url validation is transport-aware in the handler (TRA-928): a non-nil
	// pointer to "" (what the form sends for mqtt) must not be rejected here.
	BaseURL      *string         `json:"base_url,omitempty" validate:"omitempty,max=255"`
	SwitchID     *int            `json:"switch_id,omitempty" validate:"omitempty,min=0"`
	CommandTopic *string         `json:"command_topic,omitempty" validate:"omitempty,max=255"`
	LocationID   *int            `json:"location_id,omitempty"`
	IsActive     *bool           `json:"is_active,omitempty"`
	Metadata     *map[string]any `json:"metadata,omitempty"`
}

// OutputDeviceResponse is the single-resource envelope.
type OutputDeviceResponse struct {
	Data OutputDevice `json:"data"`
}
