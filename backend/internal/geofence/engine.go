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
// Evaluate does both after the scan commits, and writes no notification trigger.
// Stage is the path for the MQTT subscriber: it writes the alarm and its trigger
// inside the scan transaction and drives after commit.
//
// Deciding and driving are separate steps: decide changes only in-memory state
// and returns fires, and driveFire makes the network calls. A caller that decides
// inside a transaction can therefore drive after commit, or undo the fires if the
// transaction rolls back; Stage does this. decide's lookups still run while that
// transaction is open, so it bounds them with lookupTimeout.
package geofence

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/readerrpc"
	"github.com/trakrf/platform/backend/internal/storage"
)

// lookupTimeout bounds each pooled lookup in decide. decide can run inside the
// scan transaction, so a stalled lookup would hold it open and spend the
// message's shared time limit, which would lose the scan.
const lookupTimeout = 2 * time.Second

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
	latch    *latch      // egress dedup, keyed per (org, output, epc)
	pulses   *pulseGuard // egress re-entrancy guard, keyed per (org, output)
	presence *presence   // presence tracker, keyed per (org, output)
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
		pulses:       newPulseGuard(),
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

// fireDrive is one output a fire turns on.
type fireDrive struct {
	dev      outputdevice.OutputDevice
	offAfter int           // device-side auto-off handed to the driver; the on-time for presence
	pulse    time.Duration // pulse-guard window this drive marked; 0 if it marked none
	presence bool          // a presence ON edge, whose result the presence state records
}

// fire is a read that decide resolved to an alarm. It holds the read to record,
// the drives to make, and the steps that take back the in-memory state the
// decision changed. A caller either drives a fire or undoes it. A fire may have
// no drives: a read dropped because its output is still pulsing is still an
// alarm, and sets dropped.
type fire struct {
	read       storage.ResolvedRead
	receivedAt time.Time
	drives     []fireDrive
	dropped    bool // an output was busy pulsing, so this read was not driven on it
	undos      []func()
}

// undo restores the latch, pulse and presence state this fire changed, newest
// first, so the next read fires again. A fire undoes only its own state: a read
// dropped by a busy output restores its own latch and does not touch the pulse
// that dropped it. Reads that only refreshed the latch are not undone: they are
// still true sightings.
func (f fire) undo() {
	for i := len(f.undos) - 1; i >= 0; i-- {
		f.undos[i]()
	}
}

// Evaluate runs the rule decision over every membership-passing read of one MQTT
// message, then drives and records each fire. It never returns an error: side
// effects are best-effort and failures are logged + metriced so a slow/broken
// output path can never lose a scan or kill ingestion. receivedAt (server time) is
// both the dedup/presence timestamp and the alarm's FiredAt.
func (e *Engine) Evaluate(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, reads []storage.ResolvedRead) {
	fires, refreshes := e.decide(ctx, orgID, receivedAt, reads)
	for _, f := range fires {
		e.driveFire(ctx, orgID, f)
		e.recordFire(ctx, orgID, f, tagScanID, receivedAt)
	}
	e.refreshPresence(ctx, orgID, receivedAt, refreshes)
}

