package orgs

import (
	"encoding/json"
	stderrors "errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// @Summary Create a new API key for an organization
// @Description Mints an API-key JWT scoped to the target org. Session-JWT-only — API-key tokens are rejected with 401.
// @Tags api-keys,internal
// @ID api_keys.create
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param request body apikey.CreateAPIKeyRequest true "Key creation payload"
// @Success 201 {object} map[string]any "data: apikey.APIKeyCreateResponse"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 409 {object} modelerrors.ErrorResponse "Active-key cap reached"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/api-keys [post]
// CreateAPIKey handles POST /api/v1/orgs/{id}/api-keys.
func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", reqID)
		return
	}

	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid org id", "", reqID)
		return
	}

	var req apikey.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid JSON body", "", reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	for _, s := range req.Scopes {
		if !apikey.ValidScopes[s] {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
				"Invalid scope", "Unknown scope: "+s, reqID)
			return
		}
	}

	// Soft cap
	count, err := h.storage.CountActiveAPIKeys(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to check key count", "", reqID)
		return
	}
	if count >= apikey.ActiveKeyCap {
		httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
			"Key limit reached",
			"Organization has reached the 10 active API key limit. Revoke an unused key first.",
			reqID)
		return
	}

	key, err := h.storage.CreateAPIKey(r.Context(), orgID, req.Name, req.Scopes, claims.UserID, req.ExpiresAt)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to create api key", "", reqID)
		return
	}

	signed, err := jwt.GenerateAPIKey(key.JTI, orgID, req.Scopes, req.ExpiresAt)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to sign api key", "", reqID)
		return
	}

	resp := apikey.APIKeyCreateResponse{
		Key:       signed,
		ID:        key.ID,
		Name:      key.Name,
		Scopes:    key.Scopes,
		CreatedAt: key.CreatedAt,
		ExpiresAt: key.ExpiresAt,
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": resp})
}

// @Summary List active API keys for an organization
// @Tags api-keys,internal
// @ID api_keys.list
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Success 200 {object} map[string]any "data: []apikey.APIKeyListItem"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/api-keys [get]
// ListAPIKeys handles GET /api/v1/orgs/{id}/api-keys.
func (h *Handler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid org id", "", reqID)
		return
	}

	keys, err := h.storage.ListActiveAPIKeys(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to list api keys", "", reqID)
		return
	}

	items := make([]apikey.APIKeyListItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, apikey.APIKeyListItem{
			ID:         k.ID,
			JTI:        k.JTI,
			Name:       k.Name,
			Scopes:     k.Scopes,
			CreatedAt:  k.CreatedAt,
			ExpiresAt:  k.ExpiresAt,
			LastUsedAt: k.LastUsedAt,
		})
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

// @Summary Revoke an API key
// @Tags api-keys,internal
// @ID api_keys.revoke
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param keyID path int true "API key id"
// @Success 204 "No Content"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/api-keys/{keyID} [delete]
// RevokeAPIKey handles DELETE /api/v1/orgs/{id}/api-keys/{keyID}.
func (h *Handler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid org id", "", reqID)
		return
	}
	keyID, err := strconv.Atoi(chi.URLParam(r, "keyID"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid key id", "", reqID)
		return
	}

	if err := h.storage.RevokeAPIKey(r.Context(), orgID, keyID); err != nil {
		if stderrors.Is(err, storage.ErrAPIKeyNotFound) {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				"Not found", "API key not found", reqID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to revoke api key", "", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
