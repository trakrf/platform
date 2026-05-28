package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/organization"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
	"github.com/trakrf/platform/backend/internal/util/password"
)

var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	return v
}()

// authServicer is the subset of authservice.Service used by Handler.
// Defined as an interface to allow test stubs.
type authServicer interface {
	Signup(ctx context.Context, request auth.SignupRequest, userAgent, ip string, hashPassword func(string) (string, error), generateJWT func(int, string, *int) (string, error)) (*auth.AuthResponse, error)
	Login(ctx context.Context, request auth.LoginRequest, userAgent, ip string, comparePassword func(string, string) error, generateJWT func(int, string, *int) (string, error)) (*auth.AuthResponse, error)
	Refresh(ctx context.Context, presentedSecret, userAgent, ip string, generateJWT func(int, string, *int) (string, error)) (*auth.RefreshResponse, error)
	Logout(ctx context.Context, presentedSecret string) error
	ForgotPassword(ctx context.Context, emailAddr, resetURL string) error
	ResetPassword(ctx context.Context, token, newPassword string, hashPassword func(string) (string, error)) error
	AcceptInvitation(ctx context.Context, token string, userID int) (*organization.AcceptInvitationResponse, error)
	GetInvitationInfo(ctx context.Context, token string) (*auth.InvitationInfoResponse, error)
	MintAPITokenPair(ctx context.Context, jti string, scopes []string, orgID int, apiKeyID int64, userAgent, ip string) (accessToken, refreshSecret string, expiresIn int, err error)
	RefreshAPIToken(ctx context.Context, presentedSecret, userAgent, ip string) (*authservice.APITokenResponse, error)
}

// Ensure *authservice.Service satisfies authServicer at compile time.
var _ authServicer = (*authservice.Service)(nil)

type Handler struct {
	service authServicer
	store   *storage.Storage
}

func NewHandler(service *authservice.Service, store *storage.Storage) *Handler {
	return &Handler{service: service, store: store}
}

// @Summary User signup
// @Description Register new user with auto-created personal organization
// @Tags auth,internal
// @Accept json
// @Produce json
// @Param request body auth.SignupRequest true "Signup request (email and password only)"
// @Success 201 {object} map[string]any "data: auth.SignupResponse"
// @Failure 400 {object} errors.ErrorResponse "Validation error"
// @Failure 409 {object} errors.ErrorResponse "Email already exists"
// @Failure 415 {object} errors.ErrorResponse "unsupported_media_type"
// @Failure 500 {object} errors.ErrorResponse "Internal server error"
// @Router /api/v1/auth/signup [post]
func (handler *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	var request auth.SignupRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	response, err := handler.service.Signup(r.Context(), request, r.UserAgent(), clientIP(r), password.Hash, jwt.Generate)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "email already exists") {
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				apierrors.AuthSignupEmailExists, middleware.GetRequestID(r.Context()))

			return
		}
		if strings.Contains(errMsg, "organization identifier already taken") {
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				apierrors.AuthSignupOrgIdentifierTaken, middleware.GetRequestID(r.Context()))

			return
		}
		// Handle invitation-related errors
		if strings.HasPrefix(errMsg, "email_mismatch:") {
			invitedEmail := strings.TrimPrefix(errMsg, "email_mismatch:")
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				fmt.Sprintf(apierrors.SignupInvitationEmailMismatch, invitedEmail), middleware.GetRequestID(r.Context()))

			return
		}
		switch errMsg {
		case "invalid_token":
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				apierrors.InvitationInvalidToken, middleware.GetRequestID(r.Context()))

			return
		case "expired":
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				apierrors.InvitationExpired, middleware.GetRequestID(r.Context()))

			return
		case "cancelled":
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				apierrors.InvitationCancelled, middleware.GetRequestID(r.Context()))

			return
		case "already_accepted":
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				apierrors.InvitationAcceptAlreadyUsed, middleware.GetRequestID(r.Context()))

			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			apierrors.AuthSignupFailed, middleware.GetRequestID(r.Context()))

		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": response})
}

