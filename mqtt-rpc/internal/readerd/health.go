package readerd

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// HealthPolicy sets the monitor's timings and limits.
type HealthPolicy struct {
	// Interval is how often /API is probed.
	Interval time.Duration
	// WedgeAfter is how long /API must stay unhealthy before the reader counts
	// as wedged. It is also the wait after each recovery action. A boot takes
	// about 3 min to serve (TLS entropy stall, then the GlassFish deploy), so
	// it must be longer than that.
	WedgeAfter time.Duration
	// MaxAutoReboots caps automatic reboots until the API is healthy again for
	// ResetAfter.
	MaxAutoReboots int
	ResetAfter     time.Duration
	// ReconcileRetry spaces reconcile attempts that fail while /API is healthy.
	ReconcileRetry time.Duration
	// AutoRecover enables the recovery actions. Off, the monitor only reports.
	AutoRecover bool
}

// DefaultHealthPolicy is the production policy.
func DefaultHealthPolicy() HealthPolicy {
	return HealthPolicy{
		Interval:       30 * time.Second,
		WedgeAfter:     6 * time.Minute,
		MaxAutoReboots: 2,
		ResetAfter:     time.Hour,
		ReconcileRetry: 5 * time.Minute,
		AutoRecover:    true,
	}
}

const (
	probeTimeout      = 10 * time.Second
	reconcileTimeout  = 30 * time.Second
	appRestartTimeout = 2 * time.Minute
	// outageProbes is how many failed probes in a row make an outage that needs
	// the reconcile (and its inventory re-arm) once /API is back. One failed
	// probe is a blip, not a reason to pause inventory.
	outageProbes = 2
)

// rebootCounter is the persisted automatic-reboot count (RebootStore).
type rebootCounter interface {
	Count() (int, error)
	Record(at time.Time) error
	Reset() error
}

// MonitorDeps are the monitor's collaborators.
type MonitorDeps struct {
	// Probe returns a readerrpc.API* state and a reason.
	Probe func(ctx context.Context) (state, detail string)
	// Reconcile applies the golden config; nil means reconcile is off.
	Reconcile func(ctx context.Context) error
	Host      Host
	Store     rebootCounter
	// Publish receives the health whenever it changes.
	Publish func(readerrpc.Health)
	// GoingDown is called just before an automatic reboot, with the reason.
	GoingDown func(reason string)
	Log       *zerolog.Logger
	Now       func() time.Time
}

// Monitor probes the reader API and drives the golden-config reconcile and the
// wedge recovery from what it finds. Tick is called from one goroutine (Run);
// Health is safe from any.
//
// It recovers in steps, once per episode. The first step restarts the vendor
// app. If /API is still unhealthy a further WedgeAfter later, the second step
// is a clean reboot, capped by MaxAutoReboots. Past the cap the daemon stays up
// and reports auto_reboots_exhausted for a person to decide.
type Monitor struct {
	policy HealthPolicy
	deps   MonitorDeps
	log    zerolog.Logger

	mu sync.Mutex
	h  readerrpc.Health // guarded by mu; what Health returns

	// Owned by Tick.
	published     *readerrpc.Health
	badSince      time.Time // start of the current unhealthy window; zero while healthy
	goodSince     time.Time // start of the current healthy run; zero while unhealthy
	failures      int       // consecutive unhealthy probes
	step          string    // last recovery action this episode
	needReconcile bool
	lastReconcile time.Time
}

// NewMonitor builds a monitor. It reads the persisted reboot count once so the
// first status already carries it.
func NewMonitor(p HealthPolicy, deps MonitorDeps) *Monitor {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	log := zerolog.Nop()
	if deps.Log != nil {
		log = *deps.Log
	}
	m := &Monitor{
		policy:        p,
		deps:          deps,
		log:           log.With().Str("component", "health").Logger(),
		needReconcile: deps.Reconcile != nil,
	}
	m.h = readerrpc.Health{API: readerrpc.APIUnknown, GoldenConfig: readerrpc.GoldenOff}
	if deps.Reconcile != nil {
		m.h.GoldenConfig = readerrpc.GoldenPending
	}
	if n, err := deps.Store.Count(); err == nil {
		m.h.AutoReboots = n
	}
	return m
}

// Health returns the current reader-API health.
func (m *Monitor) Health() readerrpc.Health {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.h
}

