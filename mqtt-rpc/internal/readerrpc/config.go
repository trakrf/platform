package readerrpc

// AntennaConfig is the enablement + transmit power for a single antenna port. It
// is the per-antenna unit of both GetOperProfile (current state) and
// SetOperProfile (desired state).
type AntennaConfig struct {
	Antenna  int     `json:"antenna"`
	Enabled  bool    `json:"enabled"`
	PowerDBm float64 `json:"power_dbm"`
}

// ReaderConfig is the readable/settable reader configuration.
//
// Antennas carries per-antenna enablement + power. On a SetOperProfile request a
// nil/empty Antennas means "leave unchanged"; entries present are merged over the
// reader's current profile.
//
// DwellMs/DedupWindowMs/AntennaDifferentiation are editable read-timing knobs
// (TRA-1003): GetOperProfile populates them and SetOperProfile writes them (dwell
// via the servlet RMW applied to all ports; dedup/antDiff via modEvent). A nil
// field on a SetOperProfile request means "leave unchanged". RSSIGateDBm is
// READ-ONLY (populated on GET, ignored on SET; TRA-1002 owns it).
// Region/Session/Q/Target are reserved.
type ReaderConfig struct {
	Antennas []AntennaConfig `json:"antennas,omitempty"`

	DwellMs                *int  `json:"dwell_ms,omitempty"`
	DedupWindowMs          *int  `json:"dedup_window_ms,omitempty"`
	RSSIGateDBm            *int  `json:"rssi_gate_dbm,omitempty"`
	AntennaDifferentiation *bool `json:"antenna_differentiation,omitempty"`

	Region  *string `json:"region,omitempty"`
	Session *int    `json:"session,omitempty"`
	Q       *int    `json:"q,omitempty"`
	Target  *int    `json:"target,omitempty"`
}

// OperProfileParams are the params for Reader.GetOperProfile. Force requests a
// force-logout-then-proceed when the reader's single root session is held by
// another client (see BusyError).
type OperProfileParams struct {
	Force bool `json:"force,omitempty"`
}

// SetOperProfileParams are the params for Reader.SetOperProfile: the desired
// config (flattened) plus the force flag.
type SetOperProfileParams struct {
	ReaderConfig
	Force bool `json:"force,omitempty"`
}

// GpoSetParams are the params for Gpo.Set: drive one general purpose output.
//
// Port is the reader's 1-based GPO port. PulseMs, when On is true and it is > 0,
// requests a ONE-SHOT: the reader drives the port on now and releases it after
// the delay with no second message. It is the reader-side analog of the Shelly
// toggle_after timer, so an output device's auto_off_seconds maps straight onto
// it and the OFF edge never depends on a second round trip surviving. PulseMs is
// ignored for off commands, and 0 means "stay on until an explicit off".
type GpoSetParams struct {
	Port    int  `json:"port"`
	On      bool `json:"on"`
	PulseMs int  `json:"pulse_ms,omitempty"`
}

// GpoSetResult is the response to Gpo.Set. Pulsed reports whether the reader
// armed a one-shot release timer for this call.
type GpoSetResult struct {
	Port   int  `json:"port"`
	On     bool `json:"on"`
	Pulsed bool `json:"pulsed,omitempty"`
}

// TxPowerCap describes the transmit-power capabilities of a reader.
type TxPowerCap struct {
	MinDBm     float64 `json:"min_dbm"`
	MaxDBm     float64 `json:"max_dbm"`
	PerAntenna bool    `json:"per_antenna"`
}

// Capabilities is the response to Reader.GetCapabilities.
type Capabilities struct {
	ContractVersion int        `json:"contract_version"`
	ReaderModel     string     `json:"reader_model"`
	Antennas        int        `json:"antennas"`
	TxPower         TxPowerCap `json:"tx_power"`
	Supports        []string   `json:"supports"`
	Unsupported     []string   `json:"unsupported"`
}

// Status is the response to Reader.GetStatus and the status-topic payload.
type Status struct {
	Online        bool   `json:"online"`
	Reading       bool   `json:"reading"`
	ActiveProfile string `json:"active_profile,omitempty"`
}

// SetConfigResult is the response to Reader.SetOperProfile.
type SetConfigResult struct {
	Applied     string `json:"applied"`
	EffectiveAt string `json:"effective_at,omitempty"`
}

// Applied values for SetConfigResult.
const (
	AppliedImmediate     = "immediate"
	AppliedPendingReload = "pending_reload"
)
