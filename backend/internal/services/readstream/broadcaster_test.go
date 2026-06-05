package readstream

import (
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

func reads(epcs ...string) []scanread.Read {
	out := make([]scanread.Read, 0, len(epcs))
	for _, e := range epcs {
		out = append(out, scanread.Read{
			EPC:              e,
			CapturePointName: "dock",
			AntennaPort:      1,
			RSSI:             -55,
			ReaderTimestamp:  time.UnixMilli(1_700_000_000_000),
		})
	}
	return out
}

func TestBroadcaster_OrgIsolation(t *testing.T) {
	b := New()
	chA, cancelA := b.Subscribe(1)
	defer cancelA()
	chB, cancelB := b.Subscribe(2)
	defer cancelB()

	b.Publish(1, "trakrf.id/dock-1/reads", reads("EPC-A"))

	select {
	case ev := <-chA:
		if ev.EPC != "EPC-A" || ev.ReaderKey != "dock-1" {
			t.Fatalf("org 1 got wrong event: %+v", ev)
		}
		if ev.ReaderTimestampMs != 1_700_000_000_000 {
			t.Fatalf("bad ts ms: %d", ev.ReaderTimestampMs)
		}
	case <-time.After(time.Second):
		t.Fatal("org 1 did not receive its event")
	}

	select {
	case ev := <-chB:
		t.Fatalf("org 2 leaked an event: %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBroadcaster_DropsWhenFull(t *testing.T) {
	b := New()
	_, cancel := b.Subscribe(1) // never drained
	defer cancel()
	// Far more than the buffer; must not block or panic.
	for i := 0; i < clientBuffer*4; i++ {
		b.Publish(1, "trakrf.id/r/reads", reads("E"))
	}
}

func TestBroadcaster_UnsubscribeStopsDelivery(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe(1)
	cancel()
	b.Publish(1, "trakrf.id/r/reads", reads("E"))
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("received an event after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBroadcaster_PublishNoSubscribersNoop(t *testing.T) {
	b := New()
	b.Publish(99, "trakrf.id/r/reads", reads("E")) // must not panic
}

func TestReaderKeyFromTopic(t *testing.T) {
	if got := readerKeyFromTopic("trakrf.id/dock-7/reads"); got != "dock-7" {
		t.Fatalf("got %q, want dock-7", got)
	}
	if got := readerKeyFromTopic("weird/topic"); got != "weird/topic" {
		t.Fatalf("fallback got %q", got)
	}
}