// Run probes immediately, then every Interval, until ctx is done.
func (m *Monitor) Run(ctx context.Context) {
	t := time.NewTicker(m.policy.Interval)
	defer t.Stop()
	for {
		m.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick runs one probe and acts on it.
func (m *Monitor) Tick(ctx context.Context) {
	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	state, detail := m.deps.Probe(pctx)
	cancel()
	now := m.deps.Now()

	h := m.Health()
	if state != h.API {
		m.log.Info().Str("from", h.API).Str("to", state).Str("detail", detail).Msg("reader api health changed")
		h.API, h.Since = state, now.UTC().Format(time.RFC3339)
	}
	h.Detail = detail

	switch state {
	case readerrpc.APIUnknown:
		// The adapter cannot probe; nothing to act on.
	case readerrpc.APIHealthy:
		m.healthy(ctx, now, &h)
	default:
		m.unhealthy(ctx, now, &h)
	}
	m.set(h)
}

func (m *Monitor) healthy(ctx context.Context, now time.Time, h *readerrpc.Health) {
	if m.goodSince.IsZero() {
		if h.Wedged {
			m.log.Info().Str("after", m.step).Msg("reader api recovered")
		}
		m.goodSince = now
	}
	m.badSince, m.failures, m.step = time.Time{}, 0, ""
	h.Wedged, h.Recovery, h.AutoRebootsExhausted = false, "", false

	if h.AutoReboots > 0 && now.Sub(m.goodSince) >= m.policy.ResetAfter {
		if err := m.deps.Store.Reset(); err != nil {
			m.log.Error().Err(err).Msg("reset the automatic-reboot count")
		} else {
			m.log.Info().Dur("healthy_for", now.Sub(m.goodSince)).Msg("automatic-reboot count reset")
			h.AutoReboots = 0
		}
	}

	if !m.needReconcile || m.deps.Reconcile == nil {
		return
	}
	if !m.lastReconcile.IsZero() && now.Sub(m.lastReconcile) < m.policy.ReconcileRetry {
		return
	}
	m.lastReconcile = now
	rctx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	err := m.deps.Reconcile(rctx)
	cancel()
	if err != nil {
		m.log.Error().Err(err).Dur("retry_in", m.policy.ReconcileRetry).Msg("golden-config reconcile failed; will retry")
		return
	}
	m.needReconcile = false
	h.GoldenConfig = readerrpc.GoldenApplied
	m.log.Info().Msg("golden-config reconcile complete")
}

func (m *Monitor) unhealthy(ctx context.Context, now time.Time, h *readerrpc.Health) {
	m.goodSince = time.Time{}
	m.failures++
	if m.failures >= outageProbes && m.deps.Reconcile != nil {
		m.needReconcile, m.lastReconcile = true, time.Time{}
		h.GoldenConfig = readerrpc.GoldenPending
	}
	if m.badSince.IsZero() {
		m.badSince = now
	}
	if now.Sub(m.badSince) < m.policy.WedgeAfter {
		return
	}
	if !h.Wedged {
		m.log.Error().Str("api", h.API).Str("detail", h.Detail).Dur("for", now.Sub(m.badSince)).Msg("reader api wedged")
	}
	h.Wedged = true
	if !m.policy.AutoRecover {
		return
	}
	// Each action starts a fresh window, so the next step waits a full WedgeAfter.
	m.badSince = now
	if m.step == "" {
		m.restartApp(ctx, h)
		return
	}
	m.reboot(now, h)
}

func (m *Monitor) restartApp(ctx context.Context, h *readerrpc.Health) {
	m.step = readerrpc.RecoveryAppRestart
	h.Recovery = m.step
	m.set(*h) // report the wedge before a restart that can take a minute
	m.log.Warn().Msg("auto-recovery: restarting the reader app")
	actx, cancel := context.WithTimeout(ctx, appRestartTimeout)
	defer cancel()
	if err := m.deps.Host.RestartReaderApp(actx); err != nil {
		m.log.Error().Err(err).Msg("auto-recovery: reader app restart failed; a reboot follows if it stays wedged")
	}
}

func (m *Monitor) reboot(now time.Time, h *readerrpc.Health) {
	n, err := m.deps.Store.Count()
	if err != nil {
		// A count that cannot be read is a cap that cannot be enforced.
		m.exhausted(h, err)
		return
	}
	h.AutoReboots = n
	if n >= m.policy.MaxAutoReboots {
		m.exhausted(h, nil)
		return
	}
	if err := m.deps.Store.Record(now); err != nil {
		m.exhausted(h, err)
		return
	}
	m.step = readerrpc.RecoveryReboot
	h.Recovery, h.AutoReboots = m.step, n+1
	m.set(*h)
	m.log.Warn().Int("auto_reboots", n+1).Int("max", m.policy.MaxAutoReboots).Msg("auto-recovery: rebooting the reader")
	if m.deps.GoingDown != nil {
		m.deps.GoingDown(readerrpc.StatusReasonAutoRecovery)
	}
	if err := m.deps.Host.Reboot(); err != nil {
		m.log.Error().Err(err).Msg("auto-recovery: reboot failed; retrying after the next window")
	}
}

func (m *Monitor) exhausted(h *readerrpc.Health, err error) {
	if !h.AutoRebootsExhausted {
		ev := m.log.Error()
		if err != nil {
			ev = ev.Err(err)
		}
		ev.Int("auto_reboots", h.AutoReboots).Msg("auto-recovery: no automatic reboot left; staying up for a person to decide")
	}
	h.AutoRebootsExhausted = true
}

// set stores h and publishes it if it changed.
func (m *Monitor) set(h readerrpc.Health) {
	m.mu.Lock()
	m.h = h
	m.mu.Unlock()
	if m.published != nil && *m.published == h {
		return
	}
	m.published = &h
	if m.deps.Publish != nil {
		m.deps.Publish(h)
	}
}
