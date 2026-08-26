package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/apierrors"
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
// @Description Superadmin-only (TRA-1103). Cross-org list of every user in the system.
// @Tags users,internal
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param per_page query int false "Items per page" default(20)
// @Success 200 {object} users.ListResponse
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "Superadmin privileges required"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security SessionAuth
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
			apierrors.UserListFailed, middleware.GetRequestID(r.Context()))

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
// @Description Superadmin-only (TRA-1103). Reads any user, in any org.
// @Tags users,internal
// @Accept json
// @Produce json
// @Param id path int true "User ID" minimum(1) format(int64)
// @Success 200 {object} map[string]any "data: user.User"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid user ID"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "Superadmin privileges required"
// @Failure 404 {object} modelerrors.ErrorResponse "User not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security SessionAuth
// @Router /api/v1/users/{id} [get]
func (handler *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	u, err := handler.storage.GetUserByID(r.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.UserGetFailed, middleware.GetRequestID(r.Context()))

		return
	}

	if u == nil {
		httputil.Respond404(w, r, apierrors.UserNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": u})
}

// POST /api/v1/users is deliberately absent (TRA-1103).
//
// It existed as ungated CRUD scaffolding and had no callers anywhere — not the
// SPA, not the CLI, and never the public spec. Two things made removing it
// better than keeping it:
//
//   - It could not produce a working account. The request carried a
//     `password_hash` field that storage wrote verbatim into
//     users.password_hash, so the submitted value became the bcrypt hash and
//     could never verify against itself. Every account it created was unable to
//     log in — confirmed live against preview, which returned 401 for a
//     freshly created user.
//   - It could not produce a *useful* account either. It inserted into
//     trakrf.users alone and never wrote an org_users row, so the result
//     belonged to no org and could do nothing even once it could authenticate.
//
// Real user creation happens through signup and the org invitation flow, which
// establish org membership. Directory-style provisioning is expected to arrive
// as SAML/OIDC rather than a REST create endpoint, so restoring this one would
// be building the thing we would then have to deprecate.

// @Summary Update user
// @Description Superadmin-only (TRA-1103). Editing your own profile goes through PATCH /api/v1/users/me. Setting must_change_password gates the user behind the change-password screen at their next login (TRA-1135); omitting the field leaves the flag as it is.
// @Tags users,internal
// @Accept json
// @Produce json
// @Param id path int true "User ID" minimum(1) format(int64)
// @Param request body user.UpdateUserRequest true "User update data"
// @Success 200 {object} map[string]any "data: user.User"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid ID, JSON, or validation error"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "Superadmin privileges required"
// @Failure 404 {object} modelerrors.ErrorResponse "User not found"
// @Failure 409 {object} modelerrors.ErrorResponse "Email already exists"
// @Failure 415 {object} modelerrors.ErrorResponse "unsupported_media_type"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security SessionAuth
// @Router /api/v1/users/{id} [put]
func (handler *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	var request user.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			err.Error(), middleware.GetRequestID(r.Context()))

		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			err.Error(), middleware.GetRequestID(r.Context()))

		return
	}

	u, err := handler.storage.UpdateUser(r.Context(), id, request)
	if err != nil {
		if errors.Is(err, modelerrors.ErrUserDuplicateEmail) {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.UserUpdateEmailExists, middleware.GetRequestID(r.Context()))

			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.UserUpdateFailed, middleware.GetRequestID(r.Context()))

		return
	}

	if u == nil {
		httputil.Respond404(w, r, apierrors.UserUpdateNotFound, middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": u})
}

// @Summary Delete user
// @Description Superadmin-only (TRA-1103). Soft deletes any user, in any org.
// @Tags users,internal
// @Accept json
// @Produce json
// @Param id path int true "User ID" minimum(1) format(int64)
// @Success 204 "No content"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid user ID"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "Superadmin privileges required"
// @Failure 404 {object} modelerrors.ErrorResponse "User not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security SessionAuth
// @Router /api/v1/users/{id} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	if err := handler.storage.SoftDeleteUser(r.Context(), id); err != nil {
		if errors.Is(err, modelerrors.ErrUserNotFound) {
			httputil.Respond404(w, r, apierrors.UserDeleteNotFound, middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.UserDeleteFailed, middleware.GetRequestID(r.Context()))

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RegisterRoutes registers user endpoints on the given router.
//
// TRA-1103: every route here is superadmin-only. This is the cross-org
// back-office identity surface — it reads and writes users regardless of which
// org the caller belongs to, and `trakrf.users` carries no RLS to fall back on,
// so the handler gate is the only thing standing between a session and every
// account in the system. Until this ticket there was no gate at all: any
// authenticated user could enumerate every user with their email, and PUT
// /api/v1/users/{id} could retarget someone else's email to an address the
// caller controlled, then take the account over through the ordinary
// forgot-password flow.
//
// superadmin is middleware.RequireSuperadmin(store). It is threaded in rather
// than built here so the registration site in router.go reads as the complete
// authorization story, and so tests wire the same gate production does.
//
// Deliberately NOT the org-role gates: RequireOrgAdmin would let an admin of
// any org reach users outside it, which is the same hole one tier up. Nothing
// self-service belongs here either — a user editing their own profile goes
// through PATCH /api/v1/users/me (TRA-958), which takes the id from session
// claims and never from the path.
//
// These routes sit outside the entitlement gate for the same reason the
// superadmin org surfaces do: an operator must still be able to act on a
// lapsed org.
func (handler *Handler) RegisterRoutes(r chi.Router, superadmin func(http.Handler) http.Handler) {
	r.With(superadmin).Get("/api/v1/users", handler.List)
	r.With(superadmin).Get("/api/v1/users/{id}", handler.Get)
	r.With(superadmin).Put("/api/v1/users/{id}", handler.Update)
	r.With(superadmin).Delete("/api/v1/users/{id}", handler.Delete)
}
