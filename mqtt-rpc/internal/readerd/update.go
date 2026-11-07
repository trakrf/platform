package readerd

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// selfTestInterval is how often a daemon on probation re-sends its loopback RPC.
const selfTestInterval = 30 * time.Second

// Updater is the daemon's remote self-update (internal/update.Updater). It is
// optional: a daemon without one does not advertise Daemon.Update.
type Updater interface {
	Start(readerrpc.DaemonUpdateParams) (readerrpc.DaemonUpdateResult, error)
	Info() readerrpc.DaemonInfo
	OnProbation() bool
	Commit() error
}

// SetUpdater enables Daemon.Update and the post-update commit. Call before Start.
func (d *Daemon) SetUpdater(u Updater) { d.updater = u }

func (d *Daemon) selfTestTopic() string { return d.cfg.Broker.BaseTopic + "/selftest" }

// selfTest sends this daemon a Reader.GetCapabilities through the broker, to its
// own RPC topic, while it is on probation after an update. Answering it proves
// connect, subscribe, dispatch and publish end to end — and GetCapabilities never
// touches the vendor /API, so a wedged reader app cannot fail the check and roll
// back the very build meant to fix it. Any cloud RPC answered on probation counts
// the same way.
func (d *Daemon) selfTest() error {
	if d.updater == nil || !d.updater.OnProbation() || d.publish == nil {
		return nil
	}
	b, err := json.Marshal(readerrpc.Request{Src: d.selfTestTopic(), Method: readerrpc.MethodGetCapabilities})
	if err != nil {
		return err
	}
	return d.publish(readerrpc.RPCTopic(d.cfg.Broker.BaseTopic), b)
}

// probationLoop re-sends the self-test until the daemon commits or stops, in
// case the first one raced the subscription.
func (d *Daemon) probationLoop() {
	t := time.NewTicker(selfTestInterval)
	defer t.Stop()
	for {
		select {
		case <-d.stopCh:
			return
		case <-t.C:
			if !d.updater.OnProbation() {
				return
			}
			if d.client != nil && d.client.IsConnected() {
				if err := d.selfTest(); err != nil {
					d.log.Warn().Err(err).Msg("update self-test publish failed")
				}
			}
		}
	}
}

// answered is called after a reply was accepted by the broker. On probation that
// is the commit criterion.
func (d *Daemon) answered() {
	if d.updater == nil || !d.updater.OnProbation() {
		return
	}
	if err := d.updater.Commit(); err != nil {
		d.log.Error().Err(err).Msg("update commit failed; the guard will roll back if this persists")
		return
	}
	d.log.Info().Str("version", d.cfg.Version).Msg("update committed: answered an rpc through the broker")
}

// daemonUpdate handles Daemon.Update.
func (d *Daemon) daemonUpdate(req readerrpc.Request) readerrpc.Response {
	if d.updater == nil {
		return readerrpc.NewError(req, readerrpc.CodeMethodNotFound, "unsupported method: "+req.Method)
	}
	var p readerrpc.DaemonUpdateParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return readerrpc.NewError(req, readerrpc.CodeInvalidParams, "invalid Daemon.Update params: "+err.Error())
	}
	res, err := d.updater.Start(p)
	if err != nil {
		var re *readerrpc.UpdateRefusedError
		if errors.As(err, &re) {
			d.log.Warn().Str("to", p.Version).Str("reason", re.Reason).Msg("update refused")
			return readerrpc.NewError(req, readerrpc.CodeUpdateRefused, re.Error())
		}
		return readerrpc.NewError(req, readerrpc.CodeInternal, err.Error())
	}
	return d.result(req, res)
}

// withDaemonInfo adds the daemon's own identity to the adapter's capabilities.
func (d *Daemon) withDaemonInfo(caps readerrpc.Capabilities) readerrpc.Capabilities {
	if d.updater == nil {
		caps.Daemon = &readerrpc.DaemonInfo{Version: d.cfg.Version}
		return caps
	}
	info := d.updater.Info()
	caps.Daemon = &info
	caps.Supports = append(append([]string(nil), caps.Supports...), readerrpc.MethodDaemonUpdate)
	return caps
}
