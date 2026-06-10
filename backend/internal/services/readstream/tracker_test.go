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

func TestTracker_FreshSubscriberSeedIsEmpty(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	ch, cancel := tr.Subscribe(7, "")
	defer cancel()

	ev := recv(t, ch, time.Second)
	if ev.Type != eventSnapshot {
		t.Fatalf("first event must be snapshot, got %s", ev.Type)
	}
	var p snapshotPayload
	if err := json.Unmarshal(ev.Data, &p); err != nil {
		t.Fatalf("bad snapshot json: %v", err)
	}
	if len(p.Tags) != 0 {
		t.Fatalf("a fresh session starts empty (counts since you started watching), got %+v", p.Tags)
	}
}

// TestTracker_PerSessionCountsIndependent is the core guarantee: two operators
// tuning at once do NOT share counts, and a later-joining session does not
// inherit the earlier one's history.
func TestTracker_PerSessionCountsIndependent(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	chA, cancelA := tr.Subscribe(7, "")
	defer cancelA()
	recv(t, chA, time.Second) // A seed (empty)

	tr.Publish(7, "trakrf.id/dock-1/reads", []scanread.Read{read("EPC1", -50, 1)})
	evA := recv(t, chA, time.Second)
	if evA.Type != eventUpsert {
		t.Fatalf("A should get an upsert, got %s", evA.Type)
	}

	// B joins AFTER the first read — it must not see that read in its seed.
	chB, cancelB := tr.Subscribe(7, "")
	defer cancelB()
	seedB := recv(t, chB, time.Second)
	var pB snapshotPayload
	if err := json.Unmarshal(seedB.Data, &pB); err != nil {
		t.Fatalf("bad seed json: %v", err)
	}
	if len(pB.Tags) != 0 {
		t.Fatalf("B joined late and must start empty, got %+v", pB.Tags)
	}

	// A second read: B sees its FIRST sight (count 1), even though the tag has
	// been read twice globally — counts are per session.
	tr.Publish(7, "trakrf.id/dock-1/reads", []scanread.Read{read("EPC1", -60, 1)})
	evB := recv(t, chB, time.Second)
	var tsB TagState
	if err := json.Unmarshal(evB.Data, &tsB); err != nil {
		t.Fatalf("bad upsert json: %v", err)
	}
	if evB.Type != eventUpsert || tsB.ReadCount != 1 {
		t.Fatalf("B must count from its own connect (upsert, count 1), got %s count=%d", evB.Type, tsB.ReadCount)
	}
}

func TestTracker_PublishDeliversUpsert(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	ch, cancel := tr.Subscribe(7, "")
	defer cancel()
	recv(t, ch, time.Second) // drain the seed snapshot

	tr.Publish(7, "trakrf.id/dock-9/reads", []scanread.Read{read("EPC2", -42, 3)})

	ev := recv(t, ch, time.Second)
	if ev.Type != eventUpsert {
		t.Fatalf("want upsert, got %s", ev.Type)
	}
	var ts TagState
	if err := json.Unmarshal(ev.Data, &ts); err != nil {
		t.Fatalf("bad upsert json: %v", err)
	}
	if ts.EPC != "EPC2" || ts.ReaderKey != "trakrf.id/dock-9/reads" || ts.AntennaPort != 3 {
		t.Fatalf("unexpected upsert tag: %+v", ts)
	}
}

func TestTracker_OrgIsolation(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	chA, cancelA := tr.Subscribe(1, "")
	defer cancelA()
	chB, cancelB := tr.Subscribe(2, "")
	defer cancelB()
	recv(t, chA, time.Second) // drain snapshots
	recv(t, chB, time.Second)

	tr.Publish(1, "trakrf.id/dock-1/reads", []scanread.Read{read("EPC1", -50, 1)})

	ev := recv(t, chA, time.Second)
	if ev.Type != eventUpsert {
		t.Fatalf("org 1 subscriber should get the upsert, got %s", ev.Type)
	}
	mustNoEvent(t, chB, 100*time.Millisecond) // org 2 must not see org 1's read
}

func TestTracker_LazyDoesNotTrackWithoutSubscribers(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	// Reads arriving with nobody watching are not accumulated.
	tr.Publish(7, "trakrf.id/dock-1/reads", []scanread.Read{read("EPC1", -50, 1)})

	ch, cancel := tr.Subscribe(7, "")
	defer cancel()
	ev := recv(t, ch, time.Second)
	var p snapshotPayload
	if err := json.Unmarshal(ev.Data, &p); err != nil {
		t.Fatalf("bad snapshot json: %v", err)
	}
	if len(p.Tags) != 0 {
		t.Fatalf("nothing should be tracked before the first subscriber, got %d tags", len(p.Tags))
	}
}

func TestTracker_LazyResetsAfterLastUnsubscribe(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	ch, cancel := tr.Subscribe(7, "")
	recv(t, ch, time.Second) // seed snapshot (empty)
	tr.Publish(7, "trakrf.id/dock-1/reads", []scanread.Read{read("EPC1", -50, 1)})
	recv(t, ch, time.Second) // upsert — tag is now tracked

	cancel() // last subscriber leaves → org state discarded

	ch2, cancel2 := tr.Subscribe(7, "")
	defer cancel2()
	ev := recv(t, ch2, time.Second)
	var p snapshotPayload
	if err := json.Unmarshal(ev.Data, &p); err != nil {
		t.Fatalf("bad snapshot json: %v", err)
	}
	if len(p.Tags) != 0 {
		t.Fatalf("counts must reset when nobody is watching; got %d tags on re-subscribe", len(p.Tags))
	}
}

func TestTracker_ScopedSubscriberOnlySeesItsReader(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	ch, cancel := tr.Subscribe(7, "trakrf.id/dock-1/reads") // scoped to dock-1's topic
	defer cancel()
	recv(t, ch, time.Second) // seed

	// A read from another reader must not reach (or be tracked by) this session.
	tr.Publish(7, "trakrf.id/dock-2/reads", []scanread.Read{read("OTHER", -50, 1)})
	mustNoEvent(t, ch, 100*time.Millisecond)

	// A read from the scoped reader does.
	tr.Publish(7, "trakrf.id/dock-1/reads", []scanread.Read{read("MINE", -50, 1)})
	ev := recv(t, ch, time.Second)
	var ts TagState
	if err := json.Unmarshal(ev.Data, &ts); err != nil {
		t.Fatalf("bad upsert json: %v", err)
	}
	if ev.Type != eventUpsert || ts.EPC != "MINE" || ts.ReaderKey != "trakrf.id/dock-1/reads" {
		t.Fatalf("scoped session should see only dock-1's read, got %s %+v", ev.Type, ts)
	}
}

func TestTracker_CancelStopsDelivery(t *testing.T) {
	tr := NewTracker(idleCfg())
	defer tr.Stop()

	ch, cancel := tr.Subscribe(7, "")
	recv(t, ch, time.Second) // snapshot
	cancel()

	tr.Publish(7, "trakrf.id/dock-1/reads", []scanread.Read{read("EPC1", -50, 1)})
	mustNoEvent(t, ch, 100*time.Millisecond)
}
