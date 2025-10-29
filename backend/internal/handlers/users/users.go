package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/i18n"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/models/user"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = validator.New()

type ListResponse struct {
	Data       []user.User       `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}

type Handler struct {
	storage *storage.Storage
}

// NewHandler creates a new users handler instance.
func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// @Summary List users
// @Description Get paginated list of users
// @Tags users
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param per_page query int false "Items per page" default(20)
// @Success 200 {object} users.ListResponse
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/users [get]
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

	users, total, err := handler.storage.ListUsers(r.Context(), perPage, offset)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("users.list.failed"), "", middleware.GetRequestID(r.Context()))
		return
	}

	resp := ListResponse{
		Data: users,
		Pagination: shared.Pagination{
			Page:    page,
			PerPage: perPage,
			Total:   total,
		},
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// @Summary Get user
// @Description Get user by ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} map[string]any "data: user.User"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid user ID"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 404 {object} modelerrors.ErrorResponse "User not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/users/{id} [get]
func (handler *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			i18n.T("users.get.invalid_id"), "", middleware.GetRequestID(r.Context()))
		return
	}

	u, err := handler.storage.GetUserByID(r.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("users.get.failed"), "", middleware.GetRequestID(r.Context()))
		return
	}

	if u == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			i18n.T("users.get.not_found"), "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": u})
}

// @Summary Create user
// @Description Create a new user
// @Tags users
// @Accept json
// @Produce json
// @Param request body user.CreateUserRequest true "User data"
// @Success 201 {object} map[string]any "data: user.User"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid JSON or validation error"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 409 {object} modelerrors.ErrorResponse "Email already exists"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/users [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var request user.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			i18n.T("users.create.invalid_json"), err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			i18n.T("users.create.validation_failed"), err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	u, err := handler.storage.CreateUser(r.Context(), request)
	if err != nil {
		if errors.Is(err, modelerrors.ErrUserDuplicateEmail) {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				i18n.T("users.create.email_exists"), "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("users.create.failed"), "", middleware.GetRequestID(r.Context()))
		return
	}

	w.Header().Set("Location", "/api/v1/users/"+strconv.Itoa(u.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": u})
}

// @Summary Update user
// @Description Update an existing user
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param request body user.UpdateUserRequest true "User update data"
// @Success 200 {object} map[string]any "data: user.User"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid ID, JSON, or validation error"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 404 {object} modelerrors.ErrorResponse "User not found"
// @Failure 409 {object} modelerrors.ErrorResponse "Email already exists"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/users/{id} [put]
func (handler *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			i18n.T("users.update.invalid_id"), "", middleware.GetRequestID(r.Context()))
		return
	}

	var request user.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			i18n.T("users.update.invalid_json"), err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			i18n.T("users.update.validation_failed"), err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	u, err := handler.storage.UpdateUser(r.Context(), id, request)
	if err != nil {
		if errors.Is(err, modelerrors.ErrUserDuplicateEmail) {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				i18n.T("users.update.email_exists"), "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("users.update.failed"), "", middleware.GetRequestID(r.Context()))
		return
	}

	if u == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			i18n.T("users.update.not_found"), "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": u})
}

// @Summary Delete user
// @Description Soft delete a user
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Success 204 "No content"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid user ID"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 404 {object} modelerrors.ErrorResponse "User not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/users/{id} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			i18n.T("users.delete.invalid_id"), "", middleware.GetRequestID(r.Context()))
		return
	}

	if err := handler.storage.SoftDeleteUser(r.Context(), id); err != nil {
		if errors.Is(err, modelerrors.ErrUserNotFound) {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				i18n.T("users.delete.not_found"), "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("users.delete.failed"), "", middleware.GetRequestID(r.Context()))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RegisterRoutes registers user endpoints on the given router.
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/users", handler.List)
	r.Get("/api/v1/users/{id}", handler.Get)
	r.Post("/api/v1/users", handler.Create)
	r.Put("/api/v1/users/{id}", handler.Update)
	r.Delete("/api/v1/users/{id}", handler.Delete)
}
