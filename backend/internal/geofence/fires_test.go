package geofence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

func reads(rs ...storage.ResolvedRead) []storage.ResolvedRead { return rs }

func TestDecide_DrivesNothing(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, pulse5s)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	fires := e.decide(context.Background(), 42, time.Unix(1000, 0), reads(read("EPC1", -50)))

	if len(fires) != 1 {
		t.Fatalf("an egress read must give one fire, got %d", len(fires))
	}
	if fires[0].read.EPC != "EPC1" {
		t.Fatalf("the fire must carry its read, got %s", fires[0].read.EPC)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.calls) != 0 {
		t.Fatalf("decide must not drive, got %d calls", len(d.calls))
	}
	if len(s.rows) != 0 {
		t.Fatalf("decide must not write alarms, got %d rows", len(s.rows))
	}
}

func TestDecide_LatchedReadAndGraceGiveNoFire(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{})
	defer e.Stop()

	at := time.Unix(1000, 0)
	if got := e.decide(context.Background(), 42, at, reads(read("EPC1", -50))); len(got) != 1 {
		t.Fatalf("first sight must fire, got %d", len(got))
	}
	if got := e.decide(context.Background(), 42, at.Add(time.Second), reads(read("EPC1", -50))); len(got) != 0 {
		t.Fatalf("a read that only refreshes the latch is not a fire, got %d", len(got))
	}

	graced := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute, StartupGrace: time.Minute}, s, &fakeDriver{})
	defer graced.Stop()
	if got := graced.decide(context.Background(), 42, at, reads(read("EPC1", -50))); len(got) != 0 {
		t.Fatalf("a read inside the startup grace is not a fire, got %d", len(got))
	}
}

func TestFireUndo_EgressRestoresLatch(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{})
	defer e.Stop()

	// The tag was seen long ago, so this read is a re-entry over an existing entry.
	at := time.Unix(1000, 0)
	e.decide(context.Background(), 42, at, reads(read("EPC1", -50)))
	reentry := at.Add(2 * time.Minute)

	fires := e.decide(context.Background(), 42, reentry, reads(read("EPC1", -50)))
	if len(fires) != 1 {
		t.Fatalf("a re-entry must fire, got %d", len(fires))
	}
	fires[0].undo()

	if got := e.decide(context.Background(), 42, reentry, reads(read("EPC1", -50))); len(got) != 1 {
		t.Fatalf("after undo the same read must fire again, got %d", len(got))
	}
}

func TestFireUndo_FirstSightRemovesLatchEntry(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{})
	defer e.Stop()

	fires := e.decide(context.Background(), 42, time.Unix(1000, 0), reads(read("EPC1", -50)))
	if e.latch.size() != 1 {
		t.Fatalf("decide must latch the tag, size=%d", e.latch.size())
	}
	fires[0].undo()

	if e.latch.size() != 0 {
		t.Fatalf("undoing a first sight must remove the entry, size=%d", e.latch.size())
	}
}

func TestFireUndo_ReleasesPulse(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, pulse5s)}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{})
	defer e.Stop()

	at := time.Unix(1000, 0)
	fires := e.decide(context.Background(), 42, at, reads(read("EPC1", -50)))
	if got := e.decide(context.Background(), 42, at.Add(time.Second), reads(read("EPC2", -50))); len(got) != 0 {
		t.Fatalf("the pulse must hold the output while the fire stands, got %d", len(got))
	}
	fires[0].undo()

	if got := e.decide(context.Background(), 42, at.Add(time.Second), reads(read("EPC3", -50))); len(got) != 1 {
		t.Fatalf("after undo another tag must fire the output, got %d", len(got))
	}
}

func TestFireUndo_PresenceForgetsOutput(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, map[string]any{"mode": "presence"})}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()
	p, timers := newTestPresence(d)
	e.presence = p

	at := time.Unix(1000, 0)
	fires := e.decide(context.Background(), 42, at, reads(read("EPC1", -50)))
	if len(fires) != 1 || len(*timers) != 1 {
		t.Fatalf("presence must fire once and arm a timer, got fires=%d timers=%d", len(fires), len(*timers))
	}
	fires[0].undo()

	if !(*timers)[0].stopped {
		t.Fatal("undo must stop the age-out timer")
	}
	if d.offCount() != 0 {
		t.Fatalf("undo must not drive the output off, got %d", d.offCount())
	}
	if got := e.decide(context.Background(), 42, at, reads(read("EPC1", -50))); len(got) != 1 {
		t.Fatalf("after undo the next read must fire, got %d", len(got))
	}
}

