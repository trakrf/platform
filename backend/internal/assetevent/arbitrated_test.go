package assetevent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/storage"
)

// arbitratedRead is a read whose asset's stored location zone arbitration
// decided. Its LocationID is what was observed, which may not be where the
// asset was stored.
func arbitratedRead(assetID, observed int, mv *storage.ArbitratedMove) storage.ResolvedRead {
	return storage.ResolvedRead{AssetID: assetID, ScanPointID: 4, LocationID: intp(observed), RSSI: -60,
		Stored: mv != nil, Arbitrated: true, Move: mv}
}

var switchAt = time.Date(2026, 11, 27, 12, 0, 41, 123456000, time.UTC)

func TestMoveStage_RecordsTheArbitratedMoveAtItsOwnTime(t *testing.T) {
	store := &fakeMoveStore{}
	rec := &fakeChangeRecorder{}
	mv := &storage.ArbitratedMove{From: intp(10), To: 20, ScanPointID: 5, At: switchAt}

	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		arbitratedRead(1, 30, mv), arbitratedRead(1, 30, nil)))
	require.NoError(t, err)
	require.Len(t, rec.got, 1)
	c := rec.got[0]
	assert.Equal(t, 10, *c.FromLocationID)
	assert.Equal(t, 20, c.ToLocationID, "the stored location, not the observed one")
	assert.Equal(t, 5, *c.ScanPointID)
	assert.Equal(t, switchAt, c.DetectedAt, "the switch instant, not the minute floor")
	assert.Equal(t, notificationevent.OriginReader, c.Origin)
	assert.Zero(t, store.calls, "the origin came with the move: no lookup")
}

func TestMoveStage_ArbitratedReadWithoutMoveRecordsNothing(t *testing.T) {
	store := &fakeMoveStore{}
	rec := &fakeChangeRecorder{}
	// Stored, at a different observed location than before: without
	// arbitration's say-so that is not a move.
	r := arbitratedRead(1, 30, nil)
	r.Stored = true
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(r))
	require.NoError(t, err)
	assert.Empty(t, rec.got)
	assert.Zero(t, store.calls)
}

func TestEvaluate_EmitsTheArbitratedMoveOnce(t *testing.T) {
	f := newFixture()
	e, q := newEvaluatorFixture(f)
	mv := &storage.ArbitratedMove{From: intp(10), To: 20, ScanPointID: 5, At: switchAt}

	e.Evaluate(context.Background(), 7, 1, switchAt.Add(time.Millisecond), []storage.ResolvedRead{
		arbitratedRead(1, 10, mv), arbitratedRead(1, 10, nil),
	})
	require.Len(t, q.events, 1)
	ev := q.events[0]
	require.NotNil(t, ev.From)
	assert.Equal(t, 10, ev.From.ID)
	assert.Equal(t, 20, ev.To.ID)
	assert.Equal(t, switchAt, ev.OccurredAt)
	assert.Empty(t, f.prevAsked, "explicit origin: no lookup")
}

func TestEvaluate_ArbitratedReadsWithoutMoveEmitNothing(t *testing.T) {
	f := newFixture()
	f.prev[1] = storage.PreviousLocation{LocationID: intp(10)}
	e, q := newEvaluatorFixture(f)
	r := arbitratedRead(1, 20, nil)
	r.Stored = true
	e.Evaluate(context.Background(), 7, 1, switchAt, []storage.ResolvedRead{r})
	assert.Empty(t, q.events)
	assert.Empty(t, f.prevAsked)
}
