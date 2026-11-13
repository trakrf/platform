package crossing

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/detection"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/storage"
)

// stageStore is the storage surface the stage needs for assets it has no state
// for. Both lookups run on the scan transaction, so the stage takes no pooled
// connection.
type stageStore interface {
	PreviousAssetLocationsTx(ctx context.Context, tx pgx.Tx, orgID int, assetIDs []int, before time.Time) (map[int]storage.PreviousLocation, error)
	ScanDeviceLocationIDsTx(ctx context.Context, tx pgx.Tx, orgID, scanDeviceID int) ([]int, error)
}

// changeRecorder writes a location change and queues its notification.
type changeRecorder interface {
	RecordChange(ctx context.Context, tx pgx.Tx, c notificationevent.LocationChange) (recorded bool, err error)
}

// Stage is the detection stage that records gate crossings.
type Stage struct {
	tracker *Tracker
	store   stageStore
	rec     changeRecorder
}

var _ detection.Stage = (*Stage)(nil)

func NewStage(t *Tracker, store stageStore, rec changeRecorder) *Stage {
	return &Stage{tracker: t, store: store, rec: rec}
}

func (s *Stage) Name() string { return "crossing" }

// Detect evaluates the scan's reads against the tracker and records each
// crossing in tx. The tracker only changes in OnCommit, and only if the rows
// were kept. A scan that fails is not redelivered: detection resumes from the
// next read, without the evidence the failed scan carried.
func (s *Stage) Detect(ctx context.Context, tx pgx.Tx, in storage.DetectInput) (detection.Pending, error) {
	reads, ignored := orderedReads(in.Reads, in.ReceivedAt)
	seeds, err := s.seeds(ctx, tx, in, reads)
	if err != nil {
		return detection.Pending{}, err
	}
	res := s.tracker.Evaluate(in.OrgID, in.ScanDeviceID, reads, seeds)

	base := crossingBase(in.ReceivedAt)
	nth := map[int]int{}
	var recorded, replayed int
	for _, c := range res.Crossings {
		from, scanPoint := c.From, c.ScanPointID
		ok, err := s.rec.RecordChange(ctx, tx, notificationevent.LocationChange{
			OrgID:          in.OrgID,
			AssetID:        c.AssetID,
			FromLocationID: &from,
			ToLocationID:   c.To,
			Origin:         notificationevent.OriginReaderCrossing,
			ScanPointID:    &scanPoint,
			DetectedAt:     base.Add(time.Duration(nth[c.AssetID]) * time.Microsecond),
		})
		if err != nil {
			return detection.Pending{}, fmt.Errorf("failed to record crossing for asset %d: %w", c.AssetID, err)
		}
		nth[c.AssetID]++
		if ok {
			recorded++
		} else {
			replayed++
		}
	}

	return detection.Pending{OnCommit: func(rowsKept bool) {
		metricReadsIgnored.Add(float64(ignored))
		if !rowsKept {
			return
		}
		s.tracker.Apply(res)
		metricCrossings.WithLabelValues("recorded").Add(float64(recorded))
		metricCrossings.WithLabelValues("deduped").Add(float64(res.Deduped + replayed))
		metricCrossings.WithLabelValues("tie").Add(float64(res.Ties))
	}}, nil
}

// crossingBase is the detected_at for the first crossing of each asset in a
// message; later ones are offset a microsecond each, so they stay distinct and
// ordered. The database keeps microseconds, so a receive time that lands on the
// scan bucket's floor at that precision is the reader move's detected_at: the
// crossing starts a microsecond after it rather than collide on the unique key.
func crossingBase(receivedAt time.Time) time.Time {
	floor := receivedAt.Truncate(storage.ScanGranularity)
	if receivedAt.Truncate(time.Microsecond).Equal(floor) {
		return floor.Add(time.Microsecond)
	}
	return receivedAt
}

// readerClockTolerance is how far behind the server receive time a reader
// timestamp may be and still be trusted. A reader with a wrong clock would
// otherwise scramble its windows.
const readerClockTolerance = 5 * time.Minute

// orderedReads returns the reads in reader time, arrival order breaking ties,
// and how many had no usable RSSI. A read takes the server receive time when the
// reader sent no timestamp, one more than readerClockTolerance behind it, or one
// ahead of it: nothing is heard after it is received, and a stamp ahead would
// otherwise hold the asset's clock in the future until real time caught up.
func orderedReads(in []storage.ResolvedRead, receivedAt time.Time) (reads []Read, ignored int) {
	for _, r := range in {
		if !usable(r.RSSI) {
			ignored++
		}
		at := r.ReaderTimestamp
		if at.IsZero() || at.After(receivedAt) || receivedAt.Sub(at) > readerClockTolerance {
			at = receivedAt
		}
		reads = append(reads, Read{AssetID: r.AssetID, ScanPointID: r.ScanPointID, LocationID: r.LocationID, RSSI: r.RSSI, At: at})
	}
	sort.SliceStable(reads, func(i, j int) bool { return reads[i].At.Before(reads[j].At) })
	return reads, ignored
}

// seeds looks up the last known location for assets the tracker has not seen at
// this reader, so a restart keeps each asset's last side. It does nothing when
// there are none.
func (s *Stage) seeds(ctx context.Context, tx pgx.Tx, in storage.DetectInput, reads []Read) (map[int]Seed, error) {
	var candidates []int
	for _, r := range reads {
		if r.LocationID != nil && usable(r.RSSI) {
			candidates = append(candidates, r.AssetID)
		}
	}
	missing := s.tracker.NeedsSeed(in.OrgID, in.ScanDeviceID, candidates)
	if len(missing) == 0 {
		return nil, nil
	}
	prev, err := s.store.PreviousAssetLocationsTx(ctx, tx, in.OrgID, missing, in.ReceivedAt.Truncate(storage.ScanGranularity))
	if err != nil {
		return nil, fmt.Errorf("failed to look up previous locations: %w", err)
	}
	locs, err := s.store.ScanDeviceLocationIDsTx(ctx, tx, in.OrgID, in.ScanDeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to look up reader locations: %w", err)
	}
	seeds := make(map[int]Seed, len(missing))
	for _, id := range missing {
		p, ok := prev[id]
		if !ok || p.LocationID == nil {
			continue
		}
		seeds[id] = Seed{Known: true, LocationID: *p.LocationID, AtReader: slices.Contains(locs, *p.LocationID)}
	}
	return seeds, nil
}
