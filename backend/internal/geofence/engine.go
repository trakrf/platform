// Package geofence is the real-time geofence rules engine (TRA-901, TRA-943). It
// sits on the MQTT ingest path: after the subscriber derives asset_scans for the
// reads that pass the membership filter, it hands those resolved reads here, and
// the engine decides whether to drive the output devices bound to the read's
// location.
//
// All rule config lives on output_device.metadata (TRA-943): mode (egress|
// presence), age_out_seconds, rssi_threshold, auto_off_seconds. The engine
// resolves location -> outputs and keys its dedup/presence state per (output,
// epc). Membership is implicit — only membership-passing reads ever reach
// Evaluate. Firing writes an alarm_events row and drives the device; both are
// best-effort and never block ingestion or the authoritative asset_scans write.
package geofence

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

// engineStore is the storage surface the engine needs; *storage.Storage
// satisfies it. Narrowed so unit tests can inject a fake.
type engineStore interface {
	InsertAlarmEvent(ctx context.Context, orgID int, ev storage.AlarmEventRow) error
	ListOutputDevicesForLocation(ctx context.Context, orgID, locationID int) ([]outputdevice.OutputDevice, error)
	GetOrgGeofenceDefaults(ctx context.Context, orgID int) (organization.GeofenceDefaults, error)
}

// outputDriver drives one output device on/off via its transport;
// alarm.Dispatcher satisfies it. Defined here (not imported from alarm) to avoid
// an import cycle — alarm depends on geofence, not vice versa.
type outputDriver interface {
	Set(ctx context.Context, dev outputdevice.OutputDevice, on bool, offAfterSec int) error
}

// Engine evaluates resolved reads and drives output devices. Construct with
// NewEngine; call Start before use and Stop on shutdown.
type Engine struct {
	cfg      Config
	store    engineStore
	driver   outputDriver
	latch    *latch    // egress dedup, keyed per (org, output, epc)
	presence *presence // presence tracker, keyed per (org, output)
	log      zerolog.Logger

	// startupGrace is the cold-start grace window (TRA-991); graceUntil is its
	// deadline in unix nanos, set once from the first evaluated read's receive
	// time (0 = window not yet opened). See withinStartupGrace.
	startupGrace time.Duration
	graceUntil   atomic.Int64
}

// NewEngine builds an engine with real-clock latch + presence sweepers.
func NewEngine(cfg Config, store *storage.Storage, driver outputDriver, log *zerolog.Logger) *Engine {
	l := log.With().Str("component", "geofence").Logger()
	clk := RealClock{}
	return &Engine{
		cfg:          cfg,
		store:        store,
		driver:       driver,
		latch:        newLatch(cfg.SweepInterval, clk),
		presence:     newPresence(driver, l),
		startupGrace: cfg.StartupGrace,
		log:          l,
	}
}

// Start is a no-op today (the sweepers start in their constructors) but keeps the
// Start/Stop lifecycle symmetric with the subscriber for the serve wiring.
func (e *Engine) Start() {}

// Stop stops the latch + presence sweepers. Idempotent.
func (e *Engine) Stop() {
	if e.latch != nil {
		e.latch.Close()
	}
	if e.presence != nil {
		e.presence.Close()
	}
}

// withinStartupGrace reports whether a read received at receivedAt falls inside
// the post-startup grace window (TRA-991). The window opens on the first
// evaluated read — i.e. when ingestion actually begins, independent of how long
// the broker took to connect — and lasts startupGrace. A non-positive
// startupGrace disables it entirely (fire on first sight, the pre-TRA-991
// behavior). Safe for concurrent Evaluate calls: CompareAndSwap lets exactly one
// goroutine open the window; the rest compare against the stored deadline.
func (e *Engine) withinStartupGrace(receivedAt time.Time) bool {
	if e.startupGrace <= 0 {
		return false
	}
	now := receivedAt.UnixNano()
	// Open the window on the first read; that read is always within grace.
	if e.graceUntil.CompareAndSwap(0, now+e.startupGrace.Nanoseconds()) {
		return true
	}
	return now < e.graceUntil.Load()
}

