// Command mqtt-rpcd is the standalone on-reader MQTT JSON-RPC control daemon.
// It runs ON a CSL CS463 reader: it auto-discovers its broker config from the
// reader's own EmbeddedGlassFish CloudServer (see internal/readerd.LoadConfig),
// builds a CS463 HTTP client pointed at the reader's localhost servlet API, and
// serves the neutral readerrpc contract until SIGINT/SIGTERM.
//
// Because it runs on the reader, READER_API_URL defaults to http://localhost.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/mqtt-rpc/internal/hwrng"
	"github.com/trakrf/platform/mqtt-rpc/internal/readerd"
	"github.com/trakrf/platform/mqtt-rpc/internal/readerd/cs463"
	"github.com/trakrf/platform/mqtt-rpc/internal/update"
)

// version is overridable via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logger := zerolog.New(os.Stderr).With().
		Timestamp().
		Str("service", "mqtt-rpcd").
		Str("version", version).
		Logger()

	// First, before anything reaches for crypto/rand: credit the reader's
	// hardware RNG to the kernel. Without it the CS463's kernel takes 90–390 s
	// after a boot to seed itself, and every TLS client on the reader — this
	// daemon's broker link and the vendor app's CloudServer — waits that long.
	seedKernelRNG(&logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := readerd.LoadConfig()
	if err != nil {
		logger.Fatal().Err(err).Msg("load config")
	}
	cfg.Version = version

	// Remote self-update. CheckPending runs first, before anything touches the
	// reader: if this process is a fresh update on probation, it commits only
	// after answering an RPC through the broker (see readerd.selfTest) — never on
	// reconcile, which a wedged vendor app would fail.
	restart := make(chan struct{}, 1)
	upd := update.New(update.Layout{Dir: update.DefaultDir}, version, func() {
		select {
		case restart <- struct{}{}:
		default:
		}
	}, logger)
	if err := upd.CheckPending(); err != nil {
		logger.Error().Err(err).Msg("read update state")
	}
	if upd.OnProbation() {
		logger.Warn().Msg("this build is an update on probation; it commits after answering an rpc through the broker, else the guard rolls it back")
	}

	// The daemon runs on the reader and talks to the reader's localhost servlet
	// API. URL/user/pass are overridable via env; pass is the only secret.
	baseURL := envOr("READER_API_URL", "http://localhost")
	user := envOr("READER_API_USER", "trakrf")
	pass := os.Getenv("READER_API_PASS")
	client := cs463.New(baseURL, user, pass, 0)

	adapter := cs463.NewAdapter(client, cs463.AdapterConfig{
		AntennaCount: cfg.AntennaCount,
		EventID:      cfg.EventID,
		Logger:       &logger,
		// Every GPO off also goes through the kernel's GPIO lines, so it lands
		// while the vendor app is wedged. Start and Stop drive all four off.
		GPO: cs463.SysfsGPO(),
		// No ON latches: one with no pulse gets this on-time, and every ON is
		// capped at readerrpc.GpoMaxOn.
		DefaultOn: cfg.GpoDefaultOn,
	})

	// The golden-config reconcile runs from the health monitor, whenever /API
	// is healthy and the config is not yet applied: at boot, once GlassFish is
	// actually serving, and again after every outage. Running it once before
	// serving lost the boot race to GlassFish (a 502) and never retried.
	//
	// The daemon is opinionated about the ONE hand-crafted prerequisite, the
	// golden CloudServer. If it is missing, it exits (fatal): a missing or
	// misnamed server is a permanent commissioning error, and serving a
	// half-configured reader is worse than not serving. With systemd
	// Restart=always, the daemon recovers once the operator creates it. Anything
	// else (reader busy, a transient error) is retried by the monitor.
	var reconcile func(context.Context) error
	if cfg.Reconcile {
		reconcile = func(ctx context.Context) error {
			err := adapter.Reconcile(ctx)
			if errors.Is(err, cs463.ErrMissingCloudServer) {
				logger.Fatal().Err(err).Msg("golden-config reconcile: required CloudServer missing — refusing to serve; create it (exact name) and the daemon will recover on restart")
			}
			return err
		}
	}

	// One host for the operator's Reader.Reboot/Shutdown and the monitor's
	// automatic recovery.
	host := readerd.NewSystemdHost(&logger)
	d := readerd.New(cfg, adapter, &logger)
	d.SetUpdater(upd)
	d.SetHost(host)
	if err := d.Start(); err != nil {
		logger.Fatal().Err(err).Msg("start daemon")
	}
	defer d.Stop()

	policy := readerd.DefaultHealthPolicy()
	policy.AutoRecover = cfg.AutoRecover
	policy.WedgeAfter = cfg.WedgeAfter
	monitor := readerd.NewMonitor(policy, readerd.MonitorDeps{
		Probe:          adapter.ProbeHealth,
		Reconcile:      reconcile,
		Host:           host,
		Store:          readerd.RebootStore{Path: readerd.DefaultRebootStorePath},
		Publish:        d.SetHealth,
		GoingDown:      d.GoingDown,
		AbortGoingDown: d.AbortGoingDown,
		CloudServer: func() string {
			return d.CloudServerFault(cfg.CloudServer, time.Now())
		},
		Log: &logger,
	})
	d.SetHealth(monitor.Health())
	go monitor.Run(ctx)

	logger.Info().
		Str("base_topic", cfg.Broker.BaseTopic).
		Str("rpc_client_id", cfg.Broker.RPCClientID).
		Str("reader_url", baseURL).
		Bool("auto_recover", policy.AutoRecover).
		Dur("wedge_after", policy.WedgeAfter).
		Msg("mqtt-rpcd running")

	select {
	case <-ctx.Done():
		logger.Info().Msg("mqtt-rpcd shutting down")
	case <-restart:
		// Exit cleanly; systemd (Restart=always) starts the binary the active
		// link now points at, and the guard's ExecStartPre counts the start.
		logger.Info().Msg("mqtt-rpcd exiting to restart onto the updated version")
	}
}

// seedKernelRNG never stops the daemon: a failure costs only the old slow boot.
func seedKernelRNG(logger *zerolog.Logger) {
	res := hwrng.Seed(hwrng.Config{})
	switch {
	case res.Skipped:
		logger.Info().Msg("no hardware RNG; the kernel seeds itself")
	case res.Err != nil:
		logger.Warn().Err(res.Err).Str("source", res.Source).
			Int("entropy_avail", res.AvailBefore).
			Msg("could not credit the hardware RNG; TLS may wait for the kernel to seed itself")
	default:
		logger.Info().Str("source", res.Source).Int("bits", res.Bits).
			Int("entropy_avail_before", res.AvailBefore).
			Int("entropy_avail_after", res.AvailAfter).
			Msg("credited the hardware RNG to the kernel")
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
