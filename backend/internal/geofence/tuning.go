package geofence

import (
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
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
