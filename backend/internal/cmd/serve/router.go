// Package serve runs the long-lived HTTP server process. It wires storage,
// services, handlers, middleware, and graceful shutdown. It does not perform
// schema migrations — those are the responsibility of the migrate subcommand
// (or the transitional combined default in main.go).
package serve

import (
	"net/http"
	"os"

	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger"

	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	lookuphandler "github.com/trakrf/platform/backend/internal/handlers/lookup"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
	"github.com/trakrf/platform/backend/internal/handlers/swaggerspec"
	testhandler "github.com/trakrf/platform/backend/internal/handlers/testhandler"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/storage"
)

func setupRouter(
	authHandler *authhandler.Handler,
	orgsHandler *orgshandler.Handler,
	usersHandler *usershandler.Handler,
	assetsHandler *assetshandler.Handler,
	locationsHandler *locationshandler.Handler,
	inventoryHandler *inventoryhandler.Handler,
	reportsHandler *reportshandler.Handler,
	lookupHandler *lookuphandler.Handler,
	healthHandler *healthhandler.Handler,
	frontendHandler *frontendhandler.Handler,
	testHandler *testhandler.Handler,
	store *storage.Storage,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(logger.Middleware)
	r.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle)
	r.Use(middleware.Recovery)
	r.Use(middleware.CORS)
	r.Use(middleware.ContentType)

	r.Handle("/assets/*", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/favicon.ico", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/icon-*", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/logo.png", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/manifest.json", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/og-image.png", http.HandlerFunc(frontendHandler.ServeFrontend))

	r.Handle("/metrics", promhttp.Handler())

	healthHandler.RegisterRoutes(r)

	authHandler.RegisterRoutes(r, middleware.Auth)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.SentryContext)

		orgsHandler.RegisterRoutes(r, store)
		orgsHandler.RegisterMeRoutes(r)
		usersHandler.RegisterRoutes(r)
		assetsHandler.RegisterRoutes(r)
		locationsHandler.RegisterRoutes(r)
		inventoryHandler.RegisterRoutes(r)
		reportsHandler.RegisterRoutes(r)
		lookupHandler.RegisterRoutes(r)

		r.Get("/swagger/openapi.internal.json", swaggerspec.ServeJSON)
		r.Get("/swagger/openapi.internal.yaml", swaggerspec.ServeYAML)
		r.Get("/swagger/*", httpSwagger.Handler(
			httpSwagger.URL("/swagger/openapi.internal.json"),
		))
	})

	if os.Getenv("APP_ENV") != "production" {
		testHandler.RegisterRoutes(r)
	}

	r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		frontendHandler.ServeSPA(w, r, "frontend/dist/index.html")
	})

	return r
}
