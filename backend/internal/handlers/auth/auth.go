package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/apierrors"
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
// @Description Register new user with auto-created personal organization
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.SignupRequest true "Signup request (email and password only)"
// @Success 201 {object} map[string]any "data: auth.SignupResponse"
// @Failure 400 {object} errors.ErrorResponse "Validation error"
// @Failure 409 {object} errors.ErrorResponse "Email already exists"
// @Failure 500 {object} errors.ErrorResponse "Internal server error"
// @Router /api/v1/auth/signup [post]
func (handler *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	var request auth.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			apierrors.AuthSignupInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			apierrors.AuthSignupValidationFailed, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	response, err := handler.service.Signup(r.Context(), request, password.Hash, jwt.Generate)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "email already exists") {
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				apierrors.AuthSignupEmailExists, "", middleware.GetRequestID(r.Context()))
			return
		}
		if strings.Contains(errMsg, "organization identifier already taken") {
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				apierrors.AuthSignupOrgIdentifierTaken, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			apierrors.AuthSignupFailed, "", middleware.GetRequestID(r.Context()))
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
			apierrors.AuthLoginInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			apierrors.AuthLoginValidationFailed, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	response, err := handler.service.Login(r.Context(), request, password.Compare, jwt.Generate)
	if err != nil {
		if strings.Contains(err.Error(), "invalid email or password") {
			httputil.WriteJSONError(w, r, http.StatusUnauthorized, errors.ErrUnauthorized,
				apierrors.AuthLoginInvalidCredentials, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			apierrors.AuthLoginFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": response})
}

// @Summary Request password reset
// @Description Send a password reset email if account exists
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.ForgotPasswordRequest true "Email address"
// @Success 200 {object} auth.MessageResponse "Success message (always returns 200)"
// @Failure 400 {object} errors.ErrorResponse "Validation error"
// @Router /api/v1/auth/forgot-password [post]
func (handler *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var request auth.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			apierrors.AuthForgotPasswordInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			apierrors.AuthForgotPasswordValidation, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	// ForgotPassword always returns nil to avoid leaking account existence
	_ = handler.service.ForgotPassword(r.Context(), request.Email, request.ResetURL)

	// Always return success to avoid leaking whether email exists
	httputil.WriteJSON(w, http.StatusOK, auth.MessageResponse{
		Message: "If an account exists, a reset email has been sent",
	})
}

// @Summary Reset password
// @Description Reset password using a valid token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.ResetPasswordRequest true "Token and new password"
// @Success 200 {object} auth.MessageResponse "Success message"
// @Failure 400 {object} errors.ErrorResponse "Invalid or expired token"
// @Failure 500 {object} errors.ErrorResponse "Internal server error"
// @Router /api/v1/auth/reset-password [post]
func (handler *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var request auth.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			apierrors.AuthResetPasswordInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			apierrors.AuthResetPasswordValidation, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	err := handler.service.ResetPassword(r.Context(), request.Token, request.Password, password.Hash)
	if err != nil {
		if strings.Contains(err.Error(), "invalid or expired") {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				apierrors.AuthResetPasswordInvalidToken, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			apierrors.AuthResetPasswordFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, auth.MessageResponse{
		Message: "Password updated successfully",
	})
}

func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/auth/signup", handler.Signup)
	r.Post("/api/v1/auth/login", handler.Login)
	r.Post("/api/v1/auth/forgot-password", handler.ForgotPassword)
	r.Post("/api/v1/auth/reset-password", handler.ResetPassword)
}
