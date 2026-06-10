package geofence

import (
	"os"
	"time"
)

// Config tunes the geofence engine. Defaults make it safe to run with no
// configuration; the engine only runs at all when the MQTT subscriber is enabled.
type Config struct {
	// RSSIThreshold is the system/code-default trip line in dBm: a boundary read
	// fires only when its RSSI >= threshold. Stronger signals are closer to 0
	// (e.g. -50 is stronger than -70), so a higher threshold means a tighter
	// portal. This is the lowest tier — an org default (org
	// metadata.geofence_defaults) and a per-output override
	// (output_device.metadata.rssi_threshold) take precedence (TRA-955).
	RSSIThreshold int
	// LatchTTL is the system/code-default absence window: once a tag fires at a
	// boundary, repeat reads are suppressed until it has been silent this long,
	// after which a re-entry fires again. Overridable per-org and per-output via
	// age_out_seconds (TRA-955).
	LatchTTL time.Duration
	// SweepInterval is how often the latch GCs aged-out entries (memory hygiene;
	// expiry is also enforced on access, so this does not affect correctness).
	SweepInterval time.Duration
}

// DefaultConfig returns production defaults: -65 dBm trip line, 60s latch TTL,
// 5m sweep interval.
func DefaultConfig() Config {
	return Config{
		RSSIThreshold: -65,
		LatchTTL:      60 * time.Second,
		SweepInterval: 5 * time.Minute,
	}
}

// ConfigFromEnv reads GEOFENCE_SWEEP_INTERVAL only. The RSSI threshold and latch
// TTL were retired as env knobs (TRA-955): they are now system/code defaults
// (DefaultConfig), overridable per-org (org metadata.geofence_defaults) and
// per-output (output_device.metadata) at runtime via the UI — no redeploy needed.
// Sweep interval is engine-global housekeeping (latch GC cadence), not per-portal
// tuning, so it stays env-configurable, falling back to the default when unset or
// unparseable.
func ConfigFromEnv() Config {
	c := DefaultConfig()
	if v := os.Getenv("GEOFENCE_SWEEP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			c.SweepInterval = d
		}
	}
	return c
}
