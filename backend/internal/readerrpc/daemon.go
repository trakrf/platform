package readerrpc

// Daemon-level methods: these act on the on-reader daemon itself, not on the
// reader it controls. None of them may touch the reader's native API — they must
// keep working while the vendor app is wedged, which is exactly when a fix has to
// be pushed.

// MethodDaemonUpdate replaces the daemon binary with a signed release.
const MethodDaemonUpdate = "Daemon.Update"

// CodeUpdateRefused signals that a Daemon.Update request was refused before
// anything was replaced (bad signature, bad hash, already updating, missing
// rollback guard, ...). It is outside the JSON-RPC reserved range.
const CodeUpdateRefused = -33002

// DaemonUpdateParams are the params for Daemon.Update: one signed release.
// Signature is base64 Ed25519 over the release message (version + sha256).
type DaemonUpdateParams struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}

// DaemonUpdateResult is the response to Daemon.Update. The reply is sent once
// the request has been verified and accepted; the download, swap and restart
// happen after it, so the caller watches Capabilities.Daemon for the outcome.
type DaemonUpdateResult struct {
	Accepted bool   `json:"accepted"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// Update states reported in UpdateInfo.State.
const (
	UpdateStateDownloading = "downloading"
	UpdateStateInstalling  = "installing"
	// UpdateStatePending: the new daemon is running but has not yet committed;
	// the rollback guard is still armed.
	UpdateStatePending    = "pending"
	UpdateStateCommitted  = "committed"
	UpdateStateRolledBack = "rolled_back"
	UpdateStateFailed     = "failed"
	// UpdateStateUnguarded: the target never committed, and the guard found no
	// other installed version to roll back to, so the target stays live with no
	// rollback armed. Unlike failed, the target DID replace the old build.
	UpdateStateUnguarded = "unguarded"
)

// UpdateInfo describes the most recent update attempt.
type UpdateInfo struct {
	State  string `json:"state"`
	Target string `json:"target,omitempty"`
	Error  string `json:"error,omitempty"`
	At     string `json:"at,omitempty"`
}

// DaemonInfo is the daemon's own identity, reported in Capabilities.
type DaemonInfo struct {
	Version string      `json:"version"`
	Update  *UpdateInfo `json:"update,omitempty"`
}

// UpdateRefusedError is the typed error for a refused Daemon.Update. The daemon
// serializes it to a CodeUpdateRefused frame; the cloud client reconstructs it.
type UpdateRefusedError struct{ Reason string }

func (e *UpdateRefusedError) Error() string { return "update refused: " + e.Reason }
