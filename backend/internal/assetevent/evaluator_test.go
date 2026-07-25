package assetevent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/storage"
)

// fakeStore answers the two detection queries from in-memory fixtures and
// records what it was asked, so a test can assert that enrichment ran only for
// assets that actually moved.
type fakeStore struct {
	prev      map[int]storage.PreviousLocation
	assets    map[int]storage.AssetIdentity
	locations map[int]string

	prevErr  error
	namesErr error

	prevAsked  []int
	namesAsked []int
	locsAsked  []int
	prevBefore time.Time
}

func (f *fakeStore) PreviousAssetLocations(_ context.Context, _ int, assetIDs []int, before time.Time) (map[int]storage.PreviousLocation, error) {
	f.prevAsked = append(f.prevAsked, assetIDs...)
	f.prevBefore = before
	if f.prevErr != nil {
		return nil, f.prevErr
	}
	out := map[int]storage.PreviousLocation{}
	for _, id := range assetIDs {
		if p, ok := f.prev[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func (f *fakeStore) LookupMoveNames(_ context.Context, _ int, assetIDs, locationIDs []int) (map[int]storage.AssetIdentity, map[int]string, error) {
	f.namesAsked = append(f.namesAsked, assetIDs...)
	f.locsAsked = append(f.locsAsked, locationIDs...)
	if f.namesErr != nil {
		return nil, nil, f.namesErr
	}
	assets := map[int]storage.AssetIdentity{}
	for _, id := range assetIDs {
		if a, ok := f.assets[id]; ok {
			assets[id] = a
		}
	}
	locs := map[int]string{}
	for _, id := range locationIDs {
		if n, ok := f.locations[id]; ok {
			locs[id] = n
		}
	}
	return assets, locs, nil
}

type captureQueue struct{ events []AssetMoved }

func (c *captureQueue) Enqueue(ev AssetMoved) { c.events = append(c.events, ev) }

func intp(v int) *int { return &v }

// newFixture builds a store holding one asset (id 1) and two locations
// (10 "Receiving", 20 "Bay 3") with no prior sighting.
func newFixture() *fakeStore {
	return &fakeStore{
		prev:      map[int]storage.PreviousLocation{},
		assets:    map[int]storage.AssetIdentity{1: {ID: 1, ExternalKey: "FORK-7", Name: "Forklift 7"}},
		locations: map[int]string{10: "Receiving", 20: "Bay 3"},
	}
}

func newEvaluatorFixture(f *fakeStore) (*Evaluator, *captureQueue) {
	q := &captureQueue{}
	return NewEvaluator(f, q, testLogger()), q
}

func read(assetID int, locationID *int) storage.ResolvedRead {
	return storage.ResolvedRead{AssetID: assetID, LocationID: locationID, EPC: "E280", RSSI: -55}
}

func TestFirstEverSightingEmitsNullOrigin(t *testing.T) {
	f := newFixture()
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{read(1, intp(20))})

	require.Len(t, q.events, 1)
	require.Nil(t, q.events[0].From, "a genuine first-ever sighting has no origin")
	require.Equal(t, 20, q.events[0].To.ID)
	require.Equal(t, "Bay 3", q.events[0].To.Name)
	require.Equal(t, "FORK-7", q.events[0].Asset.ExternalKey)
	require.NotEmpty(t, q.events[0].DeliveryID)
	require.Equal(t, 7, q.events[0].OrgID)
}

// The whole point of the feature: a rescan at the same location is silence, not
// a heartbeat.
func TestSameLocationEmitsNothing(t *testing.T) {
	f := newFixture()
	f.prev[1] = storage.PreviousLocation{LocationID: intp(20), LastSeen: time.Now().Add(-time.Minute)}
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{read(1, intp(20))})

	require.Empty(t, q.events)
}

func TestChangedLocationEmitsExactlyOne(t *testing.T) {
	f := newFixture()
	f.prev[1] = storage.PreviousLocation{LocationID: intp(10), LastSeen: time.Now().Add(-time.Minute)}
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{read(1, intp(20))})

	require.Len(t, q.events, 1)
	require.NotNil(t, q.events[0].From)
	require.Equal(t, 10, q.events[0].From.ID)
	require.Equal(t, "Receiving", q.events[0].From.Name)
	require.Equal(t, 20, q.events[0].To.ID)
}

// Multiple antennas or multiple tags on one asset produce several reads in one
// message. asset_scans keeps one row (ON CONFLICT DO NOTHING); the event stream
// must agree.
func TestTwoReadsOfOneAssetInOneMessageEmitOne(t *testing.T) {
	f := newFixture()
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{
		read(1, intp(20)),
		read(1, intp(20)),
		read(1, intp(10)), // a second location in the same message loses, as in the insert
	})

	require.Len(t, q.events, 1)
	require.Equal(t, 20, q.events[0].To.ID)
}

