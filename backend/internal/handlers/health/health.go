package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/trakrf/platform/backend/internal/buildinfo"
)

type Response struct {
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Commit    string    `json:"commit"`
	Tag       string    `json:"tag"`
	BuildTime string    `json:"build_time"`
	GoVersion string    `json:"go_version"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`
	Database  string    `json:"database"`
	// SpecRefreshedAt is the canonical "what spec is live" signal called
	// out in the TRA-743 acceptance criteria. The OpenAPI spec is
	// regenerated and embedded in the binary on every build, so this
	// mirrors BuildTime — the distinct json name is the contract BB
	// tooling watches for deploy-lag detection.
	SpecRefreshedAt string `json:"spec_refreshed_at"`
	// Schema compares the migration version the database has applied against
	// the set embedded in this binary (TRA-1190). Omitted when there is no
	// pool or the ledger cannot be read — "unknown" is not "behind".
	Schema *SchemaInfo `json:"schema,omitempty"`
}

type Handler struct {
	db        *pgxpool.Pool
	info      buildinfo.Info
	startTime time.Time
	// readSchema reads the applied migration version. nil disables the check,
	// which is the no-pool unit-test path.
	readSchema SchemaReader
}

func NewHandler(db *pgxpool.Pool, info buildinfo.Info, startTime time.Time) *Handler {
	h := &Handler{
		db:        db,
		info:      info,
		startTime: startTime,
	}
	if db != nil {
		h.readSchema = poolSchemaReader(db)
	}
	return h
}

// Healthz is the liveness probe endpoint. Stays plaintext "ok" — K8s probes
// don't parse bodies and the build metadata lives on /health instead.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// Readyz is the readiness probe endpoint.
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		slog.Error("Readiness check failed", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("database unavailable"))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// @Summary Health check
// @Description Get API health status including deployed build metadata (commit SHA, tag, build time)
// @Tags health,internal
// @Produce json
// @Success 200 {object} health.Response
// @Router /health [get]
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uptime := time.Since(h.startTime).Round(time.Second)

	// db may be nil in unit tests; real servers always pass a live pool.
	dbStatus := "unknown"
	if h.db != nil {
		dbStatus = "connected"
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.db.Ping(ctx); err != nil {
			dbStatus = "unavailable"
		}
	}

	// A schema older than this binary's embedded migration set means the stack
	// is not usable even though every other signal here is green — the state
	// that produced 89 identical e2e failures before anything reported it
	// (TRA-1190). /healthz and /readyz deliberately do not follow this: killing
	// or de-registering the pod cannot fix a schema, and would stop it serving
	// while the migration that does fix it runs.
	schema, status, healthy := h.schemaState(r.Context())

	resp := Response{
		Status:          status,
		Schema:          schema,
		Version:         h.info.Version,
		Commit:          h.info.Commit,
		Tag:             h.info.Tag,
		BuildTime:       h.info.BuildTime,
		GoVersion:       h.info.GoVersion,
		Timestamp:       time.Now().UTC(),
		Uptime:          uptime.String(),
		Database:        dbStatus,
		SpecRefreshedAt: h.info.BuildTime,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/healthz", h.Healthz)
	r.Get("/readyz", h.Readyz)
	r.Get("/health", h.Health)
	// /health.json is the canonical curl-able platform health surface; the
	// dotted extension also makes the route reachable past the SPA catch-all
	// (the same reason /version.json and /manifest.json are registered as
	// explicit routes). BB tooling watches this URL for spec_refreshed_at.
	r.Get("/health.json", h.Health)
}
