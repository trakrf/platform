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
	log.Printf("[Auth] [Signup] [%s] ===== START SIGNUP REQUEST =====", requestID)
	log.Printf("[Auth] [Signup] [%s] Request received from %s, Method: %s, ContentType: %s",
		requestID, r.RemoteAddr, r.Method, r.Header.Get("Content-Type"))

	var request auth.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("[Auth] [Signup] [%s] ERROR: Failed to decode JSON body: %v", requestID, err)
		log.Printf("[Auth] [Signup] [%s] ERROR: Returning 400 Bad Request - Invalid JSON", requestID)
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			apierrors.AuthSignupInvalidJSON, err.Error(), requestID)
		return
	}

	log.Printf("[Auth] [Signup] [%s] Decoded request - Email: %s, Password length: %d chars",
		requestID, sanitizeEmail(request.Email), len(request.Password))

	if err := validate.Struct(request); err != nil {
		log.Printf("[Auth] [Signup] [%s] ERROR: Validation failed for email %s", requestID, sanitizeEmail(request.Email))
		log.Printf("[Auth] [Signup] [%s] ERROR: Validation details: %v", requestID, err)

		// Log each validation error individually
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			for _, fieldErr := range validationErrors {
				log.Printf("[Auth] [Signup] [%s] ERROR: Field '%s' failed validation '%s' (value: '%v')",
					requestID, fieldErr.Field(), fieldErr.Tag(), fieldErr.Value())
			}
		}

		log.Printf("[Auth] [Signup] [%s] ERROR: Returning 400 Bad Request - Validation Failed", requestID)
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			apierrors.AuthSignupValidationFailed, err.Error(), requestID)
		return
	}

	log.Printf("[Auth] [Signup] [%s] Validation passed, calling service layer", requestID)
	response, err := handler.service.Signup(r.Context(), request, password.Hash, jwt.Generate)
	if err != nil {
		errMsg := err.Error()
		log.Printf("[Auth] [Signup] [%s] ERROR: Service returned error for email %s: %v", requestID, sanitizeEmail(request.Email), err)

		if strings.Contains(errMsg, "email already exists") {
			log.Printf("[Auth] [Signup] [%s] ERROR: Duplicate email detected: %s", requestID, sanitizeEmail(request.Email))
			log.Printf("[Auth] [Signup] [%s] ERROR: Returning 409 Conflict - Email Exists", requestID)
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				apierrors.AuthSignupEmailExists, "", requestID)
			return
		}
		if strings.Contains(errMsg, "organization identifier already taken") {
			log.Printf("[Auth] [Signup] [%s] ERROR: Duplicate org identifier for email: %s", requestID, sanitizeEmail(request.Email))
			log.Printf("[Auth] [Signup] [%s] ERROR: Returning 409 Conflict - Org Identifier Taken", requestID)
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				apierrors.AuthSignupOrgIdentifierTaken, "", requestID)
			return
		}
		log.Printf("[Auth] [Signup] [%s] ERROR: Unknown error, returning 500 Internal Server Error", requestID)
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			apierrors.AuthSignupFailed, "", requestID)
		return
	}

	log.Printf("[Auth] [Signup] [%s] SUCCESS: Signup completed for email: %s, user_id: %d", requestID, sanitizeEmail(request.Email), response.User.ID)
	log.Printf("[Auth] [Signup] [%s] ===== END SIGNUP REQUEST (SUCCESS) =====", requestID)
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
	log.Printf("[Auth] [Login] [%s] ===== START LOGIN REQUEST =====", requestID)
	log.Printf("[Auth] [Login] [%s] Request received from %s, Method: %s, ContentType: %s",
		requestID, r.RemoteAddr, r.Method, r.Header.Get("Content-Type"))

	var request auth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("[Auth] [Login] [%s] ERROR: Failed to decode JSON body: %v", requestID, err)
		log.Printf("[Auth] [Login] [%s] ERROR: Returning 400 Bad Request - Invalid JSON", requestID)
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			apierrors.AuthLoginInvalidJSON, err.Error(), requestID)
		return
	}

	log.Printf("[Auth] [Login] [%s] Decoded request - Email: %s, Password length: %d chars",
		requestID, sanitizeEmail(request.Email), len(request.Password))

	if err := validate.Struct(request); err != nil {
		log.Printf("[Auth] [Login] [%s] ERROR: Validation failed for email %s", requestID, sanitizeEmail(request.Email))
		log.Printf("[Auth] [Login] [%s] ERROR: Validation details: %v", requestID, err)

		// Log each validation error individually
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			for _, fieldErr := range validationErrors {
				log.Printf("[Auth] [Login] [%s] ERROR: Field '%s' failed validation '%s' (value: '%v')",
					requestID, fieldErr.Field(), fieldErr.Tag(), fieldErr.Value())
			}
		}

		log.Printf("[Auth] [Login] [%s] ERROR: Returning 400 Bad Request - Validation Failed", requestID)
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			apierrors.AuthLoginValidationFailed, err.Error(), requestID)
		return
	}

	log.Printf("[Auth] [Login] [%s] Validation passed, calling service layer", requestID)
	response, err := handler.service.Login(r.Context(), request, password.Compare, jwt.Generate)
	if err != nil {
		log.Printf("[Auth] [Login] [%s] ERROR: Service returned error for email %s: %v", requestID, sanitizeEmail(request.Email), err)

		if strings.Contains(err.Error(), "invalid email or password") {
			log.Printf("[Auth] [Login] [%s] ERROR: Invalid credentials for email: %s", requestID, sanitizeEmail(request.Email))
			log.Printf("[Auth] [Login] [%s] ERROR: Returning 401 Unauthorized - Invalid Credentials", requestID)
			httputil.WriteJSONError(w, r, http.StatusUnauthorized, errors.ErrUnauthorized,
				apierrors.AuthLoginInvalidCredentials, "", requestID)
			return
		}
		log.Printf("[Auth] [Login] [%s] ERROR: Unknown error, returning 500 Internal Server Error", requestID)
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			apierrors.AuthLoginFailed, "", requestID)
		return
	}

	log.Printf("[Auth] [Login] [%s] SUCCESS: Login completed for email: %s, user_id: %d", requestID, sanitizeEmail(request.Email), response.User.ID)
	log.Printf("[Auth] [Login] [%s] ===== END LOGIN REQUEST (SUCCESS) =====", requestID)
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": response})
}

func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/auth/signup", handler.Signup)
	r.Post("/api/v1/auth/login", handler.Login)
}
