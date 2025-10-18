package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// healthzHandler - K8s liveness probe
// Returns 200 if process is alive
// K8s will restart pod if this fails
func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// readyzHandler - K8s readiness probe
// Returns 200 if ready to serve traffic
// K8s will remove from service if this fails
func readyzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Database connectivity check
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		slog.Error("Readiness check failed", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("database unavailable"))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// HealthResponse - JSON response for /health endpoint
type HealthResponse struct {
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`
	Database  string    `json:"database"`
}

// healthHandler - Human-friendly health check with details
// Returns JSON with status, version, timestamp, uptime
// Used by humans, dashboards, monitoring tools
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uptime := time.Since(startTime).Round(time.Second)

	// Check database connectivity
	dbStatus := "connected"
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := db.Ping(ctx); err != nil {
		dbStatus = "unavailable"
	}

	resp := HealthResponse{
		Status:    "ok",
		Version:   version,
		Timestamp: time.Now().UTC(),
		Uptime:    uptime.String(),
		Database:  dbStatus,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
