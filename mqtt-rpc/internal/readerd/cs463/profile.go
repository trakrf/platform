package cs463

// Golden Operation Profile creation. Unlike the four event-engine entities, the
// profile is created with a DEFAULT config and then left to the operator to tune
// (antennas/TX power via Reader.SetConfig) — the daemon only creates it if absent and
// never reconciles/clobbers an existing one. Only the MQTT Server stays hand-crafted
// (it needs the broker secret + TLS cert).

import (
	"context"
	"net/url"
	"strconv"
)

// DefaultProfileTxPowerDBm is the TX power for the auto-created golden profile's single
// enabled antenna (port 1). POLS: a fresh reader most likely has one antenna on port 1,
// and enabling only port 1 also avoids reflected-power errors on unconnected ports.
const DefaultProfileTxPowerDBm = 30.0

// CreateProfile creates the operation profile ENTRY via /API setOperProfile with sane
// defaults and antenna 1 selected. setOperProfile cannot ENABLE the antenna on this
// firmware (the footgun), so the caller follows with a servlet SetProfilePower to
// actually enable port 1 (see Adapter.ensureProfile).
func (c *Client) CreateProfile(ctx context.Context, session, profileID string, txPowerDBm float64) error {
	p := url.Values{
		"profile_id":              {profileID},
		"linkProfile":             {"1"},
		"populationEst":           {"50"},
		"sessionNo":               {"0"},
		"target":                  {"2"},
		"queryAlgorithm":          {"DynamicQ"},
		"reflectedPowerThreshold": {"24"},
		"tagModel":                {"ANY"},
		"antenna_port":            {"1"},
		"transmitPower":           {strconv.FormatFloat(txPowerDBm, 'f', 1, 64)},
	}
	// Dwell 500ms on EVERY antenna slot (1..16), enabled or not, so any antenna the
	// operator later enables via SetConfig already carries the golden dwell (dwell=dedup).
	for i := 1; i <= 16; i++ {
		p.Set("dwellTime"+strconv.Itoa(i), strconv.Itoa(GoldenDwellMs))
	}
	return c.writeEntity(ctx, session, "setOperProfile", p)
}
