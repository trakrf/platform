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
}

type Handler struct {
	db        *pgxpool.Pool
	info      buildinfo.Info
	startTime time.Time
}

func NewHandler(db *pgxpool.Pool, info buildinfo.Info, startTime time.Time) *Handler {
	return &Handler{
		db:        db,
		info:      info,
		startTime: startTime,
	}
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

	resp := Response{
		Status:    "ok",
		Version:   h.info.Version,
		Commit:    h.info.Commit,
		Tag:       h.info.Tag,
		BuildTime: h.info.BuildTime,
		GoVersion: h.info.GoVersion,
		Timestamp: time.Now().UTC(),
		Uptime:    uptime.String(),
		Database:  dbStatus,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/healthz", h.Healthz)
	r.Get("/readyz", h.Readyz)
	r.Get("/health", h.Health)
}
