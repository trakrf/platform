package readerd

import (
	"encoding/json"
	"time"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// The retained <base>/status document (readerrpc.StatusDoc): presence, the
// daemon version, the reader-API health from the monitor, and the reason for a
// planned offline.

// SetHealth records the monitor's latest reader-API health and republishes the
// retained status. Once the daemon is going down it republishes offline, so a
// late health change cannot announce a rebooting reader as online.
func (d *Daemon) SetHealth(h readerrpc.Health) {
	d.statusMu.Lock()
	d.health = &h
	goingDown := d.reason != ""
	d.statusMu.Unlock()
	if goingDown {
		d.publishStatus(d.offlinePayload())
		return
	}
	d.publishStatus(d.onlinePayload())
}

// GoingDown publishes the retained offline status with a reason (auto-recovery)
// ahead of a planned reboot. Stop keeps the reason.
func (d *Daemon) GoingDown(reason string) {
	d.statusMu.Lock()
	d.reason = reason
	d.statusMu.Unlock()
	d.publishStatus(d.offlinePayload())
}

func (d *Daemon) publishStatus(b []byte) {
	if d.publishRetained == nil {
		return // not started; OnConnect publishes the current document
	}
	if err := d.publishRetained(readerrpc.StatusTopic(d.cfg.Broker.BaseTopic), b); err != nil {
		d.log.Warn().Err(err).Msg("publish status failed")
	}
}

func (d *Daemon) statusDoc(online bool) []byte {
	d.statusMu.Lock()
	doc := readerrpc.StatusDoc{Online: online, Version: d.cfg.Version, Health: d.health}
	if !online {
		doc.Reason = d.reason
	}
	d.statusMu.Unlock()
	if online {
		doc.At = d.now().UTC().Format(time.RFC3339)
	}
	b, _ := json.Marshal(doc)
	return b
}

func (d *Daemon) onlinePayload() []byte  { return d.statusDoc(true) }
func (d *Daemon) offlinePayload() []byte { return d.statusDoc(false) }

// lastWillPayload is what the broker publishes if the daemon drops without Stop.
func lastWillPayload() []byte {
	b, _ := json.Marshal(readerrpc.StatusDoc{Online: false})
	return b
}
