package serve

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/trakrf/platform/backend/internal/alarm"
	"github.com/trakrf/platform/backend/internal/alarm/shelly"
	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/internal/geofence"
	alarmdeviceshandler "github.com/trakrf/platform/backend/internal/handlers/alarmdevices"
	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	lookuphandler "github.com/trakrf/platform/backend/internal/handlers/lookup"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	readstreamhandler "github.com/trakrf/platform/backend/internal/handlers/readstream"
	reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
	scandeviceshandler "github.com/trakrf/platform/backend/internal/handlers/scandevices"
	scanpointshandler "github.com/trakrf/platform/backend/internal/handlers/scanpoints"
	testhandler "github.com/trakrf/platform/backend/internal/handlers/testhandler"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	"github.com/trakrf/platform/backend/internal/ingest"
	"github.com/trakrf/platform/backend/internal/logger"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/services/email"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	readstreamsvc "github.com/trakrf/platform/backend/internal/services/readstream"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// Run starts the long-lived HTTP server process. It blocks until ctx is
// canceled (SIGINT / SIGTERM), then performs a graceful shutdown.
//
// frontendFS is the embedded React bundle. The dispatcher owns the go:embed
// directive because its path (frontend/dist) cannot be reached from this
// package's subtree.
func Run(ctx context.Context, info buildinfo.Info, frontendFS fs.FS) error {
	startTime := time.Now()
	log := logger.Get()

	// Fail fast: a deployed environment must never sign tokens with a known-weak
	// secret (unset → dev fallback, or the "change-me" chart default), which
	// would let anyone forge a Bearer for any org. Refuse to boot instead.
	if err := jwt.ValidateSecret(); err != nil {
		log.Error().Err(err).Msg("Refusing to start: insecure JWT_SECRET")
		return err
	}

	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:           dsn,
			Environment:   os.Getenv("APP_ENV"),
			Release:       info.Version,
			EnableTracing: false,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Sentry initialization failed")
		} else {
			log.Info().Msg("Sentry initialized")
		}
	}
	defer sentry.Flush(2 * time.Second)

	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}

	store, err := storage.New(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize storage")
		return err
	}
	defer store.Close()
	log.Info().Msg("Storage initialized")

	// TRA-900: in-backend MQTT subscriber (replaces the RC ingester + the
	// process_tag_scans trigger). Disabled when MQTT_URL is unset, so local
	// dev / tests / pre-cutover prod stay inert.
	// TRA-903/906: the alarm Dispatcher routes a fire to the right transport per
	// device — local HTTP (Shelly RPC) or MQTT publish to the shared broker. It
	// is shared by the geofence firer (auto-fire on boundary trip) and the
	// alarm-device test-fire/reset endpoints. The MQTT publisher only exists when
	// the broker is configured; when it isn't, mqtt-transport devices fail with a
	// clear error and the http path is unaffected.
	shellyClient := shelly.New(0)

	// TRA-924: in-process per-org read broadcaster backing the Live Reads SSE
	// endpoint. Always constructed so the endpoint serves (heartbeat-only when
	// ingestion is off); the subscriber publishes parsed reads into it. Single
	// replica only — multi-replica fan-out is deferred (TRA-907).
	readBroadcaster := readstreamsvc.New()

	mqttCfg := ingest.ConfigFromEnv()
	var alarmDispatcher alarm.Dispatcher
	if mqttCfg.Enabled() {
		// TRA-906: dedicated publish client on the same broker (reuses MQTT_URL).
		alarmPublisher, stopPublisher := alarm.NewMQTTPublisher(mqttCfg, log)
		defer stopPublisher()
		alarmDispatcher = alarm.NewDispatcher(shellyClient, alarmPublisher)

		// TRA-901: geofence engine evaluates the membership-passing reads the
		// subscriber derives, firing boundary alarms. Its lifecycle is tied to the
		// subscriber's (only meaningful when ingestion is on). TRA-903/906: the
		// alarm.Firer drives the bound devices via the Dispatcher.
		geofenceEngine := geofence.NewEngine(geofence.ConfigFromEnv(), store, alarm.NewFirer(store, alarmDispatcher, log), log)
		geofenceEngine.Start()
		defer geofenceEngine.Stop()

		subscriber := ingest.NewSubscriber(mqttCfg, store, geofenceEngine, readBroadcaster, log)
		if err := subscriber.Start(); err != nil {
			log.Error().Err(err).Msg("Failed to start MQTT subscriber")
			return err
		}
		defer subscriber.Stop()
		log.Info().Msg("MQTT subscriber started")
	} else {
		// No broker: http-only dispatcher (nil mqtt → mqtt devices error clearly).
		alarmDispatcher = alarm.NewDispatcher(shellyClient, nil)
		log.Info().Msg("MQTT subscriber disabled (MQTT_URL unset)")
	}

	emailClient := email.NewClient()
	authSvc := authservice.NewService(store.Pool().(*pgxpool.Pool), store, emailClient)
	orgsSvc := orgsservice.NewService(store.Pool().(*pgxpool.Pool), store, emailClient)
	log.Info().Msg("Services initialized")

	authHandler := authhandler.NewHandler(authSvc, store)
	orgsHandler := orgshandler.NewHandler(store, orgsSvc, authSvc)
	usersHandler := usershandler.NewHandler(store)
	assetsHandler := assetshandler.NewHandler(store)
	locationsHandler := locationshandler.NewHandler(store)
	inventoryHandler := inventoryhandler.NewHandler(store)
	reportsHandler := reportshandler.NewHandler(store)
	scanDevicesHandler := scandeviceshandler.NewHandler(store)
	scanPointsHandler := scanpointshandler.NewHandler(store)
	// 2s test-fire pulse: long enough for an operator to see the strobe, short
	// enough not to leave the relay latched after a confidence check.
	alarmDevicesHandler := alarmdeviceshandler.NewHandler(store, alarmDispatcher, 2*time.Second)
	lookupHandler := lookuphandler.NewHandler(store)
	healthHandler := healthhandler.NewHandler(store.Pool().(*pgxpool.Pool), info, startTime)
	// TRA-924: Live Reads is now served by the org-enforced SSE endpoint, so the
	// browser no longer receives broker URL/creds — the readerFeed runtime config
	// is gone.
	frontendHandler := frontendhandler.NewHandler(frontendFS, "frontend/dist", os.Getenv("ENVIRONMENT_LABEL"))
	readstreamHandler := readstreamhandler.NewHandler(readBroadcaster)
	testHandler := testhandler.NewHandler(store)
	log.Info().Msg("Handlers initialized")

	r := setupRouter(authHandler, orgsHandler, usersHandler, assetsHandler, locationsHandler, inventoryHandler, reportsHandler, scanDevicesHandler, scanPointsHandler, alarmDevicesHandler, lookupHandler, healthHandler, frontendHandler, readstreamHandler, testHandler, store)
	log.Info().Msg("Routes registered")

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info().
			Str("port", port).
			Str("version", info.Version).
			Str("commit", info.Commit).
			Str("tag", info.Tag).
			Msg("Server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			log.Error().Err(err).Msg("Server failed")
			return err
		}
	case <-ctx.Done():
	}

	log.Info().Msg("Shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Shutdown error")
		return err
	}

	<-serverErr

	log.Info().Msg("Server stopped")
	return nil
}
