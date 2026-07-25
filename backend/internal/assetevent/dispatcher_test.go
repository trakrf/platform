package assetevent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func testLogger() *zerolog.Logger {
	l := zerolog.Nop()
	return &l
}

// countingSink records every delivery and fails the first `failures` of them.
type countingSink struct {
	mu       sync.Mutex
	calls    int
	failures int
	got      []AssetMoved
	block    chan struct{} // when non-nil, Deliver waits on it
}

func (s *countingSink) Deliver(_ context.Context, ev AssetMoved) error {
	if s.block != nil {
		<-s.block
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.got = append(s.got, ev)
	if s.failures > 0 {
		s.failures--
		return errors.New("boom")
	}
	return nil
}

func (s *countingSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *countingSink) events() []AssetMoved {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]AssetMoved(nil), s.got...)
}

func newTestDispatcher(sinks []Sink, queueSize, workers int) *Dispatcher {
	d := NewDispatcher(sinks, testLogger())
	d.queue = make(chan AssetMoved, queueSize)
	d.workers = workers
	d.sleep = func(time.Duration) {} // no real waiting in tests by default
	return d
}

// Ingestion must never be held up by a customer's slow endpoint. A full queue
// drops, it does not apply backpressure.
func TestEnqueueNeverBlocksWhenQueueIsFull(t *testing.T) {
	d := newTestDispatcher(nil, 2, 0) // no workers: nothing drains the queue

	for i := 0; i < 50; i++ {
		done := make(chan struct{})
		go func() {
			d.Enqueue(AssetMoved{DeliveryID: "x"})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("Enqueue blocked on a full queue (iteration %d)", i)
		}
	}
	require.Equal(t, 2, len(d.queue), "the queue holds its cap and no more")
}

func TestDeliversEnqueuedEvents(t *testing.T) {
	s := &countingSink{}
	d := newTestDispatcher([]Sink{s}, 8, 2)
	d.Start()

	for i := 0; i < 5; i++ {
		d.Enqueue(AssetMoved{DeliveryID: "d", OrgID: 1, Asset: Asset{ID: i}})
	}
	d.Stop()

	require.Equal(t, 5, s.count(), "Stop drains the queue before returning")
}

func TestRetriesTwiceThenGivesUp(t *testing.T) {
	s := &countingSink{failures: 99} // always fails
	d := newTestDispatcher([]Sink{s}, 4, 1)

	var mu sync.Mutex
	var slept []time.Duration
	d.sleep = func(dur time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		slept = append(slept, dur)
	}
	d.Start()
	d.Enqueue(AssetMoved{DeliveryID: "retry-me"})
	d.Stop()

	require.Equal(t, 3, s.count(), "initial attempt plus two retries")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, slept, 2)
	// Jittered +/-20% around 1s and 5s.
	require.InDelta(t, float64(time.Second), float64(slept[0]), float64(250*time.Millisecond))
	require.InDelta(t, float64(5*time.Second), float64(slept[1]), float64(1250*time.Millisecond))
}

func TestSucceedsOnRetry(t *testing.T) {
	s := &countingSink{failures: 1}
	d := newTestDispatcher([]Sink{s}, 4, 1)
	d.Start()
	d.Enqueue(AssetMoved{DeliveryID: "flaky"})
	d.Stop()

	require.Equal(t, 2, s.count(), "one failure then success, no third attempt")
}

// TRA-1044 will add a second sink. One broken sink must not cost the other its
// delivery.
func TestOneFailingSinkDoesNotStarveAnother(t *testing.T) {
	bad := &countingSink{failures: 99}
	good := &countingSink{}
	d := newTestDispatcher([]Sink{bad, good}, 4, 1)
	d.Start()
	d.Enqueue(AssetMoved{DeliveryID: "fan-out"})
	d.Stop()

	require.Equal(t, 3, bad.count())
	require.Equal(t, 1, good.count())
}

func TestNilSinkIsSkipped(t *testing.T) {
	good := &countingSink{}
	d := newTestDispatcher([]Sink{nil, good}, 4, 1)
	d.Start()
	d.Enqueue(AssetMoved{DeliveryID: "nil-safe"})
	d.Stop()

	require.Equal(t, 1, good.count())
}

func TestEventReachesSinkIntact(t *testing.T) {
	s := &countingSink{}
	d := newTestDispatcher([]Sink{s}, 4, 1)
	d.Start()
	from := Location{ID: 7, Name: "Receiving"}
	d.Enqueue(AssetMoved{
		DeliveryID: "abc",
		OrgID:      42,
		Asset:      Asset{ID: 9, ExternalKey: "FORK-7", Name: "Forklift 7"},
		From:       &from,
		To:         Location{ID: 8, Name: "Bay 3"},
	})
	d.Stop()

	got := s.events()
	require.Len(t, got, 1)
	require.Equal(t, "abc", got[0].DeliveryID)
	require.Equal(t, "FORK-7", got[0].Asset.ExternalKey)
	require.NotNil(t, got[0].From)
	require.Equal(t, 7, got[0].From.ID)
	require.Equal(t, 8, got[0].To.ID)
}

func TestNilDispatcherIsSafe(t *testing.T) {
	var d *Dispatcher
	require.NotPanics(t, func() {
		d.Start()
		d.Enqueue(AssetMoved{})
		d.Stop()
	})
}

func TestStopIsIdempotent(t *testing.T) {
	d := newTestDispatcher([]Sink{&countingSink{}}, 4, 1)
	d.Start()
	d.Stop()
	require.NotPanics(t, d.Stop, "a second Stop must not close the queue twice")
}
