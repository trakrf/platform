package arbitration

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/trakrf/platform/backend/internal/storage"
)

var (
	metricSwitches = promauto.NewCounter(prometheus.CounterOpts{
		Name: "arbitration_switches_total",
		Help: "Committed changes of an asset's arbitrated location, first assignments included.",
	})
	metricAssets = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "arbitration_assets",
		Help: "Assets the zone arbiter holds state for.",
	})
)

// seedStore is the lookup for assets the arbiter holds no state for. It runs on
// the scan transaction, so the stage takes no pooled connection.
type seedStore interface {
	PreviousAssetLocationsTx(ctx context.Context, tx pgx.Tx, orgID int, assetIDs []int, before time.Time) (map[int]storage.PreviousLocation, error)
}

// Stage binds an Arbiter to the scan transaction: Begin a Run per scan, pass
// Run.Arbitrate to storage.PersistReadsArbitrated, and call Committed once the
// scan commits. A scan that rolls back simply never commits its Run.
type Stage struct {
	arb   *Arbiter
	store seedStore
}

func NewStage(a *Arbiter, store seedStore) *Stage { return &Stage{arb: a, store: store} }

// Run is one scan's arbitration.
type Run struct {
	s   *Stage
	res *Result
}

func (s *Stage) Begin() *Run { return &Run{s: s} }

// Arbitrate is a storage.ArbitrateFunc. Every read is judged at the server
// receive time: gateway clocks are not comparable across gateways.
func (r *Run) Arbitrate(ctx context.Context, tx pgx.Tx, in storage.ArbitrateInput) (map[int]storage.ArbitratedLocation, error) {
	reads := make([]Read, 0, len(in.Reads))
	ids := make([]int, 0, len(in.Reads))
	for _, rd := range in.Reads {
		reads = append(reads, Read{AssetID: rd.AssetID, ScanPointID: rd.ScanPointID, LocationID: rd.LocationID, RSSI: rd.RSSI, At: in.ReceivedAt})
		ids = append(ids, rd.AssetID)
	}

	var seeds map[int]Seed
	if missing := r.s.arb.NeedsSeed(in.OrgID, ids); len(missing) > 0 {
		// The current minute's row counts: it is the last stored location.
		before := in.ReceivedAt.Truncate(storage.ScanGranularity).Add(storage.ScanGranularity)
		prev, err := r.s.store.PreviousAssetLocationsTx(ctx, tx, in.OrgID, missing, before)
		if err != nil {
			return nil, fmt.Errorf("failed to look up stored locations for arbitration: %w", err)
		}
		seeds = make(map[int]Seed, len(prev))
		for id, p := range prev {
			if p.LocationID != nil {
				seeds[id] = Seed{LocationID: *p.LocationID}
			}
		}
	}

	res := r.s.arb.Evaluate(in.OrgID, reads, seeds)
	r.res = &res
	out := make(map[int]storage.ArbitratedLocation, len(res.Assignments))
	for id, a := range res.Assignments {
		out[id] = storage.ArbitratedLocation{LocationID: a.LocationID, ScanPointID: a.ScanPointID, Hold: a.ScanPointID == 0}
	}
	return out, nil
}

// Committed applies the scan's arbitration. Without it the arbiter is as it was.
func (r *Run) Committed() {
	if r.res == nil {
		return
	}
	r.s.arb.Apply(*r.res)
	for _, a := range r.res.Assignments {
		if a.Switched {
			metricSwitches.Inc()
		}
	}
	metricAssets.Set(float64(r.s.arb.Len()))
}
