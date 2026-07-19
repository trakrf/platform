// Package kits provides internal (session-authenticated) endpoints for
// expected-together asset groups (TRA-1032): commission a kit, verify a dock
// scan against kit membership, and look kits up by label or member EPC.
// NOT part of the public API (no ,public swagger tag); wire shapes are frozen
// against the sibling frontend ticket.
package kits

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/kit"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	httputil.RegisterCustomValidations(v)
	return v
}()

// KitStorage is the narrow storage surface the handler needs (mockable).
type KitStorage interface {
	CommissionKit(ctx context.Context, orgID int, req kit.CommissionRequest) (*kit.Kit, error)
	VerifyKits(ctx context.Context, orgID int, epcs []string) (*kit.VerifyResponse, error)
	ListKits(ctx context.Context, orgID int, query, memberEPC string) ([]kit.KitSummary, error)
	GetKitByID(ctx context.Context, orgID, kitID int) (*kit.Kit, error)
}

type Handler struct {
	storage KitStorage
}

func NewHandler(storage KitStorage) *Handler {
	return &Handler{storage: storage}
}

// RegisterRoutes wires the kit routes onto r. Mount inside the session-auth
// (middleware.Auth) group. Writes are paid mutations (TRA-947) and require
// Operator+ (scan-save precedent); reads stay open to any org member.
func (h *Handler) RegisterRoutes(r chi.Router, paidGate, operatorGate func(http.Handler) http.Handler) {
	r.Get("/api/v1/kits", h.List)
	r.Get("/api/v1/kits/{kit_id}", h.Get)
	r.With(paidGate, operatorGate).Post("/api/v1/kits", h.Create)
	r.With(paidGate, operatorGate).Post("/api/v1/kits/verify", h.Verify)
}

// @Summary  Commission a kit
// @Tags     kits,internal
// @ID       kits.create
// @Accept   json
// @Produce  json
// @Param    request body kit.CommissionRequest true "Kit label + members (>=2). Unknown EPCs auto-create minimal assets."
// @Success  201 {object} kit.KitResponse
// @Failure  409 {object} httputil.ErrorResponse "A member is already an active member of another active kit"
// @Router   /api/v1/kits [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	var req kit.CommissionRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	created, err := h.storage.CommissionKit(r.Context(), orgID, req)
	if err != nil {
		writeKitError(w, r, err, reqID)
		return
	}
	w.Header().Set("Location", "/api/v1/kits/"+strconv.Itoa(created.ID))
	httputil.WriteJSON(w, http.StatusCreated, kit.KitResponse{Data: *created})
}

// @Summary  Verify a scan session against kit membership (dock check)
// @Tags     kits,internal
// @ID       kits.verify
// @Accept   json
// @Produce  json
// @Param    request body kit.VerifyRequest true "Scanned asset-tag EPCs"
// @Success  200 {object} kit.VerifyResponse
// @Router   /api/v1/kits/verify [post]
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	var req kit.VerifyRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	resp, err := h.storage.VerifyKits(r.Context(), orgID, req.EPCs)
	if err != nil {
		writeKitError(w, r, err, reqID)
		return
	}
	// Frozen contract: top-level shape, no {data} envelope (TRA-1032).
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// @Summary  List kits
// @Tags     kits,internal
// @ID       kits.list
// @Produce  json
// @Param    query query string false "Label substring filter"
// @Param    member_epc query string false "Return kits with an active member matching this EPC (normalized)"
// @Success  200 {object} kit.KitListResponse
// @Router   /api/v1/kits [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	kits, err := h.storage.ListKits(r.Context(), orgID,
		r.URL.Query().Get("query"), r.URL.Query().Get("member_epc"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, kit.KitListResponse{Data: kits})
}

// @Summary  Get a kit (members + latest verification)
// @Tags     kits,internal
// @ID       kits.get
// @Produce  json
// @Param    kit_id path int true "Kit id"
// @Success  200 {object} kit.KitResponse
// @Router   /api/v1/kits/{kit_id} [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("kit_id", chi.URLParam(r, "kit_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	k, err := h.storage.GetKitByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if k == nil {
		httputil.Respond404(w, r, "kit not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, kit.KitResponse{Data: *k})
}

// writeKitError maps the typed storage errors: membership conflict → 409
// (detail names the owning kit label), storage-detected validation → 400,
// everything else → 500.
func writeKitError(w http.ResponseWriter, r *http.Request, err error, reqID string) {
	var conflict *kit.ConflictError
	if errors.As(err, &conflict) {
		httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict, conflict.Error(), reqID)
		return
	}
	var validation *kit.ValidationError
	if errors.As(err, &validation) {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, validation.Error(), reqID)
		return
	}
	httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
}