// @Summary User login
// @Description Authenticate and receive JWT token
// @Tags auth,internal
// @Accept json
// @Produce json
// @Param request body auth.LoginRequest true "Login credentials"
// @Success 200 {object} map[string]any "data: auth.LoginResponse"
// @Failure 400 {object} errors.ErrorResponse "Validation error"
// @Failure 401 {object} errors.ErrorResponse "Invalid credentials"
// @Failure 415 {object} errors.ErrorResponse "unsupported_media_type"
// @Failure 500 {object} errors.ErrorResponse "Internal server error"
// @Router /api/v1/auth/login [post]
func (handler *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var request auth.LoginRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	response, err := handler.service.Login(r.Context(), request, r.UserAgent(), clientIP(r), password.Compare, jwt.Generate)
	if err != nil {
		if strings.Contains(err.Error(), "invalid email or password") {
			httputil.Respond401(w, r, "Invalid email or password", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			apierrors.AuthLoginFailed, middleware.GetRequestID(r.Context()))

		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": response})
}

// @Summary Request password reset
// @Description Send a password reset email if account exists
// @Tags auth,internal
// @Accept json
// @Produce json
// @Param request body auth.ForgotPasswordRequest true "Email address"
// @Success 200 {object} auth.MessageResponse "Success message (always returns 200)"
// @Failure 400 {object} errors.ErrorResponse "Validation error"
// @Failure 415 {object} errors.ErrorResponse "unsupported_media_type"
// @Router /api/v1/auth/forgot-password [post]
func (handler *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var request auth.ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			err.Error(), middleware.GetRequestID(r.Context()))

		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			err.Error(), middleware.GetRequestID(r.Context()))

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
// @Tags auth,internal
// @Accept json
// @Produce json
// @Param request body auth.ResetPasswordRequest true "Token and new password"
// @Success 200 {object} auth.MessageResponse "Success message"
// @Failure 400 {object} errors.ErrorResponse "Invalid or expired token"
// @Failure 415 {object} errors.ErrorResponse "unsupported_media_type"
// @Failure 500 {object} errors.ErrorResponse "Internal server error"
// @Router /api/v1/auth/reset-password [post]
func (handler *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var request auth.ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			err.Error(), middleware.GetRequestID(r.Context()))

		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			err.Error(), middleware.GetRequestID(r.Context()))

		return
	}

	err := handler.service.ResetPassword(r.Context(), request.Token, request.Password, password.Hash)
	if err != nil {
		if strings.Contains(err.Error(), "invalid or expired") {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				apierrors.AuthResetPasswordInvalidToken, middleware.GetRequestID(r.Context()))

			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			apierrors.AuthResetPasswordFailed, middleware.GetRequestID(r.Context()))

		return
	}

	httputil.WriteJSON(w, http.StatusOK, auth.MessageResponse{
		Message: "Password updated successfully",
	})
}

// @Summary Accept organization invitation
// @Description Accept an invitation to join an organization using the token
// @Tags auth,internal
// @Accept json
// @Produce json
// @Param request body organization.AcceptInvitationRequest true "Invitation token"
// @Success 200 {object} map[string]any "data: organization.AcceptInvitationResponse"
// @Failure 400 {object} errors.ErrorResponse "Invalid or expired token"
// @Failure 401 {object} errors.ErrorResponse "Not authenticated"
// @Failure 409 {object} errors.ErrorResponse "Already a member"
// @Failure 415 {object} errors.ErrorResponse "unsupported_media_type"
// @Failure 500 {object} errors.ErrorResponse "Internal server error"
// @Security SessionAuth
// @Router /api/v1/auth/accept-invite [post]
func (handler *Handler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.Respond401(w, r, "Please log in to accept this invitation", middleware.GetRequestID(r.Context()))
		return
	}

	var request organization.AcceptInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			err.Error(), middleware.GetRequestID(r.Context()))

		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrValidation,
			err.Error(), middleware.GetRequestID(r.Context()))

		return
	}

	response, err := handler.service.AcceptInvitation(r.Context(), request.Token, claims.UserID)
	if err != nil {
		errMsg := err.Error()
		// Check for email mismatch (format: "email_mismatch:{invited_email}")
		if strings.HasPrefix(errMsg, "email_mismatch:") {
			invitedEmail := strings.TrimPrefix(errMsg, "email_mismatch:")
			httputil.WriteJSONError(w, r, http.StatusForbidden, errors.ErrForbidden,
				fmt.Sprintf(apierrors.InvitationAcceptEmailMismatch, invitedEmail), middleware.GetRequestID(r.Context()))

			return
		}
		switch errMsg {
		case "invalid_token":
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				apierrors.InvitationInvalidToken, middleware.GetRequestID(r.Context()))

		case "expired":
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				apierrors.InvitationExpired, middleware.GetRequestID(r.Context()))

		case "cancelled":
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				apierrors.InvitationCancelled, middleware.GetRequestID(r.Context()))

		case "already_accepted":
			httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
				apierrors.InvitationAcceptAlreadyUsed, middleware.GetRequestID(r.Context()))

		case "already_member":
			httputil.WriteJSONError(w, r, http.StatusConflict, errors.ErrConflict,
				apierrors.InvitationAcceptAlreadyMember, middleware.GetRequestID(r.Context()))

		default:
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
				apierrors.InvitationAcceptFailed, middleware.GetRequestID(r.Context()))

		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": response})
}