// Evaluate runs the rule decision over every membership-passing read of one MQTT
// message. It never returns an error: side effects are best-effort and failures
// are logged + metriced so a slow/broken output path can never lose a scan or
// kill ingestion. For each read it resolves the location's active output devices
// and applies each device's mode. receivedAt (server time) is both the
// dedup/presence timestamp and the alarm's FiredAt.
func (e *Engine) Evaluate(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, reads []storage.ResolvedRead) {
	// Org-default tuning tier (TRA-955). Fetched once per message — cheaper than
	// the per-read device lookup below, and runtime-fresh (UI edits take effect on
	// the next message, no restart). Best-effort: a lookup error falls back to the
	// system/code defaults rather than dropping the message.
	orgDefaults, err := e.store.GetOrgGeofenceDefaults(ctx, orgID)
	if err != nil {
		e.log.Warn().Err(err).Int("org_id", orgID).Msg("org geofence defaults lookup failed; using system defaults")
		orgDefaults = organization.GeofenceDefaults{}
	}

	// TRA-991: while inside the cold-start grace window, reads below still seed the
	// latch/presence state (so the tags already in the zone become an "already
	// inside" baseline) but the ON edge is suppressed — a restart must not re-fire
	// every present tag.
	inGrace := e.withinStartupGrace(receivedAt)

	for _, rd := range reads {
		metricEvaluated.Inc()

		// RSSI gate. A 0 RSSI is the parser's "no usable RSSI" sentinel — 0 dBm is
		// physically implausible for RFID — so it never fires (conservative).
		if rd.RSSI == 0 {
			metricSuppressed.WithLabelValues("no_rssi").Inc()
			continue
		}
		// Outputs are location-bound. A read whose scan point has no location can
		// match no output, so nothing to drive.
		if rd.LocationID == nil {
			metricSuppressed.WithLabelValues("no_location").Inc()
			continue
		}

		devices, err := e.store.ListOutputDevicesForLocation(ctx, orgID, *rd.LocationID)
		if err != nil {
			e.log.Error().Err(err).Int("org_id", orgID).Int("location_id", *rd.LocationID).Msg("output device lookup failed")
			metricFireErrors.Inc()
			continue
		}

		firedAny := false
		for _, dev := range devices {
			// Collapse the three tuning tiers (system/code -> org default ->
			// per-output override) for this device (TRA-955).
			tuning := Resolve(e.cfg, orgDefaults, dev)

			if rd.RSSI < tuning.RSSIThreshold {
				metricSuppressed.WithLabelValues("rssi_below_threshold").Inc()
				continue
			}

			ttl := time.Duration(tuning.AgeOutSeconds) * time.Second

			if tuning.Mode == outputdevice.ModePresence {
				// Presence: ON on the 0->1 edge; OFF fires from the output's age-out
				// timer when no member is read for age-out. auto_off is ignored
				// (the engine owns the OFF edge).
				if e.presence.observe(orgID, dev, ttl, rd.EPC) {
					// observe already armed the age-out timer (state seeded); only the
					// ON drive is held back during the startup grace window.
					if inGrace {
						metricSuppressed.WithLabelValues("startup_grace").Inc()
						continue
					}
					firedAny = true
					e.drive(ctx, orgID, dev, true, 0)
				}
				continue
			}

			// Egress: fire ON, then latch for the re-arm window.
			if !e.latch.admit(latchKey(orgID, dev.ID, rd.EPC), receivedAt, ttl) {
				metricSuppressed.WithLabelValues("latched").Inc()
				continue
			}
			// admit already recorded the observation (state seeded); only the ON drive
			// is held back during the startup grace window.
			if inGrace {
				metricSuppressed.WithLabelValues("startup_grace").Inc()
				continue
			}
			firedAny = true
			e.drive(ctx, orgID, dev, true, tuning.AutoOffSeconds)
		}

		if firedAny {
			e.recordFire(ctx, orgID, tagScanID, receivedAt, rd)
		}
	}
}

// drive turns a device on/off, folding any error into the best-effort metric.
func (e *Engine) drive(ctx context.Context, orgID int, dev outputdevice.OutputDevice, on bool, offAfter int) {
	if err := e.driver.Set(ctx, dev, on, offAfter); err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Int("output_device_id", dev.ID).Bool("on", on).Msg("output device drive failed (best-effort)")
		metricFireErrors.Inc()
	}
}

// recordFire writes the durable alarm_events row for a read that drove at least
// one device on, and logs/metrics the fire. Best-effort: a write error never
// blocks ingestion.
func (e *Engine) recordFire(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, rd storage.ResolvedRead) {
	if err := e.store.InsertAlarmEvent(ctx, orgID, storage.AlarmEventRow{
		AssetID:     rd.AssetID,
		ScanPointID: rd.ScanPointID,
		LocationID:  rd.LocationID,
		EPC:         rd.EPC,
		RSSI:        rd.RSSI,
		TagScanID:   tagScanID,
		FiredAt:     receivedAt,
	}); err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Str("epc", rd.EPC).Msg("alarm_events write failed")
		metricEventWriteErrors.Inc()
	}

	metricFired.Inc()
	e.log.Info().
		Int("org_id", orgID).
		Int("asset_id", rd.AssetID).
		Int("scan_point_id", rd.ScanPointID).
		Str("epc", rd.EPC).
		Int("rssi", rd.RSSI).
		Msg("geofence rule fired")
}
