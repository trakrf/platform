package main

import (
	"encoding/json"
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
// Phase 2A: Simple check (no dependencies yet)
// Phase 3: Will add db.Ping() check
func readyzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO Phase 3: Add database connectivity check
	// if err := db.Ping(r.Context()); err != nil {
	//     w.WriteHeader(http.StatusServiceUnavailable)
	//     w.Write([]byte("database unavailable"))
	//     return
	// }

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// HealthResponse - JSON response for /health endpoint
type HealthResponse struct {
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	// Phase 3 additions:
	// Database string `json:"database,omitempty"`
	// Uptime   string `json:"uptime,omitempty"`
}

// healthHandler - Human-friendly health check with details
// Returns JSON with status, version, timestamp
// Used by humans, dashboards, monitoring tools
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := HealthResponse{
		Status:    "ok",
		Version:   version,
		Timestamp: time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