// decide applies each read's output devices and modes and returns the reads that
// fired, one fire per read. A read that drives an output, or that a busy output
// dropped, fires; presence reads fire only on the 0->1 edge. It updates latch,
// pulse and presence state but drives nothing and writes no alarm.
//
// Its lookups do not use a caller's transaction: each runs on its own pooled
// connection, so called inside a transaction they see none of its uncommitted
// writes, and that transaction stays open while they wait. Each lookup is bounded
// by lookupTimeout, which caps that wait at one timeout for the org defaults plus
// one per distinct location in the message. ctx itself gets no timeout here, so
// cancelling a lookup never reaches a query on the caller's transaction, which
// pgx would answer by closing the connection and losing the transaction.
func (e *Engine) decide(ctx context.Context, orgID int, receivedAt time.Time, reads []storage.ResolvedRead) (fires []fire, refreshes []outputdevice.OutputDevice) {
	// Org-default tuning tier (TRA-955). Fetched once per message — cheaper than
	// the per-read device lookup below, and runtime-fresh (UI edits take effect on
	// the next message, no restart). Best-effort: a lookup error falls back to the
	// system/code defaults rather than dropping the message.
	orgDefaults, err := e.orgDefaults(ctx, orgID)
	if err != nil {
		e.log.Warn().Err(err).Int("org_id", orgID).Msg("org geofence defaults lookup failed; using system defaults")
		orgDefaults = organization.GeofenceDefaults{}
	}

	// TRA-991: while inside the cold-start grace window, reads below still seed the
	// latch/presence state (so the tags already in the zone become an "already
	// inside" baseline) but the ON edge is suppressed — a restart must not re-fire
	// every present tag.
	inGrace := e.withinStartupGrace(receivedAt)

	located := map[int]deviceLookup{}
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

		lookup, done := located[*rd.LocationID]
		if !done {
			lookup = e.devicesAt(ctx, orgID, *rd.LocationID)
			located[*rd.LocationID] = lookup
		}
		if !lookup.ok {
			continue
		}

		f := fire{read: rd, receivedAt: receivedAt}
		for _, dev := range lookup.devices {
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
				// (the engine owns the OFF edge). No ON latches (TRA-1138): each
				// carries the reader's on-time, and is re-sent while the tag stays
				// in view, so a lost connection stops the output within it.
				if e.presence.observe(orgID, dev, ttl, rd.EPC) {
					// observe already armed the age-out timer (state seeded); only the
					// ON drive is held back during the startup grace window.
					if inGrace {
						metricSuppressed.WithLabelValues("startup_grace").Inc()
						continue
					}
					f.drives = append(f.drives, fireDrive{dev: dev, offAfter: presenceOnSeconds, presence: true})
					f.undos = append(f.undos, func() { e.presence.forget(orgID, dev.ID) })
					continue
				}
				if e.presence.refreshDue(orgID, dev.ID, receivedAt) {
					// A refresh, or the retry of a failed ON. Not a fire: no
					// alarm_events row and nothing to undo.
					refreshes = append(refreshes, dev)
				}
				continue
			}

			// Egress: fire ON, then latch for the re-arm window. The snapshot lets
			// undo put back what admit is about to overwrite.
			key := latchKey(orgID, dev.ID, rd.EPC)
			lastSeen, seen := e.latch.snapshot(key)
			if !e.latch.admit(key, receivedAt, ttl) {
				metricSuppressed.WithLabelValues("latched").Inc()
				continue
			}
			// admit already recorded the observation (state seeded); only the ON drive
			// is held back during the startup grace window.
			if inGrace {
				metricSuppressed.WithLabelValues("startup_grace").Inc()
				continue
			}
			// While this output is still pulsing from an earlier tag, drive nothing: a
			// second ON would restart or be swallowed by the device-side auto-off,
			// depending on transport. The read is still an alarm, so the fire is
			// kept without a drive. The latch has admitted this tag, so it will not
			// re-fire while it stays present; the fire's undo puts that back.
			pulse := time.Duration(tuning.AutoOffSeconds) * time.Second
			if !e.pulses.tryStart(orgID, dev.ID, receivedAt, pulse) {
				metricSuppressed.WithLabelValues("pulse_active").Inc()
				f.dropped = true
				f.undos = append(f.undos, func() { e.latch.restore(key, lastSeen, seen) })
				continue
			}
			f.drives = append(f.drives, fireDrive{dev: dev, offAfter: tuning.AutoOffSeconds, pulse: pulse})
			f.undos = append(f.undos, func() {
				e.latch.restore(key, lastSeen, seen)
				e.pulses.release(orgID, dev.ID, receivedAt, pulse)
			})
		}

		if len(f.drives) > 0 || f.dropped {
			fires = append(fires, f)
		}
	}
	return fires, refreshes
}

// orgDefaults reads the org tuning tier, bounded by lookupTimeout.
func (e *Engine) orgDefaults(ctx context.Context, orgID int) (organization.GeofenceDefaults, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	return e.store.GetOrgGeofenceDefaults(lookupCtx, orgID)
}

// deviceLookup is one location's outputs for the length of a decide call. A failed
// lookup is kept too, so a stall costs one timeout per location, not one per read.
type deviceLookup struct {
	devices []outputdevice.OutputDevice
	ok      bool
}

