package readerd

import (
	"encoding/json"
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

	d.statusMu.Lock()
	busy := d.reason // an automatic recovery reboot already announced
	if busy == "" {
		busy = d.powerPending
	}
	if busy == "" {
		d.powerPending = action
	}
	d.statusMu.Unlock()
	if busy != "" {
		return readerrpc.NewError(req, readerrpc.CodeInternal, "already going down: "+busy)
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
		d.AbortGoingDown()
		return
	}
	d.log.Warn().Str("action", action).Msg("power action queued with systemd")
}

// AbortGoingDown undoes GoingDown after a reboot or power-off that did not
// happen: it clears the reason and republishes the reader as online.
func (d *Daemon) AbortGoingDown() {
	d.statusMu.Lock()
	d.reason = ""
	d.powerPending = ""
	d.statusMu.Unlock()
	d.publishStatus(d.onlinePayload())
}

// withPowerMethods advertises Reader.Reboot and Reader.Shutdown when the daemon
// has a host to run them on.
func (d *Daemon) withPowerMethods(caps readerrpc.Capabilities) readerrpc.Capabilities {
	if d.host == nil {
		return caps
	}
	power := []string{readerrpc.MethodReboot, readerrpc.MethodShutdown}
	caps.Supports = append(append([]string(nil), caps.Supports...), power...)
	var unsupported []string
	for _, m := range caps.Unsupported {
		if m != readerrpc.MethodReboot && m != readerrpc.MethodShutdown {
			unsupported = append(unsupported, m)
		}
	}
	caps.Unsupported = unsupported
	return caps
}
