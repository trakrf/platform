package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/storage"
)

type countingEvaluator struct{ calls int }

func (c *countingEvaluator) Evaluate(_ context.Context, _ int, _ int64, _ time.Time, _ []storage.ResolvedRead) {
	c.calls++
}

func TestMultiEvaluator_FansOutToAllNonNil(t *testing.T) {
	a := &countingEvaluator{}
	b := &countingEvaluator{}
	m := MultiEvaluator{a, nil, b} // nil element must be skipped, not panic

	m.Evaluate(context.Background(), 1, 1, time.Now(), nil)

	if a.calls != 1 || b.calls != 1 {
		t.Fatalf("expected each evaluator called once, got a=%d b=%d", a.calls, b.calls)
	}
}

func TestMultiEvaluator_NilSliceIsNoOp(t *testing.T) {
	var m MultiEvaluator // nil
	// Must not panic.
	m.Evaluate(context.Background(), 1, 1, time.Now(), nil)
}
