package assetevent

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/storage"
)

// evaluatorStore is the storage surface detection needs; *storage.Storage
// satisfies it. Narrowed so unit tests can inject a fake.
type evaluatorStore interface {
	PreviousAssetLocations(ctx context.Context, orgID int, assetIDs []int, before time.Time) (map[int]storage.PreviousLocation, error)
	LookupMoveNames(ctx context.Context, orgID int, assetIDs, locationIDs []int) (map[int]storage.AssetIdentity, map[int]string, error)
}

// enqueuer hands a detected event to delivery; *Dispatcher satisfies it.
type enqueuer interface {
	Enqueue(ev AssetMoved)
}

// Evaluator turns scan observations into asset.moved events.
//
// It satisfies ingest.ReadEvaluator, so it joins the MQTT fan-out as another
// element of ingest.MultiEvaluator with no subscriber changes, and it exposes
// EvaluateScans for the handheld/manual Save path. Both callers invoke it
// AFTER their write has committed: enqueueing inside the write transaction
// would emit a phantom event whenever that transaction rolls back — the TRA-900
// failure mode, where the ingest fan-out silently rolled back because the org
// GUC was never set. Post-commit is the only ordering that cannot lie.
//
// Because the previous-location oracle is the database rather than process
// memory, there is no cold-start problem and no startup grace window (the thing
// TRA-991 needed for geofence), and the design is replica-safe: a move produces
// one message handled by one replica, which reads shared state and emits once.
type Evaluator struct {
	store evaluatorStore
	queue enqueuer
	log   zerolog.Logger
}

// NewEvaluator builds an evaluator. store is normally *storage.Storage and
// queue is normally *Dispatcher.
func NewEvaluator(store evaluatorStore, queue enqueuer, log *zerolog.Logger) *Evaluator {
	return &Evaluator{
		store: store,
		queue: queue,
		log:   log.With().Str("component", "assetevent").Logger(),
	}
}

// Evaluate satisfies ingest.ReadEvaluator: the MQTT/fixed-reader path. It has
// no error return by contract — failures are logged and metriced so a broken
// webhook can never lose a scan or kill ingestion.
//
// tagScanID is intentionally unused: it is physical-layer provenance, and the
// event payload carries logical data only.
func (e *Evaluator) Evaluate(ctx context.Context, orgID int, _ int64, receivedAt time.Time, reads []storage.ResolvedRead) {
	if len(reads) == 0 {
		return
	}
	candidates := make(map[int]int, len(reads))
	for _, rd := range reads {
		metricEvaluated.Inc()
		// A conflict-dropped read (TRA-1118): asset_scans already held this
		// asset's minute bucket, so stored history did not change and emitting
		// would re-publish the identical move once per message for the rest of
		// the minute. Counted so flapping stays visible in Prometheus.
		if !rd.Stored {
			metricSuppressed.WithLabelValues("not_stored").Inc()
			continue
		}
		// An asset scanned at a scan point with no location cannot have moved
		// anywhere nameable, so there is nothing to report.
		if rd.LocationID == nil {
			metricSuppressed.WithLabelValues("no_location").Inc()
			continue
		}
		// Stored identifies the row asset_scans actually kept — at most one per
		// asset per message — so this seen-check is defense in depth for
		// within-message duplicates of that stored read.
		if _, seen := candidates[rd.AssetID]; seen {
			continue
		}
		candidates[rd.AssetID] = *rd.LocationID
	}
	e.evaluate(ctx, orgID, receivedAt, candidates)
}

// EvaluateScans is the handheld/manual Save path (POST /api/v1/inventory/save).
// Every asset in a save lands at the same location. Called post-commit by the
// inventory handler.
func (e *Evaluator) EvaluateScans(ctx context.Context, orgID int, assetIDs []int, locationID int, at time.Time) {
	if len(assetIDs) == 0 {
		return
	}
	candidates := make(map[int]int, len(assetIDs))
	for _, id := range assetIDs {
		metricEvaluated.Inc()
		if _, seen := candidates[id]; seen {
			continue
		}
		candidates[id] = locationID
	}
	e.evaluate(ctx, orgID, at, candidates)
}

