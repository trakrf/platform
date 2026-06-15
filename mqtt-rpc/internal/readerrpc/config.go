package readerrpc

// AntennaPower is the transmit power for a single antenna port.
type AntennaPower struct {
	Antenna int     `json:"antenna"`
	Power   float64 `json:"power"`
}

// ReaderConfig is the readable/settable reader configuration. Pointer fields are
// optional: on a SetConfig request a nil field means "leave unchanged", enabling
// partial updates.
type ReaderConfig struct {
	TxPowerDBm []AntennaPower `json:"tx_power_dbm,omitempty"`
	Region     *string        `json:"region,omitempty"`
	Session    *int           `json:"session,omitempty"`
	Q          *int           `json:"q,omitempty"`
	Target     *int           `json:"target,omitempty"`
}

// TxPowerCap describes the transmit-power capabilities of a reader.
type TxPowerCap struct {
	MinDBm     float64 `json:"min_dbm"`
	MaxDBm     float64 `json:"max_dbm"`
	PerAntenna bool    `json:"per_antenna"`
}

// Capabilities is the response to Reader.GetCapabilities, describing what a
// reader supports.
type Capabilities struct {
	ContractVersion int        `json:"contract_version"`
	ReaderModel     string     `json:"reader_model"`
	Antennas        int        `json:"antennas"`
	TxPower         TxPowerCap `json:"tx_power"`
	Supports        []string   `json:"supports"`
	Unsupported     []string   `json:"unsupported"`
}

// Status is the response to Reader.GetStatus and the payload published on the
// status topic.
type Status struct {
	Online        bool   `json:"online"`
	Reading       bool   `json:"reading"`
	ActiveProfile string `json:"active_profile,omitempty"`
}

// SetConfigResult is the response to Reader.SetConfig. Applied reports whether
// the change took effect immediately or is pending a reload.
type SetConfigResult struct {
	Applied     string `json:"applied"`
	EffectiveAt string `json:"effective_at,omitempty"`
}

// Applied values for SetConfigResult.
const (
	AppliedImmediate     = "immediate"
	AppliedPendingReload = "pending_reload"
)
