package accounts

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/account"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = validator.New()

type ListResponse struct {
	Data       []account.Account `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}

type Handler struct {
	storage *storage.Storage
}

// NewHandler creates a new accounts handler instance.
func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// List handles GET /api/v1/accounts
func (handler *Handler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	offset := (page - 1) * perPage

	accounts, total, err := handler.storage.ListAccounts(r.Context(), perPage, offset)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to list accounts", "", middleware.GetRequestID(r.Context()))
		return
	}

	resp := ListResponse{
		Data: accounts,
		Pagination: shared.Pagination{
			Page:    page,
			PerPage: perPage,
			Total:   total,
		},
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// Get handles GET /api/v1/accounts/:id
func (handler *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid account ID", "", middleware.GetRequestID(r.Context()))
		return
	}

	acct, err := handler.storage.GetAccountByID(r.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to get account", "", middleware.GetRequestID(r.Context()))
		return
	}

	if acct == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			"Account not found", "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": acct})
}

// Create handles POST /api/v1/accounts
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var request account.CreateAccountRequest
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

	acct, err := handler.storage.CreateAccount(r.Context(), request)
	if err != nil {
		if errors.Is(err, modelerrors.ErrAccountDuplicateDomain) {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				"Domain already exists", "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to create account", "", middleware.GetRequestID(r.Context()))
		return
	}

	w.Header().Set("Location", "/api/v1/accounts/"+strconv.Itoa(acct.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": acct})
}

// Update handles PUT /api/v1/accounts/:id
func (handler *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid account ID", "", middleware.GetRequestID(r.Context()))
		return
	}

	var request account.UpdateAccountRequest
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

	acct, err := handler.storage.UpdateAccount(r.Context(), id, request)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to update account", "", middleware.GetRequestID(r.Context()))
		return
	}

	if acct == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			"Account not found", "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": acct})
}

// Delete handles DELETE /api/v1/accounts/:id
func (handler *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid account ID", "", middleware.GetRequestID(r.Context()))
		return
	}

	if err := handler.storage.SoftDeleteAccount(r.Context(), id); err != nil {
		if errors.Is(err, modelerrors.ErrAccountNotFound) {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				"Account not found", "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to delete account", "", middleware.GetRequestID(r.Context()))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RegisterRoutes registers account endpoints on the given router.
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/accounts", handler.List)
	r.Get("/api/v1/accounts/{id}", handler.Get)
	r.Post("/api/v1/accounts", handler.Create)
	r.Put("/api/v1/accounts/{id}", handler.Update)
	r.Delete("/api/v1/accounts/{id}", handler.Delete)
}
