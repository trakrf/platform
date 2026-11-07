package crossing

import (
	"os"
	"strconv"
	"time"
)

// Config tunes crossing detection. Defaults make it safe to run with no
// configuration; detection only runs at all when the MQTT subscriber is enabled.
type Config struct {
	// Window is how long a read counts as recent evidence. It must exceed a full
	// antenna cycle (dwell × antennas: 500 ms × 4 = 2s), or a location can look
	// silent between its own reads. It must also stay short: peaks have to age
	// out, or an inside antenna with a higher peak would hide an exit.
	Window time.Duration
	// MarginDB is how many dB a location's strongest recent read must beat the
	// leader's by to take over. Anything less is a tie.
	MarginDB int
	// Dedup is how long a repeat crossing with the same asset, reader, from and
	// to locations goes unrecorded. It caps the alerts from a parked asset whose
	// leader flips between two antennas.
	Dedup time.Duration
}

// DefaultConfig returns production defaults: 3s window, 3 dB margin, 60s dedup.
func DefaultConfig() Config {
	return Config{
		Window:   3 * time.Second,
		MarginDB: 3,
		Dedup:    60 * time.Second,
	}
}

// ConfigFromEnv reads CROSSING_WINDOW, CROSSING_MARGIN_DB and CROSSING_DEDUP,
// falling back to the default for any value that is unset, unparseable or out of
// range. The durations must be positive. A margin of 0 is well-formed: any
// location at least as strong as the leader takes over.
func ConfigFromEnv() Config {
	c := DefaultConfig()
	if v := os.Getenv("CROSSING_WINDOW"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			c.Window = d
		}
	}
	if v := os.Getenv("CROSSING_MARGIN_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			c.MarginDB = n
		}
	}
	if v := os.Getenv("CROSSING_DEDUP"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			c.Dedup = d
		}
	}
	return c
}
