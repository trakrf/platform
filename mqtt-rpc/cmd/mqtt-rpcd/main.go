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

	d := readerd.New(cfg, adapter, &logger)
	d.SetUpdater(upd)
	if err := d.Start(); err != nil {
		logger.Fatal().Err(err).Msg("start daemon")
	}
	defer d.Stop()

	policy := readerd.DefaultHealthPolicy()
	policy.AutoRecover = cfg.AutoRecover
	policy.WedgeAfter = cfg.WedgeAfter
	monitor := readerd.NewMonitor(policy, readerd.MonitorDeps{
		Probe:     adapter.ProbeHealth,
		Reconcile: reconcile,
		Host:      readerd.NewSystemdHost(&logger),
		Store:     readerd.RebootStore{Path: readerd.DefaultRebootStorePath},
		Publish:   d.SetHealth,
		GoingDown: d.GoingDown,
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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
