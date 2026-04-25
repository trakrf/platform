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

// CreateAPIKeyResponse is the typed envelope returned by
// POST /api/v1/orgs/{id}/api-keys. The embedded apikey.APIKeyCreateResponse
// carries the freshly minted JWT — returned exactly once, never persisted.
type CreateAPIKeyResponse struct {
	Data apikey.APIKeyCreateResponse `json:"data"`
}

// ListAPIKeysResponse is the typed envelope returned by
// GET /api/v1/orgs/{id}/api-keys.
type ListAPIKeysResponse struct {
	Data       []apikey.APIKeyListItem `json:"data"`
	Limit      int                     `json:"limit"       example:"50"`
	Offset     int                     `json:"offset"      example:"0"`
	TotalCount int                     `json:"total_count" example:"100"`
}

// @Summary Create a new API key for an organization
// @Description Mints an API-key JWT scoped to the target org. Accepts either session-admin or an API key with the keys:admin scope.
// @Tags api-keys,public
// @ID api_keys.create
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param request body apikey.CreateAPIKeyRequest true "Key creation payload"
// @Success 201 {object} orgs.CreateAPIKeyResponse
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 409 {object} modelerrors.ErrorResponse "Active-key cap reached"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Security APIKey[keys:admin]
// @Router /api/v1/orgs/{id}/api-keys [post]
// CreateAPIKey handles POST /api/v1/orgs/{id}/api-keys.
func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	// Resolve creator — exactly one of session-user or api-key-principal must be present.
	var creator apikey.Creator
	if claims := middleware.GetUserClaims(r); claims != nil {
		userID := claims.UserID
		creator = apikey.Creator{UserID: &userID}
	} else if p := middleware.GetAPIKeyPrincipal(r); p != nil {
		parent, err := h.storage.GetAPIKeyByJTI(r.Context(), p.JTI)
		if err != nil {
			if stderrors.Is(err, storage.ErrAPIKeyNotFound) {
				httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
					"API key is no longer valid", "", reqID)
				return
			}
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
				"Failed to resolve parent key", "", reqID)
			return
		}
		parentID := parent.ID
		creator = apikey.Creator{KeyID: &parentID}
	} else {
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

	key, err := h.storage.CreateAPIKey(r.Context(), orgID, req.Name, req.Scopes,
		creator, req.ExpiresAt)
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
// @Tags api-keys,public
// @ID api_keys.list
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param limit query int false "max 200"   default(50)
// @Param offset query int false "min 0"    default(0)
// @Success 200 {object} orgs.ListAPIKeysResponse
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Security APIKey[keys:admin]
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

	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, r, err, reqID)
		return
	}

	keys, err := h.storage.ListActiveAPIKeysPaginated(r.Context(), orgID, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to list api keys", "", reqID)
		return
	}

	total, err := h.storage.CountActiveAPIKeys(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to count api keys", "", reqID)
		return
	}

	items := make([]apikey.APIKeyListItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, apikey.APIKeyListItem{
			ID:             k.ID,
			JTI:            k.JTI,
			Name:           k.Name,
			Scopes:         k.Scopes,
			CreatedBy:      k.CreatedBy,
			CreatedByKeyID: k.CreatedByKeyID,
			CreatedAt:      k.CreatedAt,
			ExpiresAt:      k.ExpiresAt,
			LastUsedAt:     k.LastUsedAt,
		})
	}

	httputil.WriteJSON(w, http.StatusOK, ListAPIKeysResponse{
		Data:       items,
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

// @Summary Revoke an API key
// @Tags api-keys,public
// @ID api_keys.revoke
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param key_id path int true "API key id"
// @Success 204 "No Content"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Security APIKey[keys:admin]
// @Router /api/v1/orgs/{id}/api-keys/{key_id} [delete]
// RevokeAPIKey handles DELETE /api/v1/orgs/{id}/api-keys/{key_id}.
func (h *Handler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid org id", "", reqID)
		return
	}
	keyID, err := strconv.Atoi(chi.URLParam(r, "key_id"))
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
