// Package geofence is the real-time geofence rules engine (TRA-901). It sits on
// the MQTT ingest path: after the subscriber derives asset_scans for the reads
// that pass the membership filter, it hands those resolved reads here, and the
// engine decides whether a registered asset crossing a boundary capture point
// should fire an alarm.
//
// The decision is: registered asset (membership, already enforced upstream by
// the tag-resolution in PersistReads) × boundary scan_point × RSSI above the
// trip line × not currently latched. Membership is therefore implicit — only
// membership-passing reads ever reach Evaluate. Firing writes an alarm_events
// row and invokes the Firer; both are best-effort and never block ingestion or
// the authoritative asset_scans write.
package geofence

import (
	"context"
	"strconv"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/storage"
)

// AlarmEvent describes one fired boundary alarm. It is persisted to alarm_events
// and handed to the Firer.
type AlarmEvent struct {
	OrgID       int
	AssetID     int
	ScanPointID int
	LocationID  *int
	EPC         string
	RSSI        int
	TagScanID   int64
	FiredAt     time.Time
}

// alarmWriter is the storage dependency the engine needs; *storage.Storage
// satisfies it. Narrowed to an interface so unit tests can inject a fake.
type alarmWriter interface {
	InsertAlarmEvent(ctx context.Context, orgID int, ev storage.AlarmEventRow) error
}

// Engine evaluates resolved reads and fires boundary alarms. Construct with
// NewEngine; call Start before use and Stop on shutdown.
type Engine struct {
	cfg   Config
	store alarmWriter
	firer Firer
	latch *latch
	log   zerolog.Logger
}

// NewEngine builds an engine with a real-clock latch sweeper.
func NewEngine(cfg Config, store *storage.Storage, firer Firer, log *zerolog.Logger) *Engine {
	return &Engine{
		cfg:   cfg,
		store: store,
		firer: firer,
		latch: newLatch(cfg.LatchTTL, cfg.SweepInterval, RealClock{}),
		log:   log.With().Str("component", "geofence").Logger(),
	}
}

// Start is a no-op today (the latch sweeper starts in newLatch) but keeps the
// Start/Stop lifecycle symmetric with the subscriber for the serve wiring.
func (e *Engine) Start() {}

// Stop stops the latch sweeper. Idempotent.
func (e *Engine) Stop() {
	if e.latch != nil {
		e.latch.Close()
	}
}

// Evaluate runs the geofence decision over every membership-passing read of one
// MQTT message. It never returns an error: side effects are best-effort and
// failures are logged + metriced so a slow/broken alarm path can never lose a
// scan or kill ingestion. receivedAt (server time) is both the latch timestamp
// and the alarm's FiredAt.
func (e *Engine) Evaluate(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, reads []storage.ResolvedRead) {
	for _, rd := range reads {
		metricEvaluated.Inc()

		if !rd.IsBoundary {
			metricSuppressed.WithLabelValues("not_boundary").Inc()
			continue
		}

		// RSSI gate. A 0 RSSI is the parser's "no usable RSSI" sentinel — 0 dBm is
		// physically implausible for RFID — so it never fires (conservative).
		if rd.RSSI == 0 {
			metricSuppressed.WithLabelValues("no_rssi").Inc()
			continue
		}
		threshold := e.thresholdFor(rd)
		if rd.RSSI < threshold {
			metricSuppressed.WithLabelValues("rssi_below_threshold").Inc()
			continue
		}

		// Dedup latch: suppress while the tag is present at this boundary.
		if !e.latch.admit(keyFor(orgID, rd.ScanPointID, rd.EPC), receivedAt) {
			metricSuppressed.WithLabelValues("latched").Inc()
			continue
		}

		ev := AlarmEvent{
			OrgID:       orgID,
			AssetID:     rd.AssetID,
			ScanPointID: rd.ScanPointID,
			LocationID:  rd.LocationID,
			EPC:         rd.EPC,
			RSSI:        rd.RSSI,
			TagScanID:   tagScanID,
			FiredAt:     receivedAt,
		}

		// Durable record first, then the physical action — both best-effort.
		if err := e.store.InsertAlarmEvent(ctx, orgID, storage.AlarmEventRow{
			AssetID:     ev.AssetID,
			ScanPointID: ev.ScanPointID,
			LocationID:  ev.LocationID,
			EPC:         ev.EPC,
			RSSI:        ev.RSSI,
			TagScanID:   ev.TagScanID,
			FiredAt:     ev.FiredAt,
		}); err != nil {
			e.log.Error().Err(err).Int("org_id", orgID).Str("epc", ev.EPC).Msg("alarm_events write failed")
			metricEventWriteErrors.Inc()
		}

		if err := e.firer.Fire(ctx, ev); err != nil {
			e.log.Error().Err(err).Int("org_id", orgID).Str("epc", ev.EPC).Msg("alarm firer failed")
			metricFireErrors.Inc()
		}

		metricFired.Inc()
		e.log.Info().
			Int("org_id", orgID).
			Int("asset_id", ev.AssetID).
			Int("scan_point_id", ev.ScanPointID).
			Str("epc", ev.EPC).
			Int("rssi", ev.RSSI).
			Msg("geofence alarm fired")
	}
}

// thresholdFor returns the RSSI trip line for a read: the per-scan-point override
// (metadata.rssi_threshold) when present and parseable, else the global default.
// A malformed override is ignored (falls back to global) so bad metadata never
// silences or misfires the gate unexpectedly.
func (e *Engine) thresholdFor(rd storage.ResolvedRead) int {
	if rd.RSSIThresholdRaw != nil {
		if n, err := strconv.Atoi(*rd.RSSIThresholdRaw); err == nil {
			return n
		}
	}
	return e.cfg.RSSIThreshold
}
