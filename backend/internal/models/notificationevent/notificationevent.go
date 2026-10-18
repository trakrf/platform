// Package notificationevent defines detected events that can notify an
// asset's owners, and the location changes that are one of their sources.
//
// The database enforces every rule here (CHECKs, composite foreign keys);
// Validate mirrors them so a caller fails before the round trip, with
// ErrInvalid rather than a constraint error.
package notificationevent

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalid reports a value the database would reject.
var ErrInvalid = errors.New("notificationevent: invalid")

// Source names the detector that produced a notification event.
type Source string

const (
	SourceAlarm          Source = "alarm"
	SourceLocationChange Source = "location_change"
)

// Origin names the path that detected a location change.
type Origin string

const (
	OriginReader         Origin = "reader"          // fixed reader
	OriginManualSave     Origin = "manual_save"     // handheld inventory save
	OriginManualOverride Origin = "manual_override" // same-minute correction of a reader row
)

// LocationChange is a location_changes row: one detected move of an asset.
type LocationChange struct {
	ID             int
	OrgID          int
	AssetID        int
	FromLocationID *int // nil on a first sighting or when the previous scan had no location
	ToLocationID   int
	Origin         Origin
	ScanPointID    *int // set exactly when Origin is OriginReader
	DetectedAt     time.Time
}

// Validate mirrors location_changes' constraints.
func (c LocationChange) Validate() error {
	switch {
	case c.OrgID <= 0:
		return invalid("location change org_id must be positive")
	case c.AssetID <= 0:
		return invalid("location change asset_id must be positive")
	case c.ToLocationID <= 0:
		return invalid("location change to_location_id must be positive")
	case c.FromLocationID != nil && *c.FromLocationID <= 0:
		return invalid("location change from_location_id must be positive when set")
	case c.ScanPointID != nil && *c.ScanPointID <= 0:
		return invalid("location change scan_point_id must be positive when set")
	case c.DetectedAt.IsZero():
		return invalid("location change detected_at is required")
	}
	switch c.Origin {
	case OriginReader:
		if c.ScanPointID == nil {
			return invalid("a reader location change needs its scan point")
		}
	case OriginManualSave, OriginManualOverride:
		if c.ScanPointID != nil {
			return invalid("a manual location change has no scan point")
		}
	default:
		return invalid(fmt.Sprintf("unknown location change origin %q", c.Origin))
	}
	return nil
}

// NotificationEvent is a notification_events row. Exactly one of
// AlarmEventID and LocationChangeID is set, the one matching Source, and the
// event's OrgID, AssetID and OccurredAt equal that trigger's.
type NotificationEvent struct {
	ID               int
	OrgID            int
	Source           Source
	AssetID          int
	OccurredAt       time.Time
	AlarmEventID     *int
	LocationChangeID *int
}

// Validate mirrors notification_events_one_source and the row's NOT NULLs.
// Whether the trigger exists and matches is left to the foreign keys.
func (e NotificationEvent) Validate() error {
	switch {
	case e.OrgID <= 0:
		return invalid("notification event org_id must be positive")
	case e.AssetID <= 0:
		return invalid("notification event asset_id must be positive")
	case e.OccurredAt.IsZero():
		return invalid("notification event occurred_at is required")
	case e.AlarmEventID != nil && *e.AlarmEventID <= 0:
		return invalid("notification event alarm_event_id must be positive when set")
	case e.LocationChangeID != nil && *e.LocationChangeID <= 0:
		return invalid("notification event location_change_id must be positive when set")
	}
	switch e.Source {
	case SourceAlarm:
		if e.AlarmEventID == nil || e.LocationChangeID != nil {
			return invalid("an alarm event names its alarm and nothing else")
		}
	case SourceLocationChange:
		if e.LocationChangeID == nil || e.AlarmEventID != nil {
			return invalid("a location-change event names its location change and nothing else")
		}
	default:
		return invalid(fmt.Sprintf("unknown notification event source %q", e.Source))
	}
	return nil
}

func invalid(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalid, msg)
}
