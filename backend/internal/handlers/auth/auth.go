package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/models/errors"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
	"github.com/trakrf/platform/backend/internal/util/password"
)

var validate = validator.New()

type Handler struct {
	service *authservice.Service
}

func NewHandler(service *authservice.Service) *Handler {
	return &Handler{service: service}
}

// @Summary User signup
// @Description Register new user and organization
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.SignupRequest true "Signup request"
// @Success 201 {object} map[string]any "data: auth.SignupResponse"
// @Failure 400 {object} errors.ErrorResponse "Validation error"
// @Failure 409 {object} errors.ErrorResponse "Email or organization name already exists"
// @Failure 500 {object} errors.ErrorResponse "Internal server error"
// @Router /api/v1/auth/signup [post]
func (handler *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	var request auth.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			"Invalid JSON", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			"Validation failed", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	response, err := handler.service.Signup(r.Context(), request, password.Hash, jwt.Generate)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "email already exists") {
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				"Email already exists", "", middleware.GetRequestID(r.Context()))
			return
		}
		if strings.Contains(errMsg, "organization name already taken") {
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				"Organization name already taken", "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			"Failed to signup", "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": response})
}

// @Summary User login
// @Description Authenticate and receive JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.LoginRequest true "Login credentials"
// @Success 200 {object} map[string]any "data: auth.LoginResponse"
// @Failure 400 {object} errors.ErrorResponse "Validation error"
// @Failure 401 {object} errors.ErrorResponse "Invalid credentials"
// @Failure 500 {object} errors.ErrorResponse "Internal server error"
// @Router /api/v1/auth/login [post]
func (handler *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var request auth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			"Invalid JSON", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			"Validation failed", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	response, err := handler.service.Login(r.Context(), request, password.Compare, jwt.Generate)
	if err != nil {
		if strings.Contains(err.Error(), "invalid email or password") {
			httputil.WriteJSONError(w, r, http.StatusUnauthorized, errors.ErrUnauthorized,
				"Invalid email or password", "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			"Failed to login", "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": response})
}

func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/auth/signup", handler.Signup)
	r.Post("/api/v1/auth/login", handler.Login)
}
