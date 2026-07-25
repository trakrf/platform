// Package webhooks provides internal (session-authenticated) CRUD handlers for
// the org's webhook subscription plus a synchronous test-fire action (TRA-1043).
//
// These are management-surface endpoints — no ,public swagger tag and no
// RequireScope — because registering a webhook is an org-settings action, not
// something an integrator's API key does on the customer's behalf. The event
// PAYLOAD is the customer-facing contract; this CRUD is not.
//
// Webhooks is base platform surface, NOT a sold capability: no capabilities row
// and no RequireCap, deliberately and permanently. Capabilities are vertical
// use-case modules (geofence, mustering, inventory); horizontal platform surface
// — the datastore, the REST API, webhooks, reports — is included for every
// paying customer. Do not add a gate here later "for consistency"; it would
// contradict the positioning.
package webhooks

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	webhookmodel "github.com/trakrf/platform/backend/internal/models/webhook"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/webhook"
)

var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	httputil.RegisterCustomValidations(v)
	return v
}()

// Handler serves webhook CRUD plus the test fire.
type Handler struct {
	storage *storage.Storage
	client  *webhook.Client
}

// NewHandler builds the handler. client is the same delivery client the sink
// uses, so a test fire exercises exactly the guard, signing, and timeout the
// real path does.
func NewHandler(storage *storage.Storage, client *webhook.Client) *Handler {
	return &Handler{storage: storage, client: client}
}

// RegisterRoutes wires webhook routes onto r. Mount inside the session-auth
// (middleware.Auth) group.
//
// paidGate is middleware.SubscriptionRequired: it gates mutations only, so a
// lapsed org can still SEE its configuration but cannot change it (TRA-946 keeps
// reads open deliberately). Delivery is gated separately inside the sink,
// because an outbound POST is not an inbound request and no middleware sees it.
//
// adminGate must be RequireCurrentOrgRole(store, RoleAdmin), not RequireOrgAdmin:
// these routes carry no :orgId path param, so the param-based variant would 400
// (TRA-1033).
func (h *Handler) RegisterRoutes(r chi.Router, paidGate, adminGate func(http.Handler) http.Handler) {
	r.With(adminGate).Get("/api/v1/webhooks", h.List)
	r.With(adminGate, paidGate).Post("/api/v1/webhooks", h.Create)
	r.With(adminGate).Get("/api/v1/webhooks/{webhook_id}", h.Get)
	r.With(adminGate, paidGate).Patch("/api/v1/webhooks/{webhook_id}", h.Update)
	r.With(adminGate, paidGate).Delete("/api/v1/webhooks/{webhook_id}", h.Delete)
	r.With(adminGate, paidGate).Post("/api/v1/webhooks/{webhook_id}/test", h.Test)
}

// @Summary  List webhooks
// @Description Returns the organization's webhook, or an empty list. The secret is masked; it is returned in cleartext only in the create response.
// @Tags     webhooks,internal
// @ID       webhooks.list
// @Produce  json
// @Success  200 {object} webhook.ListResponse
// @Router   /api/v1/webhooks [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	wh, err := h.storage.GetWebhook(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	// Zero or one element. The collection shape is kept so growing to N
	// subscriptions (TRA-398 Phase 2) is not a breaking response change.
	data := []webhookmodel.Webhook{}
	if wh != nil {
		data = append(data, wh.Masked())
	}
	httputil.WriteJSON(w, http.StatusOK, webhookmodel.ListResponse{Data: data})
}

