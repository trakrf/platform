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

// Rule modes (TRA-943). egress = fire ON on a crossing then latch; presence =
// ON while >=1 member tag is present, OFF when the last ages out.
const (
	ModeEgress   = "egress"
	ModePresence = "presence"
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

// metaInt reads metadata[key] as an int. Metadata arrives as map[string]any from
// jsonb, so numbers are float64; int/int64/json.Number are tolerated too. ok is
// false when the key is absent or not numeric.
func (d OutputDevice) metaInt(key string) (int, bool) {
	m, ok := d.Metadata.(map[string]any)
	if !ok {
		return 0, false
	}
	switch n := m[key].(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// AutoOffSeconds returns metadata.auto_off_seconds, or 0 when unset, zero,
// negative, or non-numeric. 0 means "stay on until manual reset" (the latch
// default). Device-side (Shelly toggle_after); ignored by the engine in
// presence mode (the engine owns the OFF edge).
func (d OutputDevice) AutoOffSeconds() int {
	v, ok := d.metaInt("auto_off_seconds")
	if !ok || v < 0 {
		return 0
	}
	return v
}

// Mode returns metadata.mode, defaulting to ModeEgress for unset/unknown values.
func (d OutputDevice) Mode() string {
	m, ok := d.Metadata.(map[string]any)
	if ok {
		if s, _ := m["mode"].(string); s == ModePresence {
			return ModePresence
		}
	}
	return ModeEgress
}

// ModeOpt returns metadata.mode when explicitly set to a valid value, with
// set-ness. ok=false lets the resolver fall through to a lower tier — unlike
// Mode(), which hard-defaults unset/unknown to egress.
func (d OutputDevice) ModeOpt() (string, bool) {
	m, ok := d.Metadata.(map[string]any)
	if !ok {
		return "", false
	}
	switch s, _ := m["mode"].(string); s {
	case ModePresence, ModeEgress:
		return s, true
	}
	return "", false
}

// AutoOffSecondsOpt returns metadata.auto_off_seconds with set-ness (>= 0), so the
// resolver can distinguish "unset" (fall through) from an explicit 0. Mirrors the
// clamp in AutoOffSeconds.
func (d OutputDevice) AutoOffSecondsOpt() (int, bool) {
	v, ok := d.metaInt("auto_off_seconds")
	if !ok || v < 0 {
		return 0, false
	}
	return v, true
}

// AgeOutSeconds returns the per-output age-out override from
// metadata.age_out_seconds. ok is false (caller falls back to the global TTL)
// when unset, non-numeric, or <= 0. Egress: re-arm window. Presence: departure
// window.
func (d OutputDevice) AgeOutSeconds() (int, bool) {
	v, ok := d.metaInt("age_out_seconds")
	if !ok || v <= 0 {
		return 0, false
	}
	return v, true
}

// RSSIThreshold returns the per-output RSSI trip line from
// metadata.rssi_threshold (dBm; negatives valid). ok is false (caller falls back
// to the global threshold) when unset or non-numeric.
func (d OutputDevice) RSSIThreshold() (int, bool) {
	return d.metaInt("rssi_threshold")
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
	// ClearLocationID is set by the PATCH handler on an explicit JSON null for
	// location_id, requesting a column-clear (detach the location). An omitted
	// location_id leaves the binding unchanged; a present null clears it. Not
	// decoded directly (mirrors scanpoint.UpdateScanPointRequest, TRA-931).
	ClearLocationID bool `json:"-"`
}

// OutputDeviceResponse is the single-resource envelope.
type OutputDeviceResponse struct {
	Data OutputDevice `json:"data"`
}
