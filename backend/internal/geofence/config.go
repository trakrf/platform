package geofence

import (
	"os"
	"strconv"
	"time"
)

// Config tunes the geofence engine. Defaults make it safe to run with no
// configuration; the engine only runs at all when the MQTT subscriber is enabled.
type Config struct {
	// RSSIThreshold is the global trip line in dBm: a boundary read fires only
	// when its RSSI >= threshold. Stronger signals are closer to 0 (e.g. -50 is
	// stronger than -70), so a higher threshold means a tighter portal. A
	// per-scan-point override (metadata.rssi_threshold) takes precedence.
	RSSIThreshold int
	// LatchTTL is the absence window: once a tag fires at a boundary, repeat
	// reads are suppressed until it has been silent this long, after which a
	// re-entry fires again.
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

// ConfigFromEnv reads GEOFENCE_RSSI_THRESHOLD, GEOFENCE_LATCH_TTL, and
// GEOFENCE_SWEEP_INTERVAL, falling back to DefaultConfig for anything unset or
// unparseable.
func ConfigFromEnv() Config {
	c := DefaultConfig()
	if v := os.Getenv("GEOFENCE_RSSI_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.RSSIThreshold = n
		}
	}
	if v := os.Getenv("GEOFENCE_LATCH_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			c.LatchTTL = d
		}
	}
	if v := os.Getenv("GEOFENCE_SWEEP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			c.SweepInterval = d
		}
	}
	return c
}