// @Summary  Get a webhook
// @Tags     webhooks,internal
// @ID       webhooks.get
// @Produce  json
// @Param    webhook_id path int true "Webhook id"
// @Success  200 {object} webhook.Response
// @Router   /api/v1/webhooks/{webhook_id} [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, ok := h.orgID(w, r, reqID)
	if !ok {
		return
	}
	id, ok := h.pathID(w, r, reqID)
	if !ok {
		return
	}
	wh, err := h.storage.GetWebhookByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if wh == nil {
		httputil.Respond404(w, r, "Webhook not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, webhookmodel.Response{Data: wh.Masked()})
}

// @Summary  Create a webhook
// @Description Registers the organization's webhook endpoint. The signing secret is generated server-side and returned in cleartext in this response ONLY; every later response masks it. One webhook per organization.
// @Tags     webhooks,internal
// @ID       webhooks.create
// @Accept   json
// @Produce  json
// @Param    request body webhook.CreateRequest true "Webhook"
// @Success  201 {object} webhook.Response
// @Failure  409 {object} modelerrors.ErrorResponse "Organization already has a webhook"
// @Router   /api/v1/webhooks [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, ok := h.orgID(w, r, reqID)
	if !ok {
		return
	}

	var req webhookmodel.CreateRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	if err := h.client.ValidateTargetURL(req.URL); err != nil {
		httputil.WriteValidationError(w, r, reqID, []modelerrors.FieldError{{
			Field: "url", Code: "invalid_value", Message: err.Error(),
		}})
		return
	}

	secret, err := webhookmodel.GenerateSecret()
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	wh, err := h.storage.CreateWebhook(r.Context(), orgID, req.URL, secret, enabled)
	if err != nil {
		if errors.Is(err, storage.ErrWebhookExists) {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				"This organization already has a webhook. Update or delete it first.", reqID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}

	w.Header().Set("Location", "/api/v1/webhooks/"+strconv.Itoa(wh.ID))
	// The one and only time the cleartext secret leaves the system.
	httputil.WriteJSON(w, http.StatusCreated, webhookmodel.Response{Data: *wh})
}

// @Summary  Update a webhook
// @Tags     webhooks,internal
// @ID       webhooks.update
// @Accept   json
// @Produce  json
// @Param    webhook_id path int true "Webhook id"
// @Param    request body webhook.UpdateRequest true "Fields to update"
// @Success  200 {object} webhook.Response
// @Router   /api/v1/webhooks/{webhook_id} [patch]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, ok := h.orgID(w, r, reqID)
	if !ok {
		return
	}
	id, ok := h.pathID(w, r, reqID)
	if !ok {
		return
	}

	var req webhookmodel.UpdateRequest
	// `secret` is not a field on UpdateRequest, so a strict decode rejects an
	// attempt to set one: rotation is TRA-398 Phase 2, and silently ignoring it
	// would leave the caller believing their secret changed.
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	if req.URL != nil {
		if err := h.client.ValidateTargetURL(*req.URL); err != nil {
			httputil.WriteValidationError(w, r, reqID, []modelerrors.FieldError{{
				Field: "url", Code: "invalid_value", Message: err.Error(),
			}})
			return
		}
	}

	wh, err := h.storage.UpdateWebhook(r.Context(), orgID, id, req)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if wh == nil {
		httputil.Respond404(w, r, "Webhook not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, webhookmodel.Response{Data: wh.Masked()})
}

// @Summary  Delete a webhook
// @Tags     webhooks,internal
// @ID       webhooks.delete
// @Param    webhook_id path int true "Webhook id"
// @Success  204 "Deleted"
// @Router   /api/v1/webhooks/{webhook_id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, ok := h.orgID(w, r, reqID)
	if !ok {
		return
	}
	id, ok := h.pathID(w, r, reqID)
	if !ok {
		return
	}
	deleted, err := h.storage.DeleteWebhook(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if !deleted {
		httputil.Respond404(w, r, "Webhook not found", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary  Test-fire a webhook
// @Description Delivers a synthetic asset.moved event to the registered endpoint and reports what it answered. Synchronous, so the operator sees the response code immediately.
// @Tags     webhooks,internal
// @ID       webhooks.test
// @Produce  json
// @Param    webhook_id path int true "Webhook id"
// @Success  200 {object} webhook.TestResponse
// @Router   /api/v1/webhooks/{webhook_id}/test [post]
func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, ok := h.orgID(w, r, reqID)
	if !ok {
		return
	}
	id, ok := h.pathID(w, r, reqID)
	if !ok {
		return
	}
	wh, err := h.storage.GetWebhookByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if wh == nil {
		httputil.Respond404(w, r, "Webhook not found", reqID)
		return
	}

	// Entitlement is already enforced: this is a POST, so paidGate answered 402
	// before we got here. A test fire must not become a way to prove an endpoint
	// works while unpaid.
	//
	// A disabled webhook can still be test-fired on purpose — that is how an
	// operator validates an endpoint before switching delivery on.
	status, deliverErr := h.client.Deliver(r.Context(), wh.URL, wh.Secret, webhook.SyntheticEvent(orgID))

	// A failed delivery is a successful diagnostic: 200 with the failure inside
	// the envelope, so the UI can render "your endpoint returned 502" rather
	// than a generic API error.
	result := webhookmodel.TestResult{StatusCode: status}
	if deliverErr != nil {
		result.Error = deliverErr.Error()
	}
	httputil.WriteJSON(w, http.StatusOK, webhookmodel.TestResponse{Data: result})
}

func (h *Handler) orgID(w http.ResponseWriter, r *http.Request, reqID string) (int, bool) {
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return 0, false
	}
	return orgID, true
}

func (h *Handler) pathID(w http.ResponseWriter, r *http.Request, reqID string) (int, bool) {
	id, err := httputil.ParseSurrogateID("webhook_id", chi.URLParam(r, "webhook_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return 0, false
	}
	return id, true
}
