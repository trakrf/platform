package arbitration

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scandevice"
)

// Config tunes zone arbitration and says which device types it runs for.
type Config struct {
	// Window is how far back a scan point's reads count toward its level, the
	// mean of its usable RSSI in the window. A location with no read in the
	// window is unheard. It must comfortably exceed the gateway publish
	// interval (GL-S10: ~1 s).
	Window time.Duration
	// MarginDB is how many dB a challenger's level must beat the assigned
	// location's by. Anything less never moves the asset.
	MarginDB int
	// Dwell is how long a challenger must hold that lead before it takes over.
	Dwell time.Duration
	// Silence is how long the assigned location may go unheard before any heard
	// location takes it over, however weak. Sooner than that, an unheard location
	// only yields to one heard about as strongly as it last was (within
	// MarginDB): the asset has turned up somewhere comparable, so it moved. A
	// GL-S10 can skip a tag it hears at -37 dBm for five minutes while the next
	// room still hears it at -71 (homelab capture, 2026-11-27); handing over on
	// a short silence would flap on exactly that.
	Silence time.Duration
	// ReturnWindow and ReturnDwell stop ping-pong at a boundary. Going back to
	// the location the asset left less than ReturnWindow ago needs the lead
	// held for ReturnDwell instead of Dwell, and never happens on a gap alone.
	// A still asset that two rooms hear about equally is otherwise handed back
	// and forth by each gateway's routine 10-30 s report gaps; a walk does not
	// go straight back. Homelab bookshelf capture (2026-11-28): 16 moves in 51
	// minutes without it, 5 with it, and the walk capture unchanged. Zero
	// disables the rule.
	ReturnWindow time.Duration
	ReturnDwell  time.Duration
	// DeviceTypes are the device types whose reads are arbitrated. Others are
	// stored as they arrive, exactly as before arbitration existed.
	DeviceTypes map[string]bool
}

// DefaultDeviceTypes is the BLE gateways. Fixed RFID readers are out: their
// RSSI is not comparable across readers and antennas the way BLE gateways
// roughly are, and crossing already owns multi-antenna CS463 gates.
var DefaultDeviceTypes = []string{
	scandevice.DeviceTypeGLS10,
	scandevice.DeviceTypeMK107,
	scandevice.DeviceTypeESP32BLE,
}

// DefaultConfig returns production defaults: 10s window, 6 dB margin, 5s dwell,
// 10m silence, a 30s dwell to return to a location left within 5m, BLE
// gateways only.
func DefaultConfig() Config {
	return Config{
		Window:       10 * time.Second,
		MarginDB:     6,
		Dwell:        5 * time.Second,
		Silence:      10 * time.Minute,
		ReturnWindow: 5 * time.Minute,
		ReturnDwell:  30 * time.Second,
		DeviceTypes:  setOf(DefaultDeviceTypes),
	}
}

// Enabled reports whether reads from this device type are arbitrated.
func (c Config) Enabled(deviceType string) bool { return c.DeviceTypes[deviceType] }

// ConfigFromEnv reads ARBITRATION_WINDOW, ARBITRATION_MARGIN_DB,
// ARBITRATION_DWELL, ARBITRATION_SILENCE, ARBITRATION_RETURN_WINDOW,
// ARBITRATION_RETURN_DWELL and ARBITRATION_DEVICE_TYPES (comma-separated;
// "none" turns arbitration off), falling back to the default for any value
// that is unset, unparseable or out of range. Durations must be positive,
// except ARBITRATION_RETURN_WINDOW, where 0 turns the return rule off; a
// margin of 0 is well-formed.
func ConfigFromEnv() Config {
	c := DefaultConfig()
	for name, d := range map[string]*time.Duration{
		"ARBITRATION_WINDOW":  &c.Window,
		"ARBITRATION_DWELL":   &c.Dwell,
		"ARBITRATION_SILENCE": &c.Silence,
	} {
		if v, err := time.ParseDuration(os.Getenv(name)); err == nil && v > 0 {
			*d = v
		}
	}
	if v, err := time.ParseDuration(os.Getenv("ARBITRATION_RETURN_WINDOW")); err == nil && v >= 0 {
		c.ReturnWindow = v
	}
	if v, err := time.ParseDuration(os.Getenv("ARBITRATION_RETURN_DWELL")); err == nil && v > 0 {
		c.ReturnDwell = v
	}
	if v, err := strconv.Atoi(os.Getenv("ARBITRATION_MARGIN_DB")); err == nil && v >= 0 {
		c.MarginDB = v
	}
	if raw := strings.TrimSpace(os.Getenv("ARBITRATION_DEVICE_TYPES")); raw != "" {
		var types []string
		if raw != "none" {
			for _, t := range strings.Split(raw, ",") {
				if t = strings.TrimSpace(t); t != "" {
					types = append(types, t)
				}
			}
		}
		c.DeviceTypes = setOf(types)
	}
	return c
}

func setOf(types []string) map[string]bool {
	m := make(map[string]bool, len(types))
	for _, t := range types {
		m[t] = true
	}
	return m
}
