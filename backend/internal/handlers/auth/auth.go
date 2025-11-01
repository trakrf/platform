package auth

import (
	"encoding/json"
	"log"
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

// sanitizeEmail returns a partially redacted email for safe logging
// Example: john.doe@example.com -> j***@example.com
func sanitizeEmail(email string) string {
	if email == "" {
		return "[empty]"
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "[invalid-email]"
	}
	local := parts[0]
	if len(local) <= 1 {
		return string(local[0]) + "***@" + parts[1]
	}
	return string(local[0]) + "***@" + parts[1]
}

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
	requestID := middleware.GetRequestID(r.Context())
	log.Printf("[Auth] [Signup] [%s] Request received from %s", requestID, r.RemoteAddr)

	var request auth.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("[Auth] [Signup] [%s] Failed to decode JSON: %v", requestID, err)
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			apierrors.AuthSignupInvalidJSON, err.Error(), requestID)
		return
	}

	log.Printf("[Auth] [Signup] [%s] Request for email: %s", requestID, sanitizeEmail(request.Email))

	if err := validate.Struct(request); err != nil {
		log.Printf("[Auth] [Signup] [%s] Validation failed for email %s: %v", requestID, sanitizeEmail(request.Email), err)
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			apierrors.AuthSignupValidationFailed, err.Error(), requestID)
		return
	}

	log.Printf("[Auth] [Signup] [%s] Calling service for email: %s", requestID, sanitizeEmail(request.Email))
	response, err := handler.service.Signup(r.Context(), request, password.Hash, jwt.Generate)
	if err != nil {
		errMsg := err.Error()
		log.Printf("[Auth] [Signup] [%s] Service error for email %s: %v", requestID, sanitizeEmail(request.Email), err)

		if strings.Contains(errMsg, "email already exists") {
			log.Printf("[Auth] [Signup] [%s] Email already exists: %s", requestID, sanitizeEmail(request.Email))
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				apierrors.AuthSignupEmailExists, "", requestID)
			return
		}
		if strings.Contains(errMsg, "organization identifier already taken") {
			log.Printf("[Auth] [Signup] [%s] Organization identifier already taken for email: %s", requestID, sanitizeEmail(request.Email))
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				apierrors.AuthSignupOrgIdentifierTaken, "", requestID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			apierrors.AuthSignupFailed, "", requestID)
		return
	}

	log.Printf("[Auth] [Signup] [%s] Success for email: %s, user_id: %d", requestID, sanitizeEmail(request.Email), response.User.ID)
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
	requestID := middleware.GetRequestID(r.Context())
	log.Printf("[Auth] [Login] [%s] Request received from %s", requestID, r.RemoteAddr)

	var request auth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("[Auth] [Login] [%s] Failed to decode JSON: %v", requestID, err)
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			apierrors.AuthLoginInvalidJSON, err.Error(), requestID)
		return
	}

	log.Printf("[Auth] [Login] [%s] Attempt for email: %s", requestID, sanitizeEmail(request.Email))

	if err := validate.Struct(request); err != nil {
		log.Printf("[Auth] [Login] [%s] Validation failed for email %s: %v", requestID, sanitizeEmail(request.Email), err)
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			apierrors.AuthLoginValidationFailed, err.Error(), requestID)
		return
	}

	log.Printf("[Auth] [Login] [%s] Calling service for email: %s", requestID, sanitizeEmail(request.Email))
	response, err := handler.service.Login(r.Context(), request, password.Compare, jwt.Generate)
	if err != nil {
		log.Printf("[Auth] [Login] [%s] Service error for email %s: %v", requestID, sanitizeEmail(request.Email), err)

		if strings.Contains(err.Error(), "invalid email or password") {
			log.Printf("[Auth] [Login] [%s] Invalid credentials for email: %s", requestID, sanitizeEmail(request.Email))
			httputil.WriteJSONError(w, r, http.StatusUnauthorized, errors.ErrUnauthorized,
				apierrors.AuthLoginInvalidCredentials, "", requestID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			apierrors.AuthLoginFailed, "", requestID)
		return
	}

	log.Printf("[Auth] [Login] [%s] Success for email: %s, user_id: %d", requestID, sanitizeEmail(request.Email), response.User.ID)
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": response})
}

func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/auth/signup", handler.Signup)
	r.Post("/api/v1/auth/login", handler.Login)
}
