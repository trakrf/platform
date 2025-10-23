package account_users

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/account_user"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = validator.New()

type ListResponse struct {
	Data       []account_user.AccountUser `json:"data"`
	Pagination shared.Pagination          `json:"pagination"`
}

type Handler struct {
	storage *storage.Storage
}

// NewHandler creates a new account users handler instance.
func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// List handles GET /api/v1/accounts/:account_id/users
func (handler *Handler) List(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.Atoi(chi.URLParam(r, "account_id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid account ID", "", middleware.GetRequestID(r.Context()))
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	offset := (page - 1) * perPage

	accountUsers, total, err := handler.storage.ListAccountUsers(r.Context(), accountID, perPage, offset)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to list account users", "", middleware.GetRequestID(r.Context()))
		return
	}

	response := ListResponse{
		Data: accountUsers,
		Pagination: shared.Pagination{
			Page:    page,
			PerPage: perPage,
			Total:   total,
		},
	}

	httputil.WriteJSON(w, http.StatusOK, response)
}

// Add handles POST /api/v1/accounts/:account_id/users
func (handler *Handler) Add(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.Atoi(chi.URLParam(r, "account_id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid account ID", "", middleware.GetRequestID(r.Context()))
		return
	}

	var request account_user.AddUserToAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid JSON", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			"Validation failed", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	accountUser, err := handler.storage.AddUserToAccount(r.Context(), accountID, request)
	if err != nil {
		if errors.Is(err, modelerrors.ErrAccountUserDuplicate) {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				"User already member of account", "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to add user to account", "", middleware.GetRequestID(r.Context()))
		return
	}

	w.Header().Set("Location", "/api/v1/accounts/"+strconv.Itoa(accountID)+"/users/"+strconv.Itoa(request.UserID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": accountUser})
}

// Update handles PUT /api/v1/accounts/:account_id/users/:user_id
func (handler *Handler) Update(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.Atoi(chi.URLParam(r, "account_id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid account ID", "", middleware.GetRequestID(r.Context()))
		return
	}

	userID, err := strconv.Atoi(chi.URLParam(r, "user_id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid user ID", "", middleware.GetRequestID(r.Context()))
		return
	}

	var request account_user.UpdateAccountUserRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid JSON", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			"Validation failed", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	accountUser, err := handler.storage.UpdateAccountUser(r.Context(), accountID, userID, request)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to update account user", "", middleware.GetRequestID(r.Context()))
		return
	}

	if accountUser == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			"Account user not found", "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": accountUser})
}

// Remove handles DELETE /api/v1/accounts/:account_id/users/:user_id
func (handler *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.Atoi(chi.URLParam(r, "account_id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid account ID", "", middleware.GetRequestID(r.Context()))
		return
	}

	userID, err := strconv.Atoi(chi.URLParam(r, "user_id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid user ID", "", middleware.GetRequestID(r.Context()))
		return
	}

	if err := handler.storage.RemoveUserFromAccount(r.Context(), accountID, userID); err != nil {
		if errors.Is(err, modelerrors.ErrAccountUserNotFound) {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				"Account user not found", "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to remove user from account", "", middleware.GetRequestID(r.Context()))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RegisterRoutes registers account user endpoints on the given router.
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/accounts/{account_id}/users", handler.List)
	r.Post("/api/v1/accounts/{account_id}/users", handler.Add)
	r.Put("/api/v1/accounts/{account_id}/users/{user_id}", handler.Update)
	r.Delete("/api/v1/accounts/{account_id}/users/{user_id}", handler.Remove)
}
