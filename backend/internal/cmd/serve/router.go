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
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/ratelimit"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
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

	// Public OpenAPI spec — served unauthenticated so codegen tools and
	// integrators can fetch it directly from the API host. Root-path aliases
	// (/openapi.{json,yaml}) are added below.
	r.Get("/api/v1/openapi.json", swaggerspec.ServePublicJSON)
	r.Get("/api/v1/openapi.yaml", swaggerspec.ServePublicYAML)

	// Root-path aliases for codegen tools that probe /openapi.{json,yaml}.
	// Registered before any SPA catchall so the redirect wins.
	r.Get("/openapi.json", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/api/v1/openapi.json", http.StatusFound)
	})
	r.Get("/openapi.yaml", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/api/v1/openapi.yaml", http.StatusFound)
	})

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

	// Per-key rate limiter for API-key-authenticated requests (TRA-395).
	// /orgs/me is intentionally excluded as a health-check exemption.
	// Limiter lives for the process lifetime; its sweeper runs in a goroutine.
	rl := ratelimit.NewLimiter(ratelimit.DefaultConfig())

	// Public API — API-key auth (TRA-393 canary)
	r.With(middleware.APIKeyAuth(store)).Get("/api/v1/orgs/me", orgsHandler.GetOrgMe)

	// TRA-396 public read surface — accepts API-key OR session auth via EitherAuth.
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.RateLimit(rl))
		r.Use(middleware.SentryContext)

		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets", assetsHandler.ListAssets)
		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets/{identifier}", assetsHandler.GetAssetByIdentifier)

		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations", locationsHandler.ListLocations)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{identifier}", locationsHandler.GetLocationByIdentifier)

		// Scan-class endpoints (logical scan events, current-locations snapshot, asset movement history)
		// require scans:read per TRA-392 — they moved under /locations/ and /assets/ for URL
		// aesthetics but are scan data, not asset/location CRUD data.
		r.With(middleware.RequireScope("scans:read")).Get("/api/v1/locations/current", reportsHandler.ListCurrentLocations)
		r.With(middleware.RequireScope("scans:read")).Get("/api/v1/assets/{identifier}/history", reportsHandler.GetAssetHistory)
	})

	// TRA-397 public write surface — accepts API-key OR session auth via EitherAuth.
	// Every route is audited via WriteAudit and gated by a per-resource write scope.
	r.Group(func(r chi.Router) {
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.WriteAudit)
		r.Use(middleware.SentryContext)

		// Assets
		r.With(middleware.RequireScope("assets:write")).Post("/api/v1/assets", assetsHandler.Create)
		r.With(middleware.RequireScope("assets:write")).Put("/api/v1/assets/{identifier}", assetsHandler.UpdateAsset)
		r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{identifier}", assetsHandler.DeleteAsset)
		r.With(middleware.RequireScope("assets:write")).Post("/api/v1/assets/{identifier}/identifiers", assetsHandler.AddIdentifier)
		r.With(middleware.RequireScope("assets:write")).Delete("/api/v1/assets/{identifier}/identifiers/{identifierId}", assetsHandler.RemoveIdentifier)

		// Locations
		r.With(middleware.RequireScope("locations:write")).Post("/api/v1/locations", locationsHandler.Create)
		r.With(middleware.RequireScope("locations:write")).Put("/api/v1/locations/{identifier}", locationsHandler.Update)
		r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{identifier}", locationsHandler.Delete)
		r.With(middleware.RequireScope("locations:write")).Post("/api/v1/locations/{identifier}/identifiers", locationsHandler.AddIdentifier)
		r.With(middleware.RequireScope("locations:write")).Delete("/api/v1/locations/{identifier}/identifiers/{identifierId}", locationsHandler.RemoveIdentifier)

		// Inventory (scan writes)
		r.With(middleware.RequireScope("scans:write")).Post("/api/v1/inventory/save", inventoryHandler.Save)
	})

	// TRA-396 internal-only surrogate paths — session auth only, for frontend convenience.
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.SentryContext)

		r.Get("/api/v1/assets/by-id/{id}", assetsHandler.GetAssetByID)
		r.Get("/api/v1/assets/by-id/{id}/history", reportsHandler.GetAssetHistoryByID)
		r.Get("/api/v1/locations/by-id/{id}", locationsHandler.GetLocationByID)
	})

	if os.Getenv("APP_ENV") != "production" {
		testHandler.RegisterRoutes(r)
	}

	// JSON 404 for unknown /api/* paths so clients don't blow up mid-deserialize
	// on the SPA's index.html fallback. Must be registered before the /* wildcard.
	r.HandleFunc("/api/*", func(w http.ResponseWriter, req *http.Request) {
		httputil.WriteJSONError(w, req, http.StatusNotFound, errors.ErrNotFound,
			"Unknown API route: "+req.URL.Path, "", middleware.GetRequestID(req.Context()))
	})

	r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		frontendHandler.ServeSPA(w, r, "frontend/dist/index.html")
	})

	return r
}
