package geofence

import (
	"fmt"
	"time"

	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/readerrpc"
)

// Tuning is the fully-resolved per-output geofence tuning (TRA-955): the values
// the engine actually applies for one output device, after collapsing the three
// config tiers.
type Tuning struct {
	RSSIThreshold  int    `json:"rssi_threshold"`   // dBm trip line
	AgeOutSeconds  int    `json:"age_out_seconds"`  // egress re-arm / presence departure window
	AutoOffSeconds int    `json:"auto_off_seconds"` // device-side auto-off (egress only)
	Mode           string `json:"mode"`             // "egress" | "presence"
}

// SystemTuning is the code/system-tier Tuning: the Config numbers plus the model
// defaults for the fields Config does not carry (mode = egress, auto_off = 0).
func SystemTuning(cfg Config) Tuning {
	return Tuning{
		RSSIThreshold:  cfg.RSSIThreshold,
		AgeOutSeconds:  int(cfg.LatchTTL.Seconds()),
		AutoOffSeconds: 0,
		Mode:           outputdevice.ModeEgress,
	}
}

// Resolve collapses the three tuning tiers for one output device. Precedence per
// field: per-output override > org default > system/code default.
func Resolve(cfg Config, org organization.GeofenceDefaults, dev outputdevice.OutputDevice) Tuning {
	t := SystemTuning(cfg)

	if org.RSSIThreshold != nil {
		t.RSSIThreshold = *org.RSSIThreshold
	}
	if v, ok := dev.RSSIThreshold(); ok {
		t.RSSIThreshold = v
	}

	if org.AgeOutSeconds != nil {
		t.AgeOutSeconds = *org.AgeOutSeconds
	}
	if v, ok := dev.AgeOutSeconds(); ok {
		t.AgeOutSeconds = v
	}

	if org.AutoOffSeconds != nil {
		t.AutoOffSeconds = *org.AutoOffSeconds
	}
	if v, ok := dev.AutoOffSecondsOpt(); ok {
		t.AutoOffSeconds = v
	}

	if org.Mode != nil && (*org.Mode == outputdevice.ModePresence || *org.Mode == outputdevice.ModeEgress) {
		t.Mode = *org.Mode
	}
	if v, ok := dev.ModeOpt(); ok {
		t.Mode = v
	}

	return t
}

// MaxAutoOffSeconds is the longest auto-off an output may be configured with, at
// either tier. It is the reader's hard cap on any GPO on-time (readerrpc.GpoMaxOn):
// alarm outputs fail toward off (ADR 0026), and a value the reader would cut short
// anyway is a misconfiguration to refuse, not a setting to store.
const MaxAutoOffSeconds = int(readerrpc.GpoMaxOn / time.Second)

// OutputTuningError reports why an output device, in its effective (post-merge)
// state, cannot be saved under the given org defaults. It returns "" when valid.
//
// The rule that matters: in egress mode the only OFF edge is the device-side
// auto-off timer (Shelly toggle_after, the reader's GPO one-shot), so an egress
// output whose RESOLVED auto-off is 0 turns on at its first read and never turns
// off. That is refused here rather than given a silent default pulse length.
// Presence mode owns its OFF edge in the engine and ignores auto-off, so only the
// type and ceiling checks apply to it.
func OutputTuningError(org organization.GeofenceDefaults, dev outputdevice.OutputDevice) string {
	if m, ok := dev.Metadata.(map[string]any); ok {
		if raw, has := m["mode"]; has && raw != nil {
			if s, _ := raw.(string); s != outputdevice.ModeEgress && s != outputdevice.ModePresence {
				return fmt.Sprintf("mode must be %q or %q", outputdevice.ModeEgress, outputdevice.ModePresence)
			}
		}
	}
	if v, present, ok := dev.AutoOffSecondsRaw(); present {
		if !ok {
			return "auto_off_seconds must be a whole number of seconds"
		}
		if v < 0 || v > MaxAutoOffSeconds {
			return fmt.Sprintf("auto_off_seconds must be between 0 and %d", MaxAutoOffSeconds)
		}
	}
	t := Resolve(DefaultConfig(), org, dev)
	if t.Mode == outputdevice.ModeEgress && t.AutoOffSeconds <= 0 {
		return fmt.Sprintf("auto_off_seconds is required for an egress output when the org default does not supply one: "+
			"without it the output turns on and never turns off. Set 1-%d seconds", MaxAutoOffSeconds)
	}
	return ""
}

// OrgDefaultsError reports an org-tier auto-off outside 0..MaxAutoOffSeconds, or
// "". Whether a change strands existing devices is a separate check that needs
// the devices (see OutputTuningError).
func OrgDefaultsError(d organization.GeofenceDefaults) string {
	if d.AutoOffSeconds != nil && (*d.AutoOffSeconds < 0 || *d.AutoOffSeconds > MaxAutoOffSeconds) {
		return fmt.Sprintf("auto_off_seconds must be between 0 and %d", MaxAutoOffSeconds)
	}
	return ""
}
