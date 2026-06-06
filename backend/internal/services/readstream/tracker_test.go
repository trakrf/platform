package readstream

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// idleCfg keeps the background tick/keyframe loops effectively dormant so tests
// drive state through Publish/Subscribe only.
func idleCfg() TrackerConfig {
	return TrackerConfig{TTL: time.Minute, Coalesce: time.Second, TickInterval: time.Hour, KeyframeInterval: time.Hour}
}

func recv(t *testing.T, ch <-chan Event, within time.Duration) Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(within):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func mustNoEvent(t *testing.T, ch <-chan Event, within time.Duration) {
	t.Helper()
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event: %s %s", ev.Type, ev.Data)
	case <-time.After(within):
	}
}

func TestTracker_SubscribeSeedsSnapshot(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()
	tr.Publish(7, "trakrf.id/dock-1/reads", []scanread.Read{read("EPC1", -50, 1)})

	ch, cancel := tr.Subscribe(7)
	defer cancel()

	ev := recv(t, ch, time.Second)
	if ev.Type != eventSnapshot {
		t.Fatalf("first event must be snapshot, got %s", ev.Type)
	}
	var p snapshotPayload
	if err := json.Unmarshal(ev.Data, &p); err != nil {
		t.Fatalf("bad snapshot json: %v", err)
	}
	if len(p.Tags) != 1 || p.Tags[0].EPC != "EPC1" {
		t.Fatalf("snapshot must contain EPC1, got %+v", p.Tags)
	}
	if p.UniqueTags != 1 {
		t.Fatalf("want uniqueTags 1, got %d", p.UniqueTags)
	}
}

func TestTracker_PublishDeliversEnter(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	ch, cancel := tr.Subscribe(7)
	defer cancel()
	recv(t, ch, time.Second) // drain the seed snapshot

	tr.Publish(7, "trakrf.id/dock-9/reads", []scanread.Read{read("EPC2", -42, 3)})

	ev := recv(t, ch, time.Second)
	if ev.Type != eventEnter {
		t.Fatalf("want enter, got %s", ev.Type)
	}
	var ts TagState
	if err := json.Unmarshal(ev.Data, &ts); err != nil {
		t.Fatalf("bad enter json: %v", err)
	}
	if ts.EPC != "EPC2" || ts.ReaderKey != "dock-9" || ts.AntennaPort != 3 {
		t.Fatalf("unexpected enter tag: %+v", ts)
	}
}

func TestTracker_OrgIsolation(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	chA, cancelA := tr.Subscribe(1)
	defer cancelA()
	chB, cancelB := tr.Subscribe(2)
	defer cancelB()
	recv(t, chA, time.Second) // drain snapshots
	recv(t, chB, time.Second)

	tr.Publish(1, "trakrf.id/dock-1/reads", []scanread.Read{read("EPC1", -50, 1)})

	ev := recv(t, chA, time.Second)
	if ev.Type != eventEnter {
		t.Fatalf("org 1 subscriber should get the enter, got %s", ev.Type)
	}
	mustNoEvent(t, chB, 100*time.Millisecond) // org 2 must not see org 1's read
}

func TestTracker_CancelStopsDelivery(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	ch, cancel := tr.Subscribe(7)
	recv(t, ch, time.Second) // snapshot
	cancel()

	tr.Publish(7, "trakrf.id/dock-1/reads", []scanread.Read{read("EPC1", -50, 1)})
	mustNoEvent(t, ch, 100*time.Millisecond)
}
