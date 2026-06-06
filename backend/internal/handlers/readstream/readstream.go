// Package readstream serves the org-scoped Live Reads SSE endpoint (TRA-924).
// It enforces org context with the same JWT machinery as the REST API, so a
// caller only ever streams its own org's reads — and the browser holds no broker
// credentials.
package readstream

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/trakrf/platform/backend/internal/middleware"
	rs "github.com/trakrf/platform/backend/internal/services/readstream"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// heartbeatInterval keeps idle SSE connections alive through proxies and well
// inside the server IdleTimeout.
const heartbeatInterval = 20 * time.Second

// Handler streams org-filtered live presence deltas over SSE.
type Handler struct {
	tracker *rs.Tracker
}

// NewHandler builds the SSE handler over the shared presence tracker.
func NewHandler(t *rs.Tracker) *Handler { return &Handler{tracker: t} }

// RegisterRoutes mounts the SSE endpoint. The caller must apply session auth
// (the route lives in the authenticated group).
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/reads/stream", h.Stream)
}

// Stream holds an SSE connection open, forwarding the caller's org reads until
// the client disconnects.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	// Capability probe: the outermost writer in the chain must support flushing
	// (it does in prod via the sentry fancy-writer; the logger wrapper is made
	// transparent so the flush delegates cleanly).
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// rc.Flush / rc.SetWriteDeadline walk the Unwrap chain, so they reach the
	// real connection through the middleware wrappers.
	rc := http.NewResponseController(w)
	// Long-lived stream: clear the server's per-request WriteTimeout for this
	// connection (otherwise it dies after WriteTimeout). Best effort.
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx/proxy response buffering
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	_ = rc.Flush()

	ch, cancel := h.tracker.Subscribe(orgID)
	defer cancel()

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
			// Named SSE event (snapshot|enter|update|leave) so the client reducer
			// can dispatch without sniffing the payload.
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, ev.Data); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}
