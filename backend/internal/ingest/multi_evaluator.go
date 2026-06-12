package ingest

import (
	"context"
	"time"

	"github.com/trakrf/platform/backend/internal/storage"
)

// MultiEvaluator fans one message's membership-passing reads out to several
// ReadEvaluators (e.g. the geofence engine + the mustering engine). It satisfies
// ReadEvaluator itself, so the subscriber stays unaware of how many consumers
// there are. Nil-safe: nil elements (a disabled engine) are skipped, and a nil
// MultiEvaluator is a no-op.
type MultiEvaluator []ReadEvaluator

// Evaluate forwards to each non-nil evaluator in order. Each evaluator owns its
// own best-effort error handling (Evaluate has no error return by contract), so
// one slow/broken consumer cannot block the others or lose a scan.
func (m MultiEvaluator) Evaluate(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, reads []storage.ResolvedRead) {
	for _, e := range m {
		if e != nil {
			e.Evaluate(ctx, orgID, tagScanID, receivedAt, reads)
		}
	}
}