func TestFireUndo_CoversEveryOutputOfTheRead(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{
		dev(11, pulse5s), dev(12, map[string]any{"mode": "presence"}),
	}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{})
	defer e.Stop()

	at := time.Unix(1000, 0)
	fires := e.decide(context.Background(), 42, at, reads(read("EPC1", -50)))
	if len(fires) != 1 || len(fires[0].drives) != 2 {
		t.Fatalf("one read on two outputs must be one fire with two drives, got %+v", fires)
	}
	fires[0].undo()

	again := e.decide(context.Background(), 42, at, reads(read("EPC1", -50)))
	if len(again) != 1 || len(again[0].drives) != 2 {
		t.Fatalf("after undo both outputs must fire again, got %+v", again)
	}
}

func TestDriveFire_DrivesEachOutputWithItsAutoOff(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{
		dev(11, pulse5s),
		dev(12, map[string]any{"mode": "presence", "auto_off_seconds": float64(99)}),
	}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	fires := e.decide(context.Background(), 42, time.Unix(1000, 0), reads(read("EPC1", -50)))
	e.driveFire(context.Background(), 42, fires[0])

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.calls) != 2 {
		t.Fatalf("expected two drives, got %d", len(d.calls))
	}
	if c := d.calls[0]; c.dev.ID != 11 || !c.on || c.offAfter != 5 {
		t.Fatalf("egress must drive ON with its auto-off, got %+v", c)
	}
	if c := d.calls[1]; c.dev.ID != 12 || !c.on || c.offAfter != 0 {
		t.Fatalf("presence must drive ON with no auto-off, got %+v", c)
	}
}

func TestDriveFire_FailedDriveReleasesPulse(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, pulse5s)}}
	d := &fakeDriver{err: errors.New("device offline")}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	at := time.Unix(1000, 0)
	fires := e.decide(context.Background(), 42, at, reads(read("EPC1", -50)))
	e.driveFire(context.Background(), 42, fires[0])

	if got := e.decide(context.Background(), 42, at.Add(time.Second), reads(read("EPC2", -50))); len(got) != 1 {
		t.Fatalf("a failed drive must free the output for the next tag, got %d", len(got))
	}
}

// The whole message is decided before anything drives, so a failed drive frees
// the output for the next message, not for a later read of the same message.
func TestEvaluate_FailedDriveFreesOutputAfterTheMessage(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, pulse5s)}}
	d := &fakeDriver{err: errors.New("device offline")}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	before := pulseActiveDrops(t)
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), reads(read("EPC1", -50), read("EPC2", -50)))

	if len(s.rows) != 1 || s.rows[0].EPC != "EPC1" {
		t.Fatalf("only the first tag fires, and a failed drive still records it, got %+v", s.rows)
	}
	if got := pulseActiveDrops(t) - before; got != 1 {
		t.Fatalf("expected the second tag to be dropped, got %v drops", got)
	}
}

func TestAlarmRow_CarriesTheFiredRead(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{})
	defer e.Stop()

	at := time.Unix(1000, 0)
	fires := e.decide(context.Background(), 42, at, reads(read("EPC1", -50)))
	row := e.alarmRow(fires[0], 555, at)

	if row.AssetID != 7 || row.ScanPointID != 3 || row.LocationID == nil || *row.LocationID != 9 ||
		row.EPC != "EPC1" || row.RSSI != -50 || row.TagScanID != 555 || !row.FiredAt.Equal(at) {
		t.Fatalf("alarm row fields wrong: %+v", row)
	}
}

func TestLatchSnapshotRestore(t *testing.T) {
	l := newLatch(0, nil)
	defer l.Close()

	key := latchKey(1, 2, "EPC")
	if _, ok := l.snapshot(key); ok {
		t.Fatal("an unseen key must have no snapshot")
	}

	// Restoring an absent snapshot removes the entry admit created.
	base := time.Unix(100, 0)
	lastSeen, ok := l.snapshot(key)
	l.admit(key, base, testTTL)
	l.restore(key, lastSeen, ok)
	if l.size() != 0 {
		t.Fatalf("restore of an absent key must delete it, size=%d", l.size())
	}

	// Restoring a present snapshot puts lastSeen back, so the re-entry re-arms.
	l.admit(key, base, testTTL)
	lastSeen, ok = l.snapshot(key)
	if !ok || lastSeen != base.UnixNano() {
		t.Fatalf("snapshot must report the last sighting, got %d ok=%v", lastSeen, ok)
	}
	reentry := base.Add(2 * testTTL)
	if !l.admit(key, reentry, testTTL) {
		t.Fatal("a sighting past the TTL must fire")
	}
	l.restore(key, lastSeen, ok)
	if !l.admit(key, reentry, testTTL) {
		t.Fatal("after restore the same sighting must fire again")
	}
}
