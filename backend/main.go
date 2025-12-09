package main

import (
	"context"
	"embed"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	httpSwagger "github.com/swaggo/http-swagger"
	_ "github.com/trakrf/platform/backend/docs"
	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/middleware"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/services/email"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/storage"
)

//go:embed frontend/dist
var frontendFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

var (
	version   = "dev"
	startTime time.Time
)

func runMigrations(pool *pgxpool.Pool) error {
	log := logger.Get()

	// Convert pgxpool to *sql.DB for golang-migrate
	db := stdlib.OpenDBFromPool(pool)

	// Create iofs source from embedded migrations
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	// Create postgres driver
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	// Create migrator
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	// Run migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	migrationVersion, dirty, _ := m.Version()
	log.Info().Uint("version", migrationVersion).Bool("dirty", dirty).Msg("Migrations complete")

	return nil
}

// @title TrakRF Platform API
// @version 1.0
// @description Multi-tenant platform API with authentication and organization management
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token

func setupRouter(
	authHandler *authhandler.Handler,
	orgsHandler *orgshandler.Handler,
	usersHandler *usershandler.Handler,
	assetsHandler *assetshandler.Handler,
	locationsHandler *locationshandler.Handler,
	healthHandler *healthhandler.Handler,
	frontendHandler *frontendhandler.Handler,
	store *storage.Storage,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(logger.Middleware)
	r.Use(middleware.Recovery)
	r.Use(middleware.CORS)
	r.Use(middleware.ContentType)

	r.Handle("/assets/*", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/favicon.ico", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/icon-*", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/logo.png", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/manifest.json", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/og-image.png", http.HandlerFunc(frontendHandler.ServeFrontend))

	r.Get("/swagger/*", httpSwagger.WrapHandler)

	healthHandler.RegisterRoutes(r)

	authHandler.RegisterRoutes(r)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)

		orgsHandler.RegisterRoutes(r, store)
		orgsHandler.RegisterMeRoutes(r)
		usersHandler.RegisterRoutes(r)
		assetsHandler.RegisterRoutes(r)
		locationsHandler.RegisterRoutes(r)
	})

	r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		frontendHandler.ServeSPA(w, r, "frontend/dist/index.html")
	})

	return r
}

func main() {
	startTime = time.Now()

	loggerCfg := logger.NewConfig(version)
	logger.Initialize(loggerCfg)
	log := logger.Get()

	log.Info().Msg("Logger initialized")

	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}

	ctx := context.Background()
	store, err := storage.New(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize storage")
		os.Exit(1)
	}
	log.Info().Msg("Storage initialized")

	if err := runMigrations(store.Pool().(*pgxpool.Pool)); err != nil {
		log.Error().Err(err).Msg("Failed to run migrations")
		os.Exit(1)
	}
	log.Info().Msg("Migrations applied")

	emailClient := email.NewClient()
	authSvc := authservice.NewService(store.Pool().(*pgxpool.Pool), store, emailClient)
	orgsSvc := orgsservice.NewService(store.Pool().(*pgxpool.Pool), store, emailClient)
	log.Info().Msg("Services initialized")

	authHandler := authhandler.NewHandler(authSvc)
	orgsHandler := orgshandler.NewHandler(store, orgsSvc)
	usersHandler := usershandler.NewHandler(store)
	assetsHandler := assetshandler.NewHandler(store)
	locationsHandler := locationshandler.NewHandler(store)
	healthHandler := healthhandler.NewHandler(store.Pool().(*pgxpool.Pool), version, startTime)
	frontendHandler := frontendhandler.NewHandler(frontendFS, "frontend/dist")
	log.Info().Msg("Handlers initialized")

	r := setupRouter(authHandler, orgsHandler, usersHandler, assetsHandler, locationsHandler, healthHandler, frontendHandler, store)
	log.Info().Msg("Routes registered")

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info().Str("port", port).Str("version", version).Msg("Server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Server failed")
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Shutdown error")
	}

	store.Close()
	log.Info().Msg("Server stopped")
}
