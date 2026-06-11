// Package muster contains the wire-types for TRA-978 mustering POC.
// All id fields use int (matching the house style in asset/scandevice models)
// even though the DB column is BIGINT — pgx scans BIGINT into int on 64-bit
// platforms, so this is safe. The plan spec listed int64 for some fields but
// the canonical house type is int (see asset.Asset, scandevice.ScanDevice).
// DEVIATION NOTED: id types are `int` throughout (not int64 as written in the
// plan). Later phases must use int too. See plan file for update.
package muster

import (
	"encoding/json"
	"time"
)

// Event is a mustering drill event. One active event per org at a time.
type Event struct {
	ID            int             `json:"id"`
	OrgID         int             `json:"org_id"`
	Status        string          `json:"status"` // active|completed|cancelled
	StartedAt     time.Time       `json:"started_at"`
	EndedAt       *time.Time      `json:"ended_at,omitempty"`
	WindowMinutes int             `json:"window_minutes"`
	StartedBy     *int            `json:"started_by,omitempty"`
	Report        json.RawMessage `json:"report,omitempty"`
	Metadata      map[string]any  `json:"metadata,omitempty"`
	Entries       []Entry         `json:"entries,omitempty"`
	Counts        Counts          `json:"counts"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Entry is a person-level mustering entry within an event.
// Status transitions: missing → at_muster → verified
//
//	missing → safe_manual
//	at_muster → safe_manual
//
// verified and safe_manual are sticky (no further transitions).
type Entry struct {
	ID                 int        `json:"id"`
	OrgID              int        `json:"org_id"`
	MusterEventID      int        `json:"muster_event_id"`
	AssetID            int        `json:"asset_id"`
	Label              string     `json:"label"`
	ExpectedLocationID *int       `json:"expected_location_id,omitempty"`
	Status             string     `json:"status"` // missing|at_muster|verified|safe_manual
	MusterLocationID   *int       `json:"muster_location_id,omitempty"`
	FirstMusterSeenAt  *time.Time `json:"first_muster_seen_at,omitempty"`
	VerifiedBy         *int       `json:"verified_by,omitempty"`
	VerifiedAt         *time.Time `json:"verified_at,omitempty"`
	MarkedSafeBy       *int       `json:"marked_safe_by,omitempty"`
	MarkedSafeAt       *time.Time `json:"marked_safe_at,omitempty"`
	MarkedSafeNote     string     `json:"marked_safe_note,omitempty"`
	LastSeenLocationID *int       `json:"last_seen_location_id,omitempty"` // populated only while event active (break-glass)
	LastSeenAt         *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// Counts aggregates entry statuses for an event.
type Counts struct {
	Expected   int `json:"expected"`
	Missing    int `json:"missing"`
	AtMuster   int `json:"at_muster"`
	Verified   int `json:"verified"`
	SafeManual int `json:"safe_manual"`
}

// ZonePresence is a presence-only headcount for one location (zone or muster point).
// Count is filled by the engine from in-memory state; storage returns 0.
type ZonePresence struct {
	LocationID  int    `json:"location_id"`
	Name        string `json:"name"`
	MusterPoint bool   `json:"muster_point"`
	Count       int    `json:"count"`
}

// PersonPresence is the most-recent sighting of a person-asset within the
// presence window. LocationID may be nil if never seen.
type PersonPresence struct {
	AssetID    int       `json:"asset_id"`
	Label      string    `json:"label"`
	LocationID *int      `json:"location_id,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// ErrActiveEventExists is returned by CreateMusterEvent when the org already
// has an active event. Handlers map this to 409 Conflict.
type ErrActiveEventExists struct{}

func (e ErrActiveEventExists) Error() string {
	return "org already has an active muster event"
}

// ErrInvalidTransition is returned by UpdateEntryStatus when the requested
// action is not valid from the entry's current status. Handlers map this to 409.
type ErrInvalidTransition struct {
	Current string
	Action  string
}

func (e ErrInvalidTransition) Error() string {
	return "invalid status transition: action=" + e.Action + " from status=" + e.Current
}
