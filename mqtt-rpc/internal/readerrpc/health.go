package readerrpc

// The retained <base>/status document and the reader-API health it carries.
//
// Online/offline alone is not enough. The CS463 vendor app can come up
// answering every /API request with HTTP 200 and an empty body. The radio keeps
// reading, the daemon stays connected, and nothing reaches the cloud. Health
// reports what a content check of /API found, so that state is visible from
// outside the reader.

// API health states, from a probe judged on the parsed /API body, never on the
// HTTP status.
const (
	// APIUnknown: not probed yet.
	APIUnknown = "unknown"
	// APIHealthy: /API answered the probe with a well-formed reply.
	APIHealthy = "healthy"
	// APIDegraded: reachable, but not answering correctly, as with an empty
	// 200. This is the wedge, and the dangerous state.
	APIDegraded = "degraded"
	// APIUnreachable: no answer, or nginx reports the app down (502).
	APIUnreachable = "unreachable"
)

// Recovery actions the daemon takes on a wedged reader, in ladder order.
const (
	RecoveryAppRestart = "app_restart"
	RecoveryReboot     = "reboot"
)

// Golden-config reconcile states.
const (
	GoldenApplied = "applied"
	GoldenPending = "pending"
	GoldenOff     = "off" // READERD_RECONCILE=false
)

// StatusReasonAutoRecovery is the offline reason the daemon publishes before it
// reboots a wedged reader, so a planned recovery reads differently from a failure.
const StatusReasonAutoRecovery = "auto-recovery"

// StatusReasonReboot and StatusReasonShutdown are the offline reasons for a
// reboot or shutdown an operator asked for (Reader.Reboot, Reader.Shutdown).
const (
	StatusReasonReboot   = "reboot"
	StatusReasonShutdown = "shutdown"
)

// Health is the reader-API health block of the status document.
type Health struct {
	// API is one of the API* states.
	API string `json:"api"`
	// Since is when API entered its current state (RFC 3339, the reader's clock).
	Since string `json:"since,omitempty"`
	// Detail says why the last probe was not healthy, or what error a healthy
	// /API returned (a wrong password, for example).
	Detail string `json:"detail,omitempty"`
	// Wedged is set once the API has been unhealthy past the grace window.
	Wedged bool `json:"wedged"`
	// Recovery is the last recovery action taken in this episode, if any.
	Recovery string `json:"recovery,omitempty"`
	// AutoReboots counts automatic reboots since the API was last healthy for a
	// sustained period. It survives reboots.
	AutoReboots int `json:"auto_reboots"`
	// AutoRebootsExhausted: the cap is reached. The daemon stays up and leaves
	// the decision to a person.
	AutoRebootsExhausted bool `json:"auto_reboots_exhausted"`
	// GoldenConfig is one of the Golden* states.
	GoldenConfig string `json:"golden_config"`
	// CloudServerFault lists, when non-empty, what is wrong with the golden
	// CloudServer the reader's own app publishes reads through. Some faults stop
	// reads: a trust bundle CSL cannot use, or a broker chain that does not
	// reach its first certificate. Others are drift from qos 0 / cleanSession /
	// TLSv1.2. Faults join with "; ". The daemon stays connected either way, so
	// nothing else shows them; a missing bundle makes the vendor app go silent,
	// with no log line at all.
	CloudServerFault string `json:"cloudserver_fault,omitempty"`
}

// StatusDoc is the retained document on <base>/status. The daemon's MQTT Last
// Will is the same document with only Online=false.
type StatusDoc struct {
	Online  bool   `json:"online"`
	At      string `json:"at,omitempty"`
	Version string `json:"version,omitempty"`
	// Reason accompanies a planned offline: StatusReasonAutoRecovery,
	// StatusReasonReboot or StatusReasonShutdown.
	Reason string  `json:"reason,omitempty"`
	Health *Health `json:"health,omitempty"`
}