// evaluate is the shared body: look up where each asset was, discard the ones
// that did not move, enrich only the survivors, enqueue.
//
// The ordering is load-bearing. The event reports from_location and
// to_location, so the previous location has to be fetched either way — dedup is
// then a free comparison on a value already in hand. Discarding before
// enrichment means a no-op rescan (the overwhelmingly common case) pays for one
// hot-buffer read and nothing else: no name join, no allocation, no send.
func (e *Evaluator) evaluate(ctx context.Context, orgID int, at time.Time, candidates map[int]int) {
	if len(candidates) == 0 {
		return
	}

	assetIDs := make([]int, 0, len(candidates))
	for id := range candidates {
		assetIDs = append(assetIDs, id)
	}

	// Stored timestamps are truncated to storage.ScanGranularity (TRA-1118) and
	// PreviousAssetLocations bounds with a strict `timestamp < before`, so the
	// lookup must use the truncated minute: raw receivedAt sits above the
	// just-stored bucket, which would then act as its own predecessor and
	// suppress every genuine move as no_change. (EvaluateScans already passes a
	// truncated time; Truncate is idempotent.)
	before := at.Truncate(storage.ScanGranularity)

	start := time.Now()
	prev, err := e.store.PreviousAssetLocations(ctx, orgID, assetIDs, before)
	metricLookupSeconds.Observe(time.Since(start).Seconds())
	if err != nil {
		// Best-effort: without a trustworthy previous location we would have to
		// guess, and guessing produces phantom moves. Drop the message instead.
		metricLookupErrors.Inc()
		e.log.Error().Err(err).Int("org_id", orgID).Msg("previous-location lookup failed; no asset events emitted for this message")
		return
	}

	type move struct {
		assetID int
		from    *int
		to      int
	}
	moves := make([]move, 0, len(candidates))
	locationIDs := make(map[int]struct{}, len(candidates))

	for assetID, to := range candidates {
		p, seen := prev[assetID]
		// Absent from the map = genuine first-ever sighting. A prior sighting
		// whose location was NULL also yields a null origin, but it is a real
		// observation, so it can still produce a move.
		if seen && p.LocationID != nil && *p.LocationID == to {
			metricSuppressed.WithLabelValues("no_change").Inc()
			continue
		}
		var from *int
		if seen && p.LocationID != nil {
			from = p.LocationID
			locationIDs[*from] = struct{}{}
		}
		locationIDs[to] = struct{}{}
		moves = append(moves, move{assetID: assetID, from: from, to: to})
	}
	if len(moves) == 0 {
		return
	}

	movedAssetIDs := make([]int, 0, len(moves))
	for _, m := range moves {
		movedAssetIDs = append(movedAssetIDs, m.assetID)
	}
	locIDs := make([]int, 0, len(locationIDs))
	for id := range locationIDs {
		locIDs = append(locIDs, id)
	}

	assets, locations, err := e.store.LookupMoveNames(ctx, orgID, movedAssetIDs, locIDs)
	if err != nil {
		metricLookupErrors.Inc()
		e.log.Error().Err(err).Int("org_id", orgID).Msg("asset/location name lookup failed; no asset events emitted for this message")
		return
	}

	for _, m := range moves {
		a, ok := assets[m.assetID]
		if !ok {
			// Deleted between the scan write and this lookup. Better to send
			// nothing than an event with an empty asset name.
			metricSuppressed.WithLabelValues("unresolved_names").Inc()
			continue
		}
		toName, ok := locations[m.to]
		if !ok {
			metricSuppressed.WithLabelValues("unresolved_names").Inc()
			continue
		}
		var from *Location
		if m.from != nil {
			// A since-deleted origin degrades to a null origin rather than
			// suppressing the event: the move itself still happened.
			if name, ok := locations[*m.from]; ok {
				from = &Location{ID: *m.from, Name: name}
			}
		}

		ev := AssetMoved{
			DeliveryID: uuid.NewString(),
			OccurredAt: at,
			OrgID:      orgID,
			Asset:      Asset{ID: a.ID, ExternalKey: a.ExternalKey, Name: a.Name},
			From:       from,
			To:         Location{ID: m.to, Name: toName},
		}
		metricEmitted.Inc()
		e.log.Info().
			Int("org_id", orgID).
			Int("asset_id", a.ID).
			Int("to_location_id", m.to).
			Str("delivery_id", ev.DeliveryID).
			Msg("asset.moved detected")
		e.queue.Enqueue(ev)
	}
}
