package notificationevent

import (
	"errors"
	"testing"
	"time"
)

var at = time.Date(2026, 10, 18, 12, 0, 30, 0, time.UTC)

func ptr(i int) *int { return &i }

func validChange() LocationChange {
	return LocationChange{OrgID: 1, AssetID: 2, ToLocationID: 3, Origin: OriginManualSave, DetectedAt: at}
}

func TestLocationChangeValidate(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*LocationChange)
		valid bool
	}{
		{"manual save", func(c *LocationChange) {}, true},
		{"manual override", func(c *LocationChange) { c.Origin = OriginManualOverride }, true},
		{"reader with its scan point", func(c *LocationChange) { c.Origin = OriginReader; c.ScanPointID = ptr(9) }, true},
		{"first sighting has no origin location", func(c *LocationChange) { c.FromLocationID = nil }, true},
		{"move with an origin location", func(c *LocationChange) { c.FromLocationID = ptr(4) }, true},

		{"reader without a scan point", func(c *LocationChange) { c.Origin = OriginReader }, false},
		{"manual save with a scan point", func(c *LocationChange) { c.ScanPointID = ptr(9) }, false},
		{"manual override with a scan point", func(c *LocationChange) { c.Origin = OriginManualOverride; c.ScanPointID = ptr(9) }, false},
		{"simulated is not an origin", func(c *LocationChange) { c.Origin = Origin("simulated") }, false},
		{"empty origin", func(c *LocationChange) { c.Origin = "" }, false},
		{"zero org", func(c *LocationChange) { c.OrgID = 0 }, false},
		{"zero asset", func(c *LocationChange) { c.AssetID = 0 }, false},
		{"zero destination", func(c *LocationChange) { c.ToLocationID = 0 }, false},
		{"zero time", func(c *LocationChange) { c.DetectedAt = time.Time{} }, false},
		{"non-positive scan point", func(c *LocationChange) { c.Origin = OriginReader; c.ScanPointID = ptr(0) }, false},
		{"non-positive origin location", func(c *LocationChange) { c.FromLocationID = ptr(-1) }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validChange()
			tc.edit(&c)
			err := c.Validate()
			if tc.valid && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if !tc.valid && !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() = %v, want ErrInvalid", err)
			}
		})
	}
}

func validAlarmEvent() NotificationEvent {
	return NotificationEvent{OrgID: 1, Source: SourceAlarm, AssetID: 2, OccurredAt: at, AlarmEventID: ptr(5)}
}

func TestNotificationEventValidate(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*NotificationEvent)
		valid bool
	}{
		{"alarm with its alarm", func(e *NotificationEvent) {}, true},
		{"location change with its change", func(e *NotificationEvent) {
			e.Source, e.AlarmEventID, e.LocationChangeID = SourceLocationChange, nil, ptr(6)
		}, true},

		{"alarm without its alarm", func(e *NotificationEvent) { e.AlarmEventID = nil }, false},
		{"alarm that also names a change", func(e *NotificationEvent) { e.LocationChangeID = ptr(6) }, false},
		{"alarm that names only a change", func(e *NotificationEvent) { e.AlarmEventID, e.LocationChangeID = nil, ptr(6) }, false},
		{"location change without its change", func(e *NotificationEvent) { e.Source, e.AlarmEventID = SourceLocationChange, nil }, false},
		{"location change that names only an alarm", func(e *NotificationEvent) { e.Source = SourceLocationChange }, false},
		{"unknown source", func(e *NotificationEvent) { e.Source = Source("manual") }, false},
		{"empty source", func(e *NotificationEvent) { e.Source = "" }, false},
		{"zero org", func(e *NotificationEvent) { e.OrgID = 0 }, false},
		{"zero asset", func(e *NotificationEvent) { e.AssetID = 0 }, false},
		{"zero time", func(e *NotificationEvent) { e.OccurredAt = time.Time{} }, false},
		{"non-positive alarm id", func(e *NotificationEvent) { e.AlarmEventID = ptr(0) }, false},
		{"non-positive change id", func(e *NotificationEvent) {
			e.Source, e.AlarmEventID, e.LocationChangeID = SourceLocationChange, nil, ptr(-6)
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := validAlarmEvent()
			tc.edit(&e)
			err := e.Validate()
			if tc.valid && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if !tc.valid && !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() = %v, want ErrInvalid", err)
			}
		})
	}
}
