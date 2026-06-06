// Package alarm wires the geofence engine's fire decision to physical alarm
// output devices (TRA-903). Firer implements geofence.Firer: on each fire it
// looks up the active output devices bound to the tripped scan point and drives
// them on. It lives outside the geofence package to avoid an import cycle —
// alarm depends on geofence (for AlarmEvent + the Firer contract), not vice
// versa.
package alarm

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/trakrf/platform/backend/internal/geofence"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

// deviceLookup is the storage dependency Firer needs; *storage.Storage
// satisfies it. Narrowed to an interface so unit tests can inject a fake.
type deviceLookup interface {
	ListOutputDevicesForLocation(ctx context.Context, orgID, locationID int) ([]outputdevice.OutputDevice, error)
}

// deviceSetter drives one output device on/off using its configured transport;
// Dispatcher satisfies it. Narrowed so tests can inject a fake.
type deviceSetter interface {
	Set(ctx context.Context, dev outputdevice.OutputDevice, on bool, offAfterSec int) error
}

// Firer drives output devices when the geofence engine fires. Construct with
// NewFirer. Fire is best-effort: per-device errors are logged and aggregated
// but never panic, matching the engine's best-effort fire contract.
type Firer struct {
	store deviceLookup
	act   deviceSetter
	log   zerolog.Logger
}

// NewFirer builds a Firer with a component-tagged logger.
func NewFirer(store deviceLookup, act deviceSetter, log *zerolog.Logger) Firer {
	return Firer{
		store: store,
		act:   act,
		log:   log.With().Str("component", "alarm").Logger(),
	}
}

// Fire logs the boundary alarm, then turns on every active output device bound
// to the event's scan point. Each device's metadata.auto_off_seconds becomes the
// device-side flip-back timer (0 = stay on until manual reset). A device-drive
// failure is logged (fail-quiet) and folded into the returned error so the
// engine's fire-error metric increments; the engine never lets that block
// ingestion.
func (f Firer) Fire(ctx context.Context, ev geofence.AlarmEvent) error {
	f.log.Warn().
		Int("org_id", ev.OrgID).
		Int("asset_id", ev.AssetID).
		Int("scan_point_id", ev.ScanPointID).
		Str("epc", ev.EPC).
		Int("rssi", ev.RSSI).
		Time("fired_at", ev.FiredAt).
		Msg("geofence boundary alarm")

	// The alarm cares about the logical location, not the reader/antenna. If the
	// tripped scan point isn't mapped to a location, no location-bound alarm can
	// match — nothing to fire.
	if ev.LocationID == nil {
		return nil
	}

	devices, err := f.store.ListOutputDevicesForLocation(ctx, ev.OrgID, *ev.LocationID)
	if err != nil {
		return fmt.Errorf("alarm: lookup devices for location %d: %w", *ev.LocationID, err)
	}

	var errs []error
	for _, d := range devices {
		if err := f.act.Set(ctx, d, true, d.AutoOffSeconds()); err != nil {
			f.log.Error().Err(err).
				Int("output_device_id", d.ID).
				Str("transport", d.Transport).
				Msg("output device fire failed (fail-quiet)")
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
