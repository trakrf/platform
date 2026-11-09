package readerd

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// DefaultPowerDelay is how long Reader.Reboot and Reader.Shutdown wait after
// replying before they act, so the caller gets the acknowledgement before the
// daemon's broker connection goes away.
const DefaultPowerDelay = 2 * time.Second

// SetHost enables Reader.Reboot and Reader.Shutdown. Call before Start.
func (d *Daemon) SetHost(h Host) { d.host = h }

// power handles Reader.Reboot and Reader.Shutdown: validate, refuse a second
// request while one is under way, log who asked, reply, and act after
// powerDelay. action is StatusReasonReboot or StatusReasonShutdown.
func (d *Daemon) power(req readerrpc.Request, action string) readerrpc.Response {
	if d.host == nil {
		return readerrpc.NewError(req, readerrpc.CodeMethodNotFound, "unsupported method: "+req.Method)
	}
	var p readerrpc.PowerParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return readerrpc.NewError(req, readerrpc.CodeInvalidParams, "invalid "+req.Method+" params: "+err.Error())
		}
	}

	if state := d.updateUnderWay(); state != "" {
		return readerrpc.NewError(req, readerrpc.CodePowerRefused, "a daemon update is under way ("+state+"); try again once it has settled")
	}
	if held, ok := d.claimGoingDown(action); !ok {
		return readerrpc.NewError(req, readerrpc.CodePowerRefused, "already going down: "+held)
	}

	delay := d.powerDelay
	d.log.Warn().
		Str("method", req.Method).
		Str("requested_by", p.RequestedBy).
		Str("src", req.Src).
		Int("id", req.ID).
		Time("asked_at", d.now().UTC()).
		Dur("in", delay).
		Msg("power action requested over rpc")
	d.after(delay, func() { d.powerNow(action) })
	return d.result(req, readerrpc.PowerResult{Action: action, InMs: int(delay / time.Millisecond)})
}

// powerNow announces the planned outage on the retained status, then asks
// systemd. If systemd refuses, the reader is still up, so it must not stay
// announced as offline.
func (d *Daemon) powerNow(action string) {
	d.GoingDown(action)
	run := d.host.Reboot
	if action == readerrpc.StatusReasonShutdown {
		run = d.host.Poweroff
	}
	if err := run(); err != nil {
		d.log.Error().Err(err).Str("action", action).Msg("power action failed; the reader stays up")
		d.AbortGoingDown(action)
		return
	}
	d.log.Warn().Str("action", action).Msg("power action queued with systemd")
}

// updateUnderWay names the state of a daemon update that a reboot must not
// interrupt: downloading, or installing (the binary swap). "" when there is
// none. Both end on their own: in failure, or in the daemon's exit.
//
// A build on probation is deliberately not refused. Probation ends only when
// the build commits or the guard rolls back, which can take 15–20 min and, on a
// read-only filesystem, never happens; a reboot is what fixes that, and the
// guard counts the extra start and still rolls back across it.
func (d *Daemon) updateUnderWay() string {
	if d.updater == nil {
		return ""
	}
	if u := d.updater.Info().Update; u != nil {
		switch u.State {
		case readerrpc.UpdateStateDownloading, readerrpc.UpdateStateInstalling:
			return u.State
		}
	}
	return ""
}

// goingDown names the owner of the going-down claim; "" when it is free.
func (d *Daemon) goingDown() string {
	d.statusMu.Lock()
	defer d.statusMu.Unlock()
	return d.claim
}

// withPowerMethods advertises Reader.Reboot and Reader.Shutdown when the daemon
// has a host to run them on.
func (d *Daemon) withPowerMethods(caps readerrpc.Capabilities) readerrpc.Capabilities {
	if d.host == nil {
		return caps
	}
	supports := append([]string(nil), caps.Supports...)
	for _, m := range []string{readerrpc.MethodReboot, readerrpc.MethodShutdown} {
		if !slices.Contains(supports, m) {
			supports = append(supports, m)
		}
	}
	caps.Supports = supports
	var unsupported []string
	for _, m := range caps.Unsupported {
		if m != readerrpc.MethodReboot && m != readerrpc.MethodShutdown {
			unsupported = append(unsupported, m)
		}
	}
	caps.Unsupported = unsupported
	return caps
}