// @Summary Get invitation info
// @Description Get invitation details (org name, role) without authentication
// @Tags auth,internal
// @Produce json
// @Param token query string true "Invitation token"
// @Success 200 {object} map[string]any "data: auth.InvitationInfoResponse"
// @Failure 400 {object} errors.ErrorResponse "Missing token"
// @Failure 404 {object} errors.ErrorResponse "Invalid/expired/cancelled token"
// @Failure 500 {object} errors.ErrorResponse "Internal server error"
// @Router /api/v1/auth/invitation-info [get]
func (handler *Handler) GetInvitationInfo(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, errors.ErrBadRequest,
			apierrors.InvitationInfoMissingToken, middleware.GetRequestID(r.Context()))

		return
	}

	response, err := handler.service.GetInvitationInfo(r.Context(), token)
	if err != nil {
		errMsg := err.Error()
		// All these cases return 404 to avoid leaking token validity
		switch errMsg {
		case "invalid_token", "expired", "cancelled", "already_accepted":
			httputil.Respond404(w, r, apierrors.InvitationInvalidToken, middleware.GetRequestID(r.Context()))
		default:
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
				apierrors.InvitationInfoFailed, middleware.GetRequestID(r.Context()))

		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": response})
}

// @Summary Refresh access token
// @Description Exchange a current refresh token for a new access JWT + rotated refresh token. Single-use: presenting an already-used refresh token revokes the chain and returns 401.
// @Tags auth,internal
// @Accept json
// @Produce json
// @Param request body auth.RefreshRequest true "Current refresh token"
// @Success 200 {object} auth.RefreshResponse
// @Failure 400 {object} errors.ErrorResponse "Validation error"
// @Failure 401 {object} errors.ErrorResponse "Invalid, expired, used, or revoked refresh token"
// @Failure 415 {object} errors.ErrorResponse "unsupported_media_type"
// @Router /api/v1/auth/refresh [post]
func (handler *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var request auth.RefreshRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	response, err := handler.service.Refresh(r.Context(), request.RefreshToken, r.UserAgent(), clientIP(r), jwt.Generate)
	if err != nil {
		// Treat every failure path as opaque to the caller — replay, expiry,
		// revocation, and unknown all collapse to 401. The chain-revoke
		// side effect on replay is server-side only.
		httputil.Respond401(w, r, "Invalid or expired refresh token", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, response)
}

// @Summary Logout (revoke refresh token)
// @Description Invalidate the supplied refresh token so it cannot be exchanged again. The access JWT remains valid until its short TTL elapses — clients should also drop it client-side.
// @Tags auth,internal
// @Accept json
// @Produce json
// @Param request body auth.LogoutRequest true "Refresh token to revoke"
// @Success 200 {object} auth.MessageResponse
// @Failure 400 {object} errors.ErrorResponse "Validation error"
// @Failure 415 {object} errors.ErrorResponse "unsupported_media_type"
// @Router /api/v1/auth/logout [post]
func (handler *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var request auth.LogoutRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	// Tolerant: revoking an unknown token returns 200 to avoid revealing
	// hash existence. The service swallows not-found.
	if err := handler.service.Logout(r.Context(), request.RefreshToken); err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, errors.ErrInternal,
			"Failed to revoke refresh token", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, auth.MessageResponse{Message: "Logged out"})
}

// clientIP returns the originating client IP for a request. Prefers
// X-Forwarded-For (first hop) when the request arrived through a proxy,
// otherwise falls back to RemoteAddr stripped of its port.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// XFF is "client, proxy1, proxy2" — first is the originator.
		for i, c := range xff {
			if c == ',' {
				return strings.TrimSpace(xff[:i])
			}
		}
		return strings.TrimSpace(xff)
	}
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

func (handler *Handler) RegisterRoutes(r chi.Router, jwtMiddleware func(http.Handler) http.Handler) {
	r.Post("/api/v1/auth/signup", handler.Signup)
	r.Post("/api/v1/auth/login", handler.Login)
	r.Post("/api/v1/auth/refresh", handler.Refresh)
	r.Post("/api/v1/auth/logout", handler.Logout)
	r.Post("/api/v1/oauth/token", handler.Token)
	r.Post("/api/v1/auth/forgot-password", handler.ForgotPassword)
	r.Post("/api/v1/auth/reset-password", handler.ResetPassword)
	r.Get("/api/v1/auth/invitation-info", handler.GetInvitationInfo)

	// Protected auth routes
	r.With(jwtMiddleware).Post("/api/v1/auth/accept-invite", handler.AcceptInvite)
}