func TestReadWithNoLocationIsSkipped(t *testing.T) {
	f := newFixture()
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{read(1, nil)})

	require.Empty(t, q.events)
	require.Empty(t, f.prevAsked, "a location-less read must not even trigger a lookup")
}

// A prior sighting that carried no location is still a sighting, and moving to
// a real location from it is a real move — reported with a null origin.
func TestPreviousNullLocationEmitsNullOrigin(t *testing.T) {
	f := newFixture()
	f.prev[1] = storage.PreviousLocation{LocationID: nil, LastSeen: time.Now().Add(-time.Minute)}
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{read(1, intp(20))})

	require.Len(t, q.events, 1)
	require.Nil(t, q.events[0].From)
}

// Enrichment is the expensive half. It must run only for assets that survived
// the delta comparison, so read volume never pays for it.
func TestEnrichmentRunsOnlyForSurvivingDeltas(t *testing.T) {
	f := newFixture()
	f.assets[2] = storage.AssetIdentity{ID: 2, ExternalKey: "PALLET-2", Name: "Pallet 2"}
	f.prev[1] = storage.PreviousLocation{LocationID: intp(20), LastSeen: time.Now()} // stationary
	f.prev[2] = storage.PreviousLocation{LocationID: intp(10), LastSeen: time.Now()} // moved
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{
		read(1, intp(20)),
		read(2, intp(20)),
	})

	require.Len(t, q.events, 1)
	require.Equal(t, 2, q.events[0].Asset.ID)
	require.ElementsMatch(t, []int{2}, f.namesAsked, "the stationary asset must never reach enrichment")
	require.ElementsMatch(t, []int{10, 20}, f.locsAsked)
}

// Without a trustworthy previous location we would be guessing, and guessing
// invents moves. Drop the message instead — best-effort, never fatal.
func TestLookupErrorEmitsNothing(t *testing.T) {
	f := newFixture()
	f.prevErr = errors.New("db down")
	e, q := newEvaluatorFixture(f)

	require.NotPanics(t, func() {
		e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{read(1, intp(20))})
	})
	require.Empty(t, q.events)
}

func TestNameLookupErrorEmitsNothing(t *testing.T) {
	f := newFixture()
	f.namesErr = errors.New("db down")
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{read(1, intp(20))})
	require.Empty(t, q.events)
}

// An asset deleted between the scan write and enrichment would otherwise be
// sent with an empty name.
func TestUnresolvableAssetIsDropped(t *testing.T) {
	f := newFixture()
	delete(f.assets, 1)
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{read(1, intp(20))})
	require.Empty(t, q.events)
}

// A since-deleted origin degrades to a null origin: the move still happened.
func TestUnresolvableOriginDegradesToNullOrigin(t *testing.T) {
	f := newFixture()
	f.prev[1] = storage.PreviousLocation{LocationID: intp(99), LastSeen: time.Now()}
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), []storage.ResolvedRead{read(1, intp(20))})

	require.Len(t, q.events, 1)
	require.Nil(t, q.events[0].From)
	require.Equal(t, 20, q.events[0].To.ID)
}

func TestEvaluateScansEmitsPerMovedAsset(t *testing.T) {
	f := newFixture()
	f.assets[2] = storage.AssetIdentity{ID: 2, ExternalKey: "PALLET-2", Name: "Pallet 2"}
	f.prev[1] = storage.PreviousLocation{LocationID: intp(20), LastSeen: time.Now()} // stationary
	e, q := newEvaluatorFixture(f)

	at := time.Now()
	e.EvaluateScans(context.Background(), 7, []int{1, 2, 2}, 20, at)

	require.Len(t, q.events, 1, "one event for the mover, none for the stationary asset, duplicates collapsed")
	require.Equal(t, 2, q.events[0].Asset.ID)
	require.Equal(t, at, q.events[0].OccurredAt)
	require.Equal(t, at, f.prevBefore, "the lookup must exclude the scan being evaluated")
}

func TestEmptyInputIsANoOp(t *testing.T) {
	f := newFixture()
	e, q := newEvaluatorFixture(f)

	e.Evaluate(context.Background(), 7, 0, time.Now(), nil)
	e.EvaluateScans(context.Background(), 7, nil, 20, time.Now())

	require.Empty(t, q.events)
	require.Empty(t, f.prevAsked)
}
