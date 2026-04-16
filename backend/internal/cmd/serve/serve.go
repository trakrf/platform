package serve

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5/pgxpool"

	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	lookuphandler "github.com/trakrf/platform/backend/internal/handlers/lookup"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
	testhandler "github.com/trakrf/platform/backend/internal/handlers/testhandler"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	"github.com/trakrf/platform/backend/internal/logger"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/services/email"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/storage"
)

// Run starts the long-lived HTTP server process. It blocks until ctx is
// canceled (SIGINT / SIGTERM), then performs a graceful shutdown.
//
// frontendFS is the embedded React bundle. The dispatcher owns the go:embed
// directive because its path (frontend/dist) cannot be reached from this
// package's subtree.
func Run(ctx context.Context, version string, frontendFS fs.FS) error {
	startTime := time.Now()
	log := logger.Get()

	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:           dsn,
			Environment:   os.Getenv("APP_ENV"),
			Release:       version,
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

	emailClient := email.NewClient()
	authSvc := authservice.NewService(store.Pool().(*pgxpool.Pool), store, emailClient)
	orgsSvc := orgsservice.NewService(store.Pool().(*pgxpool.Pool), store, emailClient)
	log.Info().Msg("Services initialized")

	authHandler := authhandler.NewHandler(authSvc)
	orgsHandler := orgshandler.NewHandler(store, orgsSvc)
	usersHandler := usershandler.NewHandler(store)
	assetsHandler := assetshandler.NewHandler(store)
	locationsHandler := locationshandler.NewHandler(store)
	inventoryHandler := inventoryhandler.NewHandler(store)
	reportsHandler := reportshandler.NewHandler(store)
	lookupHandler := lookuphandler.NewHandler(store)
	healthHandler := healthhandler.NewHandler(store.Pool().(*pgxpool.Pool), version, startTime)
	frontendHandler := frontendhandler.NewHandler(frontendFS, "frontend/dist")
	testHandler := testhandler.NewHandler(store)
	log.Info().Msg("Handlers initialized")

	r := setupRouter(authHandler, orgsHandler, usersHandler, assetsHandler, locationsHandler, inventoryHandler, reportsHandler, lookupHandler, healthHandler, frontendHandler, testHandler, store)
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
		log.Info().Str("port", port).Str("version", version).Msg("Server starting")
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

	log.Info().Msg("Server stopped")
	return nil
}
