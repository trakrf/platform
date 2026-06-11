// Package mustering provides the internal (session-authenticated) mustering POC
// endpoints (TRA-978): the SSE presence/event stream, the REST lifecycle
// surface, and the simulator/seed demo controls. None of these are part of the
// public API — they mount in the session-auth group of router.go, same as
// scan-device CRUD and the Live Reads stream, and carry no swagger ,public tag.
package mustering

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/trakrf/platform/backend/internal/middleware"
	mustering "github.com/trakrf/platform/backend/internal/mustering"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// heartbeatInterval keeps idle SSE connections alive through proxies and well
// inside the server IdleTimeout (mirrors the readstream handler's 20s).
const heartbeatInterval = 20 * time.Second

// Stream holds an SSE connection open, sending a snapshot on connect then
// engine deltas + heartbeats until the client disconnects.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	rc := http.NewResponseController(w)
	// Long-lived stream: clear the server's per-request WriteTimeout (otherwise
	// the connection dies after WriteTimeout). Best effort.
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	_ = rc.Flush()

	// Register for deltas BEFORE computing the snapshot so a transition that
	// races the snapshot is delivered (idempotent; client reconciles).
	ch, cancel := h.broadcaster.Subscribe(orgID)
	defer cancel()

	// Snapshot on connect.
	if snap, err := h.engine.Status(r.Context(), orgID); err == nil {
		if data, mErr := json.Marshal(snap); mErr == nil {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", mustering.EventSnapshot, data)
			_ = rc.Flush()
		}
	}

	hb := time.NewTicker(heartbeatInterval)
	defer hb.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hb.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, ev.Data); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}

// RegisterRoutes mounts the mustering routes. Caller applies session auth (the
// route lives in the authenticated group of router.go).
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/mustering/stream", h.Stream)
	r.Get("/api/v1/mustering/status", h.GetStatus)
	r.Post("/api/v1/mustering/events", h.CreateEvent)
	r.Get("/api/v1/mustering/events", h.ListEvents)
	r.Get("/api/v1/mustering/events/{id}", h.GetEvent)
	r.Post("/api/v1/mustering/events/{id}/all-clear", h.AllClear)
	r.Post("/api/v1/mustering/events/{id}/cancel", h.Cancel)
	r.Post("/api/v1/mustering/events/{id}/unlock", h.Unlock)
	r.Patch("/api/v1/mustering/events/{id}/entries/{entryId}", h.PatchEntry)
	r.Post("/api/v1/mustering/simulate", h.Simulate)
	r.Post("/api/v1/mustering/seed", h.Seed)
	// TRA-978 phase 7: optional static floor plan (image + pins on Locations).
	r.Get("/api/v1/mustering/floor-plan", h.GetFloorPlan)
	r.Put("/api/v1/mustering/floor-plan", h.PutFloorPlan)
}
