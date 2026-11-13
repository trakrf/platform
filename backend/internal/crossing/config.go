package crossing

import (
	"os"
	"strconv"
	"time"
)

// Config tunes crossing detection. Defaults make it safe to run with no
// configuration; detection only runs at all when the MQTT subscriber is enabled.
// Every value is for tuning on a CS463.
type Config struct {
	// Window is how long a read counts as what a location hears now. It must
	// exceed a full antenna cycle (dwell × antennas: 500 ms × 4 = 2s), or a
	// location can look silent between its own reads.
	Window time.Duration
	// History is how long a read is kept after it leaves the window, as the
	// reference a location's current level is judged against.
	History time.Duration
	// MinReads is how many reads in the window a location needs before its level
	// counts as sustained. One read never crosses.
	MinReads int
	// MarginDB is how many dB the new location's level must beat the leader's by.
	// Anything less, with both heard, is a tie.
	MarginDB int
	// FadeDB is how many dB the leader must have dropped below its recent peak.
	FadeDB int
	// GrowDB is how many dB a location must have risen above its recent trough
	// to count as growing.
	GrowDB int
	// Silence is how long the leader must be unheard before a location that grew
	// since can take over without the leader being seen to fade.
	Silence time.Duration
	// Gap is how long an asset can go unread at a reader before its next read
	// starts the silence clock and the growth evidence afresh.
	Gap time.Duration
	// Dedup is how long a repeat crossing with the same asset, reader, from and
	// to locations goes unrecorded. It caps the alerts from a parked asset whose
	// leader flips between two antennas.
	Dedup time.Duration
}

// DefaultConfig returns production defaults: 5s window, 60s history, 2 reads,
// 5 dB margin, 3 dB fade and growth, 30s silence, 30s gap and 300s dedup.
func DefaultConfig() Config {
	return Config{
		Window:   5 * time.Second,
		History:  60 * time.Second,
		MinReads: 2,
		MarginDB: 5,
		FadeDB:   3,
		GrowDB:   3,
		Silence:  30 * time.Second,
		Gap:      30 * time.Second,
		Dedup:    300 * time.Second,
	}
}

// ConfigFromEnv reads CROSSING_WINDOW, CROSSING_HISTORY, CROSSING_MIN_READS,
// CROSSING_MARGIN_DB, CROSSING_FADE_DB, CROSSING_GROW_DB, CROSSING_SILENCE,
// CROSSING_GAP and CROSSING_DEDUP, falling back to the default for any value that
// is unset, unparseable or out of range. The durations must be positive and the
// read count at least 1. A dB value of 0 is well-formed: a margin of 0 lets any
// location at least as strong as the leader take over.
func ConfigFromEnv() Config {
	c := DefaultConfig()
	for name, d := range map[string]*time.Duration{
		"CROSSING_WINDOW":  &c.Window,
		"CROSSING_HISTORY": &c.History,
		"CROSSING_SILENCE": &c.Silence,
		"CROSSING_GAP":     &c.Gap,
		"CROSSING_DEDUP":   &c.Dedup,
	} {
		if v, err := time.ParseDuration(os.Getenv(name)); err == nil && v > 0 {
			*d = v
		}
	}
	for name, n := range map[string]*int{
		"CROSSING_MARGIN_DB": &c.MarginDB,
		"CROSSING_FADE_DB":   &c.FadeDB,
		"CROSSING_GROW_DB":   &c.GrowDB,
	} {
		if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v >= 0 {
			*n = v
		}
	}
	if v, err := strconv.Atoi(os.Getenv("CROSSING_MIN_READS")); err == nil && v >= 1 {
		c.MinReads = v
	}
	return c
}