// devicesAt lists a location's outputs, bounded by lookupTimeout. A failure is
// logged and metriced here and leaves ok false.
func (e *Engine) devicesAt(ctx context.Context, orgID, locationID int) deviceLookup {
	lookupCtx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	devices, err := e.store.ListOutputDevicesForLocation(lookupCtx, orgID, locationID)
	if err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Int("location_id", locationID).Msg("output device lookup failed")
		metricFireErrors.Inc()
		return deviceLookup{}
	}
	return deviceLookup{devices: devices, ok: true}
}

// driveFire turns on every output of a fire; a fire with no drives does nothing.
// Best-effort: a failed drive is logged and metriced by drive, and the other
// outputs still drive.
func (e *Engine) driveFire(ctx context.Context, orgID int, f fire) {
	for _, d := range f.drives {
		ok := e.drive(ctx, orgID, d.dev, true, d.offAfter)
		if d.presence {
			// A failed ON leaves the output due, so the next read retries it.
			e.presence.driven(orgID, d.dev.ID, f.receivedAt, ok)
			continue
		}
		if !ok {
			// A failed ON must not hold the output busy for the whole window.
			e.pulses.release(orgID, d.dev.ID, f.receivedAt, d.pulse)
		}
	}
}

// refreshPresence re-sends the ON of each held presence output decide found
// due. Best-effort like driveFire; a failed refresh is retried on the next read.
func (e *Engine) refreshPresence(ctx context.Context, orgID int, receivedAt time.Time, devs []outputdevice.OutputDevice) {
	for _, dev := range devs {
		ok := e.drive(ctx, orgID, dev, true, presenceOnSeconds)
		e.presence.driven(orgID, dev.ID, receivedAt, ok)
	}
}

// presenceOnSeconds is the on-time every presence ON carries: the reader's
// default on-time, sent explicitly so the backend and the daemon cannot drift
// apart. A Shelly gets it as toggle_after, a CS463 GPO as pulse_ms.
var presenceOnSeconds = int(readerrpc.GpoDefaultOn / time.Second)

// drive turns a device on/off, folding any error into the best-effort metric.
// It reports whether the drive succeeded.
func (e *Engine) drive(ctx context.Context, orgID int, dev outputdevice.OutputDevice, on bool, offAfter int) bool {
	if err := e.driver.Set(ctx, dev, on, offAfter); err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Int("output_device_id", dev.ID).Bool("on", on).Msg("output device drive failed (best-effort)")
		metricFireErrors.Inc()
		return false
	}
	return true
}

// alarmRow builds the alarm_events row for a fire.
func (e *Engine) alarmRow(f fire, tagScanID int64, receivedAt time.Time) storage.AlarmEventRow {
	return storage.AlarmEventRow{
		AssetID:     f.read.AssetID,
		ScanPointID: f.read.ScanPointID,
		LocationID:  f.read.LocationID,
		EPC:         f.read.EPC,
		RSSI:        f.read.RSSI,
		TagScanID:   tagScanID,
		FiredAt:     receivedAt,
	}
}

// recordFire writes the durable alarm_events row for a fired read, whether or not
// it drove a device, and logs/metrics the fire. Best-effort: a write error never
// blocks ingestion.
func (e *Engine) recordFire(ctx context.Context, orgID int, f fire, tagScanID int64, receivedAt time.Time) {
	rd := f.read
	if err := e.store.InsertAlarmEvent(ctx, orgID, e.alarmRow(f, tagScanID, receivedAt)); err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Str("epc", rd.EPC).Msg("alarm_events write failed")
		metricEventWriteErrors.Inc()
	}
	e.noteFired(orgID, f)
}

// noteFired logs and metrics a fire.
func (e *Engine) noteFired(orgID int, f fire) {
	metricFired.Inc()
	e.log.Info().
		Int("org_id", orgID).
		Int("asset_id", f.read.AssetID).
		Int("scan_point_id", f.read.ScanPointID).
		Str("epc", f.read.EPC).
		Int("rssi", f.read.RSSI).
		Msg("geofence rule fired")
}
