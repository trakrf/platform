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
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger"

	antennapowerhandler "github.com/trakrf/platform/backend/internal/handlers/antennapower"
	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	lookuphandler "github.com/trakrf/platform/backend/internal/handlers/lookup"
	musteringhandler "github.com/trakrf/platform/backend/internal/handlers/mustering"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	outputdeviceshandler "github.com/trakrf/platform/backend/internal/handlers/outputdevices"
	readstreamhandler "github.com/trakrf/platform/backend/internal/handlers/readstream"
	reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
	scandeviceshandler "github.com/trakrf/platform/backend/internal/handlers/scandevices"
	scanpointshandler "github.com/trakrf/platform/backend/internal/handlers/scanpoints"
	"github.com/trakrf/platform/backend/internal/handlers/swaggerspec"
	testhandler "github.com/trakrf/platform/backend/internal/handlers/testhandler"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/middleware"
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
	scanDevicesHandler *scandeviceshandler.Handler,
	scanPointsHandler *scanpointshandler.Handler,
	outputDevicesHandler *outputdeviceshandler.Handler,
	antennaPowerHandler *antennapowerhandler.Handler,
	lookupHandler *lookuphandler.Handler,
	healthHandler *healthhandler.Handler,
	frontendHandler *frontendhandler.Handler,
	readstreamHandler *readstreamhandler.Handler,
	musteringHandler *musteringhandler.Handler,
	testHandler *testhandler.Handler,
	store *storage.Storage,
) *chi.Mux {
	r := chi.NewRouter()

	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		allowed := computeAllowedMethods(r, req.URL.Path)
		httputil.Respond405(w, req, allowed, middleware.GetRequestID(req.Context()))
	})

	// Per-key rate limiter for API-key-authenticated requests (TRA-395).
	// Limiter lives for the process lifetime; its sweeper runs in a goroutine.
	//
	// Constructed up-front so APIv1DefaultRateLimitHeaders (a global, path-
	// scoped middleware) can stamp X-RateLimit-* on every /api/v1/* response
	// before ContentType has a chance to reject with 415 (TRA-703 / BB32 C1).
	// Per-group DefaultRateLimitHeaders wraps each /api/v1/* group below to
	// keep a clean reset point for RateLimit (TRA-518); RateLimit then
	// overwrites the defaults with real per-key bucket values.
	rl := ratelimit.NewLimiter(ratelimit.DefaultConfig())

	r.Use(middleware.RequestID)
	r.Use(logger.Middleware)
	r.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle)
	r.Use(middleware.Recovery)
	r.Use(middleware.CORS)
	r.Use(middleware.APIv1DefaultRateLimitHeaders(rl))
	// ContentType is intentionally NOT global. Applying it globally would
	// reject POST/PUT/PATCH probes against retired and static-only paths
	// (`/api/v1/{assets,locations}/lookup`, `/api/v1/locations/current`,
	// the /api/v1/orgs/me method-allow guards) with 415 before chi's
	// routing-precedence guards return their normalized 404/405. Each
	// group below that registers a real write handler attaches ContentType
	// at the group head; groups that register only GETs or 404/405 guards
	// deliberately omit it.
	r.Use(chimiddleware.GetHead)

	r.Handle("/assets/*", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/favicon.ico", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/icon-*", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/logo.png", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/manifest.json", http.HandlerFunc(frontendHandler.ServeFrontend))
	r.Handle("/og-image.png", http.HandlerFunc(frontendHandler.ServeFrontend))
	// TRA-481: curl-able SPA build metadata, generated by a Vite plugin at
	// build time. Specific route entry so the SPA fallback doesn't swallow it.
	r.Handle("/version.json", http.HandlerFunc(frontendHandler.ServeFrontend))

	r.Handle("/metrics", promhttp.Handler())

	// Public OpenAPI spec — served unauthenticated so codegen tools and
	// integrators can fetch it directly from the API host. Canonical paths
	// are /api/openapi.{json,yaml} (TRA-693 / BB30 §2.3); the /api/v1/
	// variants and the root-path aliases redirect so codegen tools probing
	// them don't fork on a duplicated payload.
	r.Get("/api/openapi.json", swaggerspec.ServePublicJSON)
	r.Get("/api/openapi.yaml", swaggerspec.ServePublicYAML)
	r.Get("/api/v1/openapi.json", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/api/openapi.json", http.StatusMovedPermanently)
	})
	r.Get("/api/v1/openapi.yaml", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/api/openapi.yaml", http.StatusMovedPermanently)
	})

	// Root-path aliases for codegen tools that probe /openapi.{json,yaml}.
	// Registered before any SPA catchall so the redirect wins.
	r.Get("/openapi.json", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/api/openapi.json", http.StatusMovedPermanently)
	})
	r.Get("/openapi.yaml", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/api/openapi.yaml", http.StatusMovedPermanently)
	})

	healthHandler.RegisterRoutes(r)

	// Auth handler registers POST endpoints (signup, login, …) plus
	// GET /api/v1/auth/invitation-info. ContentType is only consulted on
	// POST/PUT/PATCH, so wrapping the whole registration with it leaves
	// the GET unaffected while enforcing CT on the auth writes.
	r.Group(func(r chi.Router) {
		r.Use(middleware.ContentType)
		authHandler.RegisterRoutes(r, middleware.Auth)
	})

	// TRA-947: build the entitlement gate once; thread it into the handlers
	// that register paid mutations inside the internal session group so we can
	// gate exactly those routes (this group also carries must-stay-open writes:
	// org/user/api-key mgmt, current-org switch, output test/reset).
	paidGate := middleware.SubscriptionRequired(store)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.SentryContext)
		r.Use(middleware.ContentType)

		orgsHandler.RegisterRoutes(r, store)
		orgsHandler.RegisterMeRoutes(r)
		usersHandler.RegisterRoutes(r)
		assetsHandler.RegisterRoutes(r, paidGate)
		inventoryHandler.RegisterRoutes(r)
		reportsHandler.RegisterRoutes(r)
		// Internal-only scan device/point management (not public API).
		scanDevicesHandler.RegisterRoutes(r, paidGate)
		scanPointsHandler.RegisterRoutes(r, paidGate)
		// Internal-only output device management (not public API).
		outputDevicesHandler.RegisterRoutes(r, paidGate)
		// TRA-993: internal-only per-antenna power tuning (not public API).
		antennaPowerHandler.RegisterRoutes(r, paidGate)
		lookupHandler.RegisterRoutes(r)
		// TRA-924: org-enforced Live Reads SSE stream (session-auth, internal).
		readstreamHandler.RegisterRoutes(r)
		// TRA-978: internal mustering POC surface (SSE + REST + simulate/seed).
		// Session-auth only, NOT in the public OpenAPI spec (no paidGate).
		musteringHandler.RegisterRoutes(r)

		r.Get("/swagger/openapi.internal.json", swaggerspec.ServeJSON)
		r.Get("/swagger/openapi.internal.yaml", swaggerspec.ServeYAML)
		r.Get("/swagger/*", httpSwagger.Handler(
			httpSwagger.URL("/swagger/openapi.internal.json"),
		))
	})

	// TRA-677/TRA-861: the test-handler-minted schemathesis key bypasses rate
	// limiting only in dev/test/preview envs (fail-closed; see env_gate.go).
	// Same gate as the test-handler mount below, so production (APP_ENV="prod")
	// cannot route a schemathesis-mint key into a bypass even if a key with that
	// name leaked into the prod DB.
	allowTestRateLimitBypass := testAffordancesAllowed(os.Getenv("APP_ENV"))

	// Public API — API-key auth (TRA-393 canary)
	r.With(
		middleware.DefaultRateLimitHeaders(rl),
		middleware.APIKeyAuth(store),
		middleware.RateLimit(rl, allowTestRateLimitBypass),
		middleware.RejectQueryParams(),
	).Get("/api/v1/orgs/me", orgsHandler.GetOrgMe)

	// TRA-466 API-key management — accepts session admin OR api-key with keys:admin scope.
	// Lives outside the session-only orgs subtree so api-key JWTs are accepted.
	r.Group(func(r chi.Router) {
		r.Use(middleware.DefaultRateLimitHeaders(rl))
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.RateLimit(rl, allowTestRateLimitBypass))
		r.Use(middleware.SentryContext)
		r.Use(middleware.ContentType)
		orgsHandler.RegisterAPIKeyRoutes(r, store)
	})

	// TRA-396 public read surface — accepts API-key OR session auth via EitherAuth.
	r.Group(func(r chi.Router) {
		r.Use(middleware.DefaultRateLimitHeaders(rl))
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.RateLimit(rl, allowTestRateLimitBypass))
		r.Use(middleware.SentryContext)

		r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets", assetsHandler.ListAssets)
		r.With(middleware.RequireScope("assets:read"), middleware.RejectQueryParams()).Get("/api/v1/assets/{asset_id}", assetsHandler.GetAsset)

		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations", locationsHandler.ListLocations)
		r.With(middleware.RequireScope("locations:read"), middleware.RejectQueryParams()).Get("/api/v1/locations/{location_id}", locationsHandler.GetLocation)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{location_id}/ancestors", locationsHandler.GetAncestors)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{location_id}/children", locationsHandler.GetChildren)
		r.With(middleware.RequireScope("locations:read")).Get("/api/v1/locations/{location_id}/descendants", locationsHandler.GetDescendants)

		// tracking:read gates both the asset movement history (time-series)
		// and the current-locations snapshot. The shared scope models the
		// "where things are and have been" surface: an integrator scoping a
		// key for live tracking gets both forms of locate-the-asset read.
		r.With(middleware.RequireScope("tracking:read")).Get("/api/v1/reports/asset-locations", reportsHandler.ListCurrentLocations)
		r.With(middleware.RequireScope("tracking:read")).Get("/api/v1/assets/{asset_id}/history", reportsHandler.GetAssetHistory)
	})

	// TRA-397 public write surface — accepts API-key OR session auth via EitherAuth.
	// Every route is audited via WriteAudit and gated by a per-resource write scope.
	// WriteAudit is deliberately positioned before RateLimit so 429 denials are
	// captured in the audit log too (the recorder sees whatever status downstream
	// middleware writes).
	r.Group(func(r chi.Router) {
		r.Use(middleware.DefaultRateLimitHeaders(rl))
		r.Use(middleware.EitherAuth(store))
		r.Use(middleware.WriteAudit)
		r.Use(middleware.SubscriptionRequired(store)) // TRA-947: 402 on not-entitled paid mutation
		r.Use(middleware.RateLimit(rl, allowTestRateLimitBypass))
		r.Use(middleware.SentryContext)
		r.Use(middleware.ContentType)

		// Assets
		r.With(middleware.RequireScope("assets:write"), middleware.RejectQueryParams()).Post("/api/v1/assets", assetsHandler.Create)
		r.With(middleware.RequireScope("assets:write"), middleware.RequireMergePatchCT, middleware.RejectQueryParams()).Patch("/api/v1/assets/{asset_id}", assetsHandler.Update)
		r.With(middleware.RequireScope("assets:write"), middleware.RejectQueryParams()).Delete("/api/v1/assets/{asset_id}", assetsHandler.Delete)
		r.With(middleware.RequireScope("assets:write"), middleware.RejectQueryParams()).Post("/api/v1/assets/{asset_id}/rename", assetsHandler.Rename)
		r.With(middleware.RequireScope("assets:write"), middleware.RejectQueryParams()).Post("/api/v1/assets/{asset_id}/tags", assetsHandler.AddTag)
		r.With(middleware.RequireScope("assets:write"), middleware.RejectQueryParams()).Delete("/api/v1/assets/{asset_id}/tags/{tag_id}", assetsHandler.RemoveTag)

		// Locations
		r.With(middleware.RequireScope("locations:write"), middleware.RejectQueryParams()).Post("/api/v1/locations", locationsHandler.Create)
		r.With(middleware.RequireScope("locations:write"), middleware.RequireMergePatchCT, middleware.RejectQueryParams()).Patch("/api/v1/locations/{location_id}", locationsHandler.Update)
		r.With(middleware.RequireScope("locations:write"), middleware.RejectQueryParams()).Delete("/api/v1/locations/{location_id}", locationsHandler.Delete)
		r.With(middleware.RequireScope("locations:write"), middleware.RejectQueryParams()).Post("/api/v1/locations/{location_id}/rename", locationsHandler.Rename)
		r.With(middleware.RequireScope("locations:write"), middleware.RejectQueryParams()).Post("/api/v1/locations/{location_id}/tags", locationsHandler.AddTag)
		r.With(middleware.RequireScope("locations:write"), middleware.RejectQueryParams()).Delete("/api/v1/locations/{location_id}/tags/{tag_id}", locationsHandler.RemoveTag)

		// Inventory (scan writes)
		r.With(middleware.RequireScope("scans:write"), middleware.RejectQueryParams()).Post("/api/v1/inventory/save", inventoryHandler.Save)
	})

	// TRA-555 / TRA-554: Internal /by-id/ families removed. Public
	// /api/v1/{assets,locations}/{id} routes already accept session JWT via
	// EitherAuth, so frontend session-auth flows hit canonical routes directly.

	// TRA-600 routing-precedence guards. Every static path on this list
	// competes with a sibling /{id} parameter route at the same level. Without
	// these registrations chi v5 falls through to the sibling for the methods
	// it doesn't see registered on the static path, surfacing a 400
	// "invalid id" or a 405 with a misleading Allow header. Registered on the
	// top-level mux (no auth) so the 405/404 fires regardless of auth state —
	// chi merges per-method handlers across groups into the same path node.
	//
	// Wrapped with DefaultRateLimitHeaders so the static-vs-{id} guard
	// responses carry X-RateLimit-* headers, matching the /api/v1/* contract
	// from TRA-518.
	r.Group(func(r chi.Router) {
		r.Use(middleware.DefaultRateLimitHeaders(rl))

		// Retired endpoints (TRA-600 Option 3): natural-key lookup moved to
		// `?external_key=` filter on the collection. Old paths return 404.
		register404Static(r, "/api/v1/assets/lookup",
			"This endpoint has been removed. Use GET /api/v1/assets?external_key= to find an asset by external_key.")
		register404Static(r, "/api/v1/locations/lookup",
			"This endpoint has been removed. Use GET /api/v1/locations?external_key= to find a location by external_key.")
		// TRA-658 BB25: path moved out of /locations/ — schema is in
		// report.* and scope is tracking:read, so /reports/ is the
		// correct namespace. Without this guard chi falls through to
		// /api/v1/locations/{location_id} and surfaces 401 (auth runs
		// before the sibling resolves to "current is not a valid id").
		register404Static(r, "/api/v1/locations/current",
			"This endpoint has moved. Use GET /api/v1/reports/asset-locations.")

		// Live static endpoints with a single supported method.
		register405Static(r, "/api/v1/orgs/me", []string{http.MethodGet})
		register405Static(r, "/api/v1/users/me", []string{http.MethodGet})
		register405Static(r, "/api/v1/users/me/current-org", []string{http.MethodPost})
		register405Static(r, "/api/v1/reports/asset-locations", []string{http.MethodGet})
		register405Static(r, "/api/v1/assets/bulk", []string{http.MethodPost})
		register405Static(r, "/api/v1/assets/bulk/{jobId}", []string{http.MethodGet})
	})

	if testAffordancesAllowed(os.Getenv("APP_ENV")) {
		// Test handler registers /test/* with POST + GETs. ContentType is
		// method-gated, so wrapping leaves GETs untouched and enforces CT
		// on POST /test/apikeys. Fail-closed gate (env_gate.go): never mounts
		// on prod (APP_ENV="prod") or any unrecognized env.
		r.Group(func(r chi.Router) {
			r.Use(middleware.ContentType)
			testHandler.RegisterRoutes(r)
		})
	}

	// JSON 404/405 for unknown /api/* paths so clients don't blow up mid-deserialize
	// on the SPA's index.html fallback. Must be registered before the /* wildcard.
	//
	// Registered per-method (GET, POST, PUT, PATCH, DELETE, OPTIONS) rather than
	// via HandleFunc so HEAD is NOT registered on the catchall — chimiddleware.GetHead
	// rewrites HEAD→GET only when no HEAD handler matches, and a method-agnostic
	// HandleFunc would register HEAD on /api/* and short-circuit the rewrite for
	// every /api/v1/* HEAD probe.
	//
	// The handler distinguishes (TRA-605):
	//   - other methods are registered for this path → 405 with real Allow
	//   - no methods are registered for this path → 404 unknown-route
	// computeAllowedMethods filters out the /api/* pattern so probes for the
	// "real" Allow set don't see the catchall as a phantom GET handler.
	//
	// TRA-518: wrapped with DefaultRateLimitHeaders so responses under /api/v1/*
	// carry rate-limit headers too.
	apiCatchall := func(w http.ResponseWriter, req *http.Request) {
		allowed := computeAllowedMethods(r, req.URL.Path)
		if len(allowed) > 0 {
			httputil.Respond405(w, req, allowed, middleware.GetRequestID(req.Context()))
			return
		}
		httputil.Respond404(w, req, "Unknown API route: "+req.URL.Path, middleware.GetRequestID(req.Context()))
	}
	for _, m := range []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
	} {
		r.With(middleware.DefaultRateLimitHeaders(rl)).MethodFunc(m, "/api/*", apiCatchall)
	}

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		frontendHandler.ServeSPA(w, r, "frontend/dist/index.html")
	})

	return r
}
