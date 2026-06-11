package mustering

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/muster"
	"github.com/trakrf/platform/backend/internal/models/scanread"
	mustering "github.com/trakrf/platform/backend/internal/mustering"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// Handler serves the internal mustering POC surface (TRA-978): SSE stream, REST
// lifecycle, and the simulator/seed demo controls. It holds the engine (lifecycle
// + report), the SSE broadcaster, storage (for list/get + seed/simulate), and the
// ingest evaluator + readstream feed that the simulator drives so a synthetic
// read is indistinguishable from a hardware read.
//
// engine + broadcaster are interfaces so the REST/SSE handlers are unit-testable
// with fakes (*mustering.Engine and *mustering.Broadcaster satisfy them);
// simulate/seed need the concrete *storage.Storage and are covered by
// integration tests.
type Handler struct {
	engine      musterEngine
	broadcaster musterBroadcaster
	store       *storage.Storage
	evaluator   readEvaluator
	feed        readPublisher
}

// musterEngine is the engine surface the REST + SSE handlers call.
// *mustering.Engine satisfies it.
type musterEngine interface {
	Status(ctx context.Context, orgID int) (mustering.SnapshotPayload, error)
	Activate(ctx context.Context, orgID, userID, windowMinutes int) (*muster.Event, error)
	AllClear(ctx context.Context, orgID, eventID, userID int) (*muster.Event, error)
	Cancel(ctx context.Context, orgID, eventID, userID int) (*muster.Event, error)
	Verify(ctx context.Context, orgID, eventID, entryID, userID int) (*muster.Entry, *muster.Counts, error)
	MarkSafe(ctx context.Context, orgID, eventID, entryID, userID int, note string) (*muster.Entry, *muster.Counts, error)
	Unlock(ctx context.Context, orgID, eventID, userID int, email string) error
}

// musterBroadcaster is the SSE registry surface the stream handler needs.
// *mustering.Broadcaster satisfies it.
type musterBroadcaster interface {
	Subscribe(orgID int) (<-chan mustering.Event, func())
}

// readEvaluator mirrors ingest.ReadEvaluator (the post-membership fan-out). The
// simulator hands its synthetic resolved reads here so the muster + geofence
// engines react exactly as they do for hardware. Declared locally to avoid an
// ingest import cycle and to keep it fakeable in tests.
type readEvaluator interface {
	Evaluate(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, reads []storage.ResolvedRead)
}

// readPublisher mirrors ingest.ReadPublisher (the pre-membership Live Reads
// feed). The simulator publishes here so Locate's RSSI indicator works in
// pure-simulator demos. Optional; nil disables the live-feed fan-out.
type readPublisher interface {
	Publish(orgID int, topic string, reads []scanread.Read)
}

// NewHandler builds the mustering handler.
func NewHandler(engine *mustering.Engine, broadcaster *mustering.Broadcaster, store *storage.Storage, evaluator readEvaluator, feed readPublisher) *Handler {
	return &Handler{engine: engine, broadcaster: broadcaster, store: store, evaluator: evaluator, feed: feed}
}

// newHandlerForTest builds a handler over fakes for the REST/SSE unit tests
// (simulate/seed need the concrete store and are integration-tested separately).
func newHandlerForTest(engine musterEngine, broadcaster musterBroadcaster) *Handler {
	return &Handler{engine: engine, broadcaster: broadcaster}
}

// userClaims returns (orgID, userID, email) from the session context, or writes
// the missing-org-context error and returns ok=false.
func (h *Handler) userClaims(w http.ResponseWriter, r *http.Request, reqID string) (orgID, userID int, email string, ok bool) {
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return 0, 0, "", false
	}
	if c := middleware.GetUserClaims(r); c != nil {
		userID = c.UserID
		email = c.Email
	}
	return orgID, userID, email, true
}

// GetStatus returns presence + the active event (same shape as the SSE snapshot).
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, _, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	snap, err := h.engine.Status(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, snap)
}

// createEventRequest is the activate body.
type createEventRequest struct {
	WindowMinutes int `json:"window_minutes"`
}

// CreateEvent activates a muster drill. 409 if one is already active.
func (h *Handler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, userID, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	var req createEventRequest
	// Body is optional; tolerate empty/whitespace.
	if r.ContentLength != 0 {
		if err := httputil.DecodeJSONStrict(r, &req); err != nil {
			httputil.RespondDecodeError(w, r, err, reqID)
			return
		}
	}
	ev, err := h.engine.Activate(r.Context(), orgID, userID, req.WindowMinutes)
	if err != nil {
		if errors.As(err, &muster.ErrActiveEventExists{}) {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict, err.Error(), reqID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": ev})
}

// ListEvents lists past + active events (headers + counts, no entries).
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, _, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	events, err := h.store.ListMusterEvents(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if events == nil {
		events = []muster.Event{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": events})
}

// GetEvent returns one event with entries + counts (+ report when completed).
func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, _, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	ev, err := h.store.GetMusterEvent(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if ev == nil {
		httputil.Respond404(w, r, "muster event not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": ev})
}

// AllClear completes the event and returns it with the computed report.
func (h *Handler) AllClear(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, userID, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	ev, err := h.engine.AllClear(r.Context(), orgID, id, userID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if ev == nil {
		httputil.Respond404(w, r, "muster event not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": ev})
}

// Cancel ends the event without a report.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, userID, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	ev, err := h.engine.Cancel(r.Context(), orgID, id, userID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if ev == nil {
		httputil.Respond404(w, r, "muster event not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": ev})
}

// Unlock logs a break-glass reveal against the event metadata.
func (h *Handler) Unlock(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, userID, email, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	// Confirm the event exists for this org (RLS naturally 404s cross-org).
	ev, err := h.store.GetMusterEvent(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if ev == nil {
		httputil.Respond404(w, r, "muster event not found", reqID)
		return
	}
	if err := h.engine.Unlock(r.Context(), orgID, id, userID, email); err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"unlocked": true}})
}

// patchEntryRequest is the verify / mark_safe body.
type patchEntryRequest struct {
	Action string `json:"action"` // verify | mark_safe
	Note   string `json:"note,omitempty"`
}

// PatchEntry applies a verify / mark_safe transition to one entry. 409 on an
// invalid transition; 404 on a missing entry / wrong org.
func (h *Handler) PatchEntry(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, userID, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	eventID, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	entryID, err := httputil.ParseSurrogateID("entryId", chi.URLParam(r, "entryId"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	var req patchEntryRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}

	var entry *muster.Entry
	var counts *muster.Counts
	switch req.Action {
	case "verify":
		entry, counts, err = h.engine.Verify(r.Context(), orgID, eventID, entryID, userID)
	case "mark_safe":
		entry, counts, err = h.engine.MarkSafe(r.Context(), orgID, eventID, entryID, userID, req.Note)
	default:
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, "action must be \"verify\" or \"mark_safe\"", reqID)
		return
	}
	if err != nil {
		if errors.As(err, &muster.ErrInvalidTransition{}) {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict, err.Error(), reqID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if entry == nil {
		httputil.Respond404(w, r, "muster entry not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"entry": entry, "counts": counts}})
}
