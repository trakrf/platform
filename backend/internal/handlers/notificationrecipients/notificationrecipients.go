// Package notificationrecipients serves org-scoped notification contacts and
// the asset subscriptions that link them (TRA-1275). Session-authenticated
// management endpoints, not part of the public API.
package notificationrecipients

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	httputil.RegisterCustomValidations(v)
	return v
}()

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

type dataResponse[T any] struct {
	Data T `json:"data"`
}

// RegisterRoutes mounts inside the session-auth group. Mutations are paid,
// reads are open, matching the other asset-adjacent management surfaces.
// Subscriptions have no DELETE: they are switched off with PATCH is_active
// false, so the record of who was subscribed is kept.
func (h *Handler) RegisterRoutes(r chi.Router, paidGate func(http.Handler) http.Handler) {
	r.Get("/api/v1/notification-recipients", h.ListRecipients)
	r.With(paidGate).Post("/api/v1/notification-recipients", h.CreateRecipient)
	r.Get("/api/v1/notification-recipients/{recipient_id}", h.GetRecipient)
	r.With(paidGate).Patch("/api/v1/notification-recipients/{recipient_id}", h.UpdateRecipient)
	r.With(paidGate).Delete("/api/v1/notification-recipients/{recipient_id}", h.DeleteRecipient)

	r.Get("/api/v1/assets/{asset_id}/notification-subscriptions", h.ListSubscriptions)
	r.With(paidGate).Post("/api/v1/assets/{asset_id}/notification-subscriptions", h.CreateSubscription)
	r.With(paidGate).Patch("/api/v1/assets/{asset_id}/notification-subscriptions/{subscription_id}", h.UpdateSubscription)
}

func (h *Handler) ListRecipients(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	items, err := h.storage.ListNotificationRecipients(r.Context(), orgID)
	if err != nil {
		httputil.RespondStorageError(w, r, err, reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, dataResponse[[]notificationrecipient.Recipient]{Data: items})
}

func (h *Handler) CreateRecipient(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	var req notificationrecipient.CreateRecipientRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	if req.Email == nil && req.Phone == nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, "email or phone is required", reqID)
		return
	}
	rec, err := h.storage.CreateNotificationRecipient(r.Context(), orgID, req)
	if err != nil {
		h.respondStorage(w, r, err, reqID)
		return
	}
	w.Header().Set("Location", "/api/v1/notification-recipients/"+strconv.Itoa(rec.ID))
	httputil.WriteJSON(w, http.StatusCreated, dataResponse[notificationrecipient.Recipient]{Data: *rec})
}

func (h *Handler) GetRecipient(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("recipient_id", chi.URLParam(r, "recipient_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	rec, err := h.storage.GetNotificationRecipient(r.Context(), orgID, id)
	if err != nil {
		httputil.RespondStorageError(w, r, err, reqID)
		return
	}
	if rec == nil {
		httputil.Respond404(w, r, "notification recipient not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, dataResponse[notificationrecipient.Recipient]{Data: *rec})
}

func (h *Handler) UpdateRecipient(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("recipient_id", chi.URLParam(r, "recipient_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	var req notificationrecipient.UpdateRecipientRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	rec, err := h.storage.UpdateNotificationRecipient(r.Context(), orgID, id, req)
	if err != nil {
		h.respondStorage(w, r, err, reqID)
		return
	}
	if rec == nil {
		httputil.Respond404(w, r, "notification recipient not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, dataResponse[notificationrecipient.Recipient]{Data: *rec})
}

func (h *Handler) DeleteRecipient(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("recipient_id", chi.URLParam(r, "recipient_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	ok, err := h.storage.DeleteNotificationRecipient(r.Context(), orgID, id)
	if err != nil {
		httputil.RespondStorageError(w, r, err, reqID)
		return
	}
	if !ok {
		httputil.Respond404(w, r, "notification recipient not found", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	assetID, err := httputil.ParseSurrogateID("asset_id", chi.URLParam(r, "asset_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	items, err := h.storage.ListAssetNotificationSubscriptions(r.Context(), orgID, assetID)
	if err != nil {
		httputil.RespondStorageError(w, r, err, reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, dataResponse[[]notificationrecipient.Subscription]{Data: items})
}

func (h *Handler) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	assetID, err := httputil.ParseSurrogateID("asset_id", chi.URLParam(r, "asset_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	var req notificationrecipient.CreateSubscriptionRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	channel := notificationrecipient.ChannelEmail
	if req.Channel != nil {
		channel = *req.Channel
	}
	sub, created, err := h.storage.CreateAssetNotificationSubscription(r.Context(), orgID, assetID, req.RecipientID, channel)
	if err != nil {
		h.respondStorage(w, r, err, reqID)
		return
	}
	// Subscribing is idempotent: an existing subscription is switched back on
	// and returned with 200 rather than rejected as a duplicate.
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	w.Header().Set("Location", "/api/v1/assets/"+strconv.Itoa(assetID)+"/notification-subscriptions/"+strconv.Itoa(sub.ID))
	httputil.WriteJSON(w, status, dataResponse[notificationrecipient.Subscription]{Data: *sub})
}

func (h *Handler) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	assetID, err := httputil.ParseSurrogateID("asset_id", chi.URLParam(r, "asset_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	subID, err := httputil.ParseSurrogateID("subscription_id", chi.URLParam(r, "subscription_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	var req notificationrecipient.UpdateSubscriptionRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	sub, err := h.storage.UpdateAssetNotificationSubscription(r.Context(), orgID, assetID, subID, req)
	if err != nil {
		h.respondStorage(w, r, err, reqID)
		return
	}
	if sub == nil {
		httputil.Respond404(w, r, "notification subscription not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, dataResponse[notificationrecipient.Subscription]{Data: *sub})
}

// respondStorage maps the subscription sentinels to 404/400 and defers
// everything else, including SQLSTATE 23505 on duplicate contacts, to the
// shared storage-error mapper (409 conflict), as when a channel change would
// duplicate another of the recipient's subscriptions on the asset.
func (h *Handler) respondStorage(w http.ResponseWriter, r *http.Request, err error, reqID string) {
	switch {
	case errors.Is(err, storage.ErrNotificationRecipientNotFound):
		httputil.Respond404(w, r, "notification recipient not found", reqID)
	case errors.Is(err, storage.ErrSubscriptionAssetNotFound):
		httputil.Respond404(w, r, "asset not found", reqID)
	case errors.Is(err, storage.ErrChannelContactMismatch):
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, err.Error(), reqID)
	default:
		httputil.RespondStorageError(w, r, err, reqID)
	}
}
