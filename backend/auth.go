package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

var authService *AuthService

// initAuthService initializes the authentication service
func initAuthService() {
	authService = NewAuthService(db, userRepo, accountRepo, accountUserRepo)
}

// signupHandler handles POST /api/v1/auth/signup
func signupHandler(w http.ResponseWriter, r *http.Request) {
	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := validate.Struct(req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
		return
	}

	resp, err := authService.Signup(r.Context(), req)
	if err != nil {
		// Check for specific error types
		errMsg := err.Error()
		if strings.Contains(errMsg, "email already exists") {
			writeJSONError(w, r, http.StatusConflict, ErrConflict, "Email already exists", "")
			return
		}
		if strings.Contains(errMsg, "account name already taken") {
			writeJSONError(w, r, http.StatusConflict, ErrConflict, "Account name already taken", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to signup", "")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": resp})
}

// loginHandler handles POST /api/v1/auth/login
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := validate.Struct(req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
		return
	}

	resp, err := authService.Login(r.Context(), req)
	if err != nil {
		// Generic error for security (prevent email enumeration)
		if strings.Contains(err.Error(), "invalid email or password") {
			writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized, "Invalid email or password", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to login", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": resp})
}

// registerAuthRoutes registers authentication endpoints
func registerAuthRoutes(r chi.Router) {
	r.Post("/api/v1/auth/signup", signupHandler)
	r.Post("/api/v1/auth/login", loginHandler)
}
