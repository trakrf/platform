package ingest

import (
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// handleMessage isn't unit-tested directly: it depends on the concrete
// *storage.Storage (InsertRawTagScan / ResolveScanTopic / PersistReads), which
// needs a live DB. The parsed-read fan-out is covered behaviorally by the
// readstream broadcaster tests and the SSE handler integration test. Here we
// only lock the seam: a ReadPublisher implementation type-checks and wires into
// NewSubscriber.

type fakePublisher struct {
	mu    sync.Mutex
	calls []pubCall
}

type pubCall struct {
	orgID int
	topic string
	reads []scanread.Read
}

func (f *fakePublisher) Publish(orgID int, topic string, rs []scanread.Read) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, pubCall{orgID, topic, rs})
}

func TestNewSubscriber_AcceptsReadPublisher(t *testing.T) {
	var _ ReadPublisher = (*fakePublisher)(nil)

	log := zerolog.Nop()
	sub := NewSubscriber(Config{}, nil, nil, &fakePublisher{}, &log)
	if sub.feed == nil {
		t.Fatal("expected feed publisher to be stored on the subscriber")
	}

	// nil feed must also be accepted (fan-out disabled).
	subNil := NewSubscriber(Config{}, nil, nil, nil, &log)
	if subNil.feed != nil {
		t.Fatal("expected nil feed when none provided")
	}
}
