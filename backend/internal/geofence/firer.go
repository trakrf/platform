package geofence

import (
	"context"

	"github.com/rs/zerolog"
)

// Firer performs the physical alarm action when the geofence engine decides to
// fire. It is the seam for TRA-903 (alarm device CRUD + Shelly Gen4 trigger):
// that ticket plugs a device-driving implementation in behind this interface
// with no change to the engine.
type Firer interface {
	Fire(ctx context.Context, ev AlarmEvent) error
}

// LogFirer is the v1 implementation: it logs the fire. The alarm_events row
// (the durable record) is written by the engine separately; this stands in for
// the physical device action until TRA-903 lands.
type LogFirer struct {
	log zerolog.Logger
}

// NewLogFirer builds a LogFirer with a component-tagged logger.
func NewLogFirer(log *zerolog.Logger) LogFirer {
	return LogFirer{log: log.With().Str("component", "geofence").Logger()}
}

func (f LogFirer) Fire(_ context.Context, ev AlarmEvent) error {
	f.log.Warn().
		Int("org_id", ev.OrgID).
		Int("asset_id", ev.AssetID).
		Int("scan_point_id", ev.ScanPointID).
		Str("epc", ev.EPC).
		Int("rssi", ev.RSSI).
		Time("fired_at", ev.FiredAt).
		Msg("geofence boundary alarm (log-only firer; physical action pending TRA-903)")
	return nil
}
