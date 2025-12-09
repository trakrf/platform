package middleware

import (
	"context"
	stderrors "errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

const orgRoleKey contextKey = "org_role"

// ErrOrgUserNotFound is returned when a user is not a member of an org
var ErrOrgUserNotFound = stderrors.New("user is not a member of this organization")

// OrgRoleStore defines the storage methods needed by RBAC middleware
type OrgRoleStore interface {
	GetUserOrgRole(ctx context.Context, userID, orgID int) (models.OrgRole, error)
	IsUserSuperadmin(ctx context.Context, userID int) (bool, error)
}

// RequireOrgMember checks that the authenticated user is a member of the org
// specified by the :orgId or :id URL parameter. Sets the user's role in context.
func RequireOrgMember(store OrgRoleStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := GetRequestID(ctx)

			// Get user claims from auth middleware
			claims := GetUserClaims(r)
			if claims == nil {
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Unauthorized", "Authentication required", requestID)
				return
			}

			// Extract org ID from URL - try both :orgId and :id
			orgIDStr := chi.URLParam(r, "orgId")
			if orgIDStr == "" {
				orgIDStr = chi.URLParam(r, "id")
			}
			if orgIDStr == "" {
				httputil.WriteJSONError(w, r, http.StatusBadRequest,
					errors.ErrBadRequest, "Bad Request", "Organization ID required", requestID)
				return
			}

			orgID, err := strconv.Atoi(orgIDStr)
			if err != nil {
				httputil.WriteJSONError(w, r, http.StatusBadRequest,
					errors.ErrBadRequest, "Bad Request", "Invalid organization ID", requestID)
				return
			}

			// Check for superadmin bypass
			isSuperadmin, err := store.IsUserSuperadmin(ctx, claims.UserID)
			if err != nil {
				logger.Get().Error().
					Err(err).
					Int("user_id", claims.UserID).
					Str("request_id", requestID).
					Msg("Failed to check superadmin status")
				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					errors.ErrInternal, "Internal Error", "Failed to check permissions", requestID)
				return
			}

			if isSuperadmin {
				// Log superadmin access for audit
				logger.Get().Warn().
					Int("user_id", claims.UserID).
					Int("org_id", orgID).
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Str("request_id", requestID).
					Msg("Superadmin org access")
				// Grant admin role to superadmin
				ctx = context.WithValue(ctx, orgRoleKey, models.RoleAdmin)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Get user's role in the org
			role, err := store.GetUserOrgRole(ctx, claims.UserID, orgID)
			if err != nil {
				if err.Error() == ErrOrgUserNotFound.Error() {
					logAccessDenied(claims.UserID, orgID, "member", r)
					httputil.WriteJSONError(w, r, http.StatusForbidden,
						errors.ErrUnauthorized, "Forbidden", "You are not a member of this organization", requestID)
					return
				}
				logger.Get().Error().
					Err(err).
					Int("user_id", claims.UserID).
					Int("org_id", orgID).
					Str("request_id", requestID).
					Msg("Failed to get user org role")
				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					errors.ErrInternal, "Internal Error", "Failed to check permissions", requestID)
				return
			}

			// Store role in context for handlers
			ctx = context.WithValue(ctx, orgRoleKey, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireOrgRole checks that the user has at least the specified role
func RequireOrgRole(store OrgRoleStore, minRole models.OrgRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// First ensure user is a member
		memberCheck := RequireOrgMember(store)
		return memberCheck(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := GetRequestID(ctx)
			claims := GetUserClaims(r)

			role, ok := GetOrgRole(ctx)
			if !ok {
				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					errors.ErrInternal, "Internal Error", "Role not found in context", requestID)
				return
			}

			if !role.HasAtLeast(minRole) {
				orgIDStr := chi.URLParam(r, "orgId")
				if orgIDStr == "" {
					orgIDStr = chi.URLParam(r, "id")
				}
				orgID, _ := strconv.Atoi(orgIDStr)
				logAccessDenied(claims.UserID, orgID, minRole.String(), r)

				httputil.WriteJSONError(w, r, http.StatusForbidden,
					errors.ErrUnauthorized, "Forbidden",
					"Insufficient permissions. Required role: "+minRole.String(), requestID)
				return
			}

			next.ServeHTTP(w, r)
		}))
	}
}

// RequireOrgAdmin is a convenience wrapper for RequireOrgRole(store, RoleAdmin)
func RequireOrgAdmin(store OrgRoleStore) func(http.Handler) http.Handler {
	return RequireOrgRole(store, models.RoleAdmin)
}

// RequireOrgManager is a convenience wrapper for RequireOrgRole(store, RoleManager)
func RequireOrgManager(store OrgRoleStore) func(http.Handler) http.Handler {
	return RequireOrgRole(store, models.RoleManager)
}

// RequireOrgOperator is a convenience wrapper for RequireOrgRole(store, RoleOperator)
func RequireOrgOperator(store OrgRoleStore) func(http.Handler) http.Handler {
	return RequireOrgRole(store, models.RoleOperator)
}

// GetOrgRole retrieves the user's org role from context
func GetOrgRole(ctx context.Context) (models.OrgRole, bool) {
	role, ok := ctx.Value(orgRoleKey).(models.OrgRole)
	return role, ok
}

// logAccessDenied logs denied access attempts for audit purposes
func logAccessDenied(userID, orgID int, requiredRole string, r *http.Request) {
	logger.Get().Warn().
		Int("user_id", userID).
		Int("org_id", orgID).
		Str("required_role", requiredRole).
		Str("path", r.URL.Path).
		Str("method", r.Method).
		Str("request_id", GetRequestID(r.Context())).
		Msg("Access denied")
}
