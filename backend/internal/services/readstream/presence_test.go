package readstream

import (
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

func read(epc string, rssi, antenna int) scanread.Read {
	return scanread.Read{EPC: epc, RSSI: rssi, AntennaPort: antenna}
}

// byKey indexes orgEvents by their tag key for order-independent assertions.
func byKey(evs []orgEvent) map[string]orgEvent {
	m := make(map[string]orgEvent, len(evs))
	for _, e := range evs {
		m[tagKey(e.tag.ReaderKey, e.tag.EPC, e.tag.AntennaPort)] = e
	}
	return m
}

func TestIngest_FirstSightEmitsEnter(t *testing.T) {
	s := newStore(30*time.Second, time.Second)
	t0 := time.UnixMilli(1_000_000)

	evs := s.ingest(7, "dock-1", read("EPC1", -50, 2), t0)

	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	e := evs[0]
	if e.orgID != 7 || e.typ != eventUpsert {
		t.Fatalf("want enter for org 7, got %v org=%d", e.typ, e.orgID)
	}
	ts := e.tag
	if ts.ReaderKey != "dock-1" || ts.EPC != "EPC1" || ts.AntennaPort != 2 {
		t.Fatalf("unexpected identity: %+v", ts)
	}
	if ts.ReadCount != 1 {
		t.Fatalf("want ReadCount 1, got %d", ts.ReadCount)
	}
	if ts.LastRSSI != -50 || ts.RSSIMin != -50 || ts.RSSIMax != -50 || ts.RSSIAvg != -50 {
		t.Fatalf("want all rssi -50, got %+v", ts)
	}
	if ts.FirstSeenMs != t0.UnixMilli() || ts.LastSeenMs != t0.UnixMilli() {
		t.Fatalf("want first==last==%d, got first=%d last=%d", t0.UnixMilli(), ts.FirstSeenMs, ts.LastSeenMs)
	}
}

func TestIngest_ResightAggregatesAndCoalesces(t *testing.T) {
	s := newStore(30*time.Second, time.Second)
	t0 := time.UnixMilli(1_000_000)
	s.ingest(7, "dock-1", read("EPC1", -50, 1), t0)

	// re-sight 100ms later with a stronger then weaker reading (same antenna, so
	// these fold into one row — antenna is part of the key as of TRA-937).
	if evs := s.ingest(7, "dock-1", read("EPC1", -40, 1), t0.Add(100*time.Millisecond)); evs != nil {
		t.Fatalf("re-sight must not emit immediately (coalesced), got %d events", len(evs))
	}
	s.ingest(7, "dock-1", read("EPC1", -60, 1), t0.Add(200*time.Millisecond))

	snap := s.snapshot(7)
	if len(snap) != 1 {
		t.Fatalf("want 1 tag, got %d", len(snap))
	}
	ts := snap[0]
	if ts.ReadCount != 3 {
		t.Fatalf("want ReadCount 3, got %d", ts.ReadCount)
	}
	if ts.LastRSSI != -60 {
		t.Fatalf("want LastRSSI -60, got %d", ts.LastRSSI)
	}
	if ts.RSSIMax != -40 || ts.RSSIMin != -60 {
		t.Fatalf("want max -40/min -60, got max=%d min=%d", ts.RSSIMax, ts.RSSIMin)
	}
	if ts.RSSIAvg != -50 { // (-50 + -40 + -60)/3
		t.Fatalf("want RSSIAvg -50, got %d", ts.RSSIAvg)
	}
	if ts.AntennaPort != 1 || ts.LastSeenMs != t0.Add(200*time.Millisecond).UnixMilli() {
		t.Fatalf("want antenna 1 and refreshed lastSeen, got %+v", ts)
	}
}

func TestFlush_EmitsCoalescedUpdateAfterInterval(t *testing.T) {
	s := newStore(30*time.Second, time.Second)
	t0 := time.UnixMilli(1_000_000)
	s.ingest(7, "dock-1", read("EPC1", -50, 1), t0)
	s.ingest(7, "dock-1", read("EPC1", -55, 1), t0.Add(100*time.Millisecond))

	// before the coalesce interval elapses: no UPDATE
	if evs := s.flush(t0.Add(500 * time.Millisecond)); len(evs) != 0 {
		t.Fatalf("want no update before coalesce interval, got %d", len(evs))
	}
	// after the interval: exactly one UPDATE carrying current state
	evs := s.flush(t0.Add(1100 * time.Millisecond))
	if len(evs) != 1 || evs[0].typ != eventUpsert {
		t.Fatalf("want 1 update, got %d (%v)", len(evs), evs)
	}
	if evs[0].tag.ReadCount != 2 || evs[0].tag.LastRSSI != -55 {
		t.Fatalf("update must carry current state, got %+v", evs[0].tag)
	}
	// nothing dirty now → no further update
	if evs := s.flush(t0.Add(2200 * time.Millisecond)); len(evs) != 0 {
		t.Fatalf("want no update when clean, got %d", len(evs))
	}
}

func TestSweep_ExpiresAndEmitsLeave(t *testing.T) {
	s := newStore(30*time.Second, time.Second)
	t0 := time.UnixMilli(1_000_000)
	s.ingest(7, "dock-1", read("EPC1", -50, 4), t0)
	s.ingest(7, "dock-1", read("EPC2", -50, 1), t0)
	// refresh EPC2 so only EPC1 ages out
	s.ingest(7, "dock-1", read("EPC2", -50, 1), t0.Add(20*time.Second))

	evs := s.sweep(t0.Add(31 * time.Second))
	if len(evs) != 1 {
		t.Fatalf("want exactly one LEAVE, got %d", len(evs))
	}
	// The LEAVE must identify the evicted row fully — including its antenna, since
	// (reader,epc,antenna) is the row identity (TRA-937).
	if evs[0].typ != eventLeave || evs[0].tag.EPC != "EPC1" || evs[0].tag.AntennaPort != 4 {
		t.Fatalf("want LEAVE for EPC1 antenna 4, got %v %s ant=%d", evs[0].typ, evs[0].tag.EPC, evs[0].tag.AntennaPort)
	}
	snap := s.snapshot(7)
	if len(snap) != 1 || snap[0].EPC != "EPC2" {
		t.Fatalf("EPC1 should be evicted, EPC2 remain; got %+v", snap)
	}
}

func TestKeying_SameEPCDifferentReadersAreDistinct(t *testing.T) {
	s := newStore(30*time.Second, time.Second)
	t0 := time.UnixMilli(1_000_000)
	s.ingest(7, "dock-1", read("EPC1", -50, 1), t0)
	evs := s.ingest(7, "dock-2", read("EPC1", -50, 1), t0)
	if len(evs) != 1 || evs[0].typ != eventUpsert {
		t.Fatalf("same EPC at a second reader must ENTER as a distinct tag, got %v", evs)
	}
	if len(s.snapshot(7)) != 2 {
		t.Fatalf("want 2 distinct (reader,epc) tags, got %d", len(s.snapshot(7)))
	}
}

func TestKeying_SameEPCSameReaderDifferentAntennaAreDistinct(t *testing.T) {
	s := newStore(30*time.Second, time.Second)
	t0 := time.UnixMilli(1_000_000)
	s.ingest(7, "dock-1", read("EPC1", -50, 1), t0)
	// Same tag at the same reader but a different antenna port is a distinct row:
	// the split view exists precisely to compare per-antenna sensitivity (TRA-937).
	evs := s.ingest(7, "dock-1", read("EPC1", -55, 2), t0)
	if len(evs) != 1 || evs[0].typ != eventUpsert {
		t.Fatalf("same EPC at a second antenna must ENTER as a distinct row, got %v", evs)
	}
	if len(s.snapshot(7)) != 2 {
		t.Fatalf("want 2 distinct (reader,epc,antenna) rows, got %d", len(s.snapshot(7)))
	}
}

func TestOrgIsolation(t *testing.T) {
	s := newStore(30*time.Second, time.Second)
	t0 := time.UnixMilli(1_000_000)
	s.ingest(1, "dock-1", read("EPC1", -50, 1), t0)
	s.ingest(2, "dock-1", read("EPC2", -50, 1), t0)
	if len(s.snapshot(1)) != 1 || s.snapshot(1)[0].EPC != "EPC1" {
		t.Fatalf("org 1 should only see EPC1")
	}
	if len(s.snapshot(2)) != 1 || s.snapshot(2)[0].EPC != "EPC2" {
		t.Fatalf("org 2 should only see EPC2")
	}
}
