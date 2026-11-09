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

// The going-down claim: one owner at a time takes the reader down. The owner is
// its reason — StatusReasonAutoRecovery for the health monitor, or the action of
// an operator's Reader.Reboot/Shutdown — and only that owner releases it, so the
// two can never take over, or undo, each other's.

// claimGoingDown takes the free claim for owner without announcing it. It
// reports the current owner when the claim is already held, by anyone: a
// second request is refused, not merged into the first.
func (d *Daemon) claimGoingDown(owner string) (held string, ok bool) {
	d.statusMu.Lock()
	defer d.statusMu.Unlock()
	if d.claim != "" {
		return d.claim, false
	}
	d.claim = owner
	return "", true
}

// GoingDown takes (or keeps) the claim for reason and publishes the retained
// offline status with it, ahead of a planned reboot or power-off. Stop keeps the
// reason. It returns false, and publishes nothing, when another owner holds it.
func (d *Daemon) GoingDown(reason string) bool {
	d.statusMu.Lock()
	if d.claim != "" && d.claim != reason {
		held := d.claim
		d.statusMu.Unlock()
		d.log.Warn().Str("reason", reason).Str("held_by", held).Msg("not going down: already going down for another reason")
		return false
	}
	d.claim, d.reason = reason, reason
	d.statusMu.Unlock()
	d.publishStatus(d.offlinePayload())
	return true
}

// AbortGoingDown releases owner's claim after a reboot or power-off that did not
// happen, and republishes the reader as online. Anyone else's claim is left
// alone.
func (d *Daemon) AbortGoingDown(owner string) {
	d.statusMu.Lock()
	if d.claim != owner {
		d.statusMu.Unlock()
		return
	}
	d.claim, d.reason = "", ""
	d.statusMu.Unlock()
	d.publishStatus(d.onlinePayload())
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
