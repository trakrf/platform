// Package readerpower defines the MQTT wire contract shared by the backend
// reader-control seam and the standalone power agent: the command/state message
// shapes and the per-reader topic derivation. Leaf package — stdlib only — so
// both the lean agent and the DB-bound backend can import it without pulling
// each other's dependencies.
package readerpower

// Topic suffixes hung off a reader's publish_topic (TRA-922 reader key).
const (
	commandSuffix = "/command/power"
	stateSuffix   = "/state/power"
)

// Status values reported by the agent on the state topic.
const (
	StatusOK    = "ok"
	StatusBusy  = "busy"
	StatusError = "error"
)

// CommandTopic is where the backend publishes power commands and the agent
// subscribes, for the reader identified by publishTopic.
func CommandTopic(publishTopic string) string { return publishTopic + commandSuffix }

// StateTopic is where the agent publishes results and the backend subscribes.
func StateTopic(publishTopic string) string { return publishTopic + stateSuffix }

// Command is published by the backend to set (or, with empty Powers, get)
// per-antenna transmit power. Powers maps antenna port ("1".."16") to dBm.
type Command struct {
	RequestID string             `json:"request_id"`
	Powers    map[string]float64 `json:"powers,omitempty"`
	Force     bool               `json:"force,omitempty"`
}

// State is published by the agent after handling a command.
type State struct {
	RequestID     string             `json:"request_id"`
	Status        string             `json:"status"` // ok | busy | error
	ActiveProfile string             `json:"active_profile,omitempty"`
	Powers        map[string]float64 `json:"powers,omitempty"`
	HolderIP      string             `json:"holder_ip,omitempty"`
	Error         string             `json:"error,omitempty"`
	Timestamp     string             `json:"ts,omitempty"` // RFC3339
}
