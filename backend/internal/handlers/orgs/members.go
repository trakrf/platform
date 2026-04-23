package orgs

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// @Summary List members of an organization
// @Tags org-members,internal
// @ID org_members.list
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Success 200 {object} map[string]any "data: []organization.Member"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/members [get]
// ListMembers returns all members of an organization.
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	members, err := h.service.ListMembers(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.MemberListFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": members})
}

// @Summary Update a member's role in an organization
// @Tags org-members,internal
// @ID org_members.update_role
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param userId path int true "User id"
// @Param request body organization.UpdateMemberRoleRequest true "New role"
// @Success 200 {object} map[string]any "message: Role updated"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/members/{userId} [put]
// UpdateMemberRole updates a member's role in an organization.
func (h *Handler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	userID, err := strconv.Atoi(chi.URLParam(r, "userId"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.MemberUpdateInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	var request organization.UpdateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.MemberUpdateInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.MemberUpdateValidationFail, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	role := models.OrgRole(request.Role)
	if !role.IsValid() {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.MemberInvalidRole, "", middleware.GetRequestID(r.Context()))
		return
	}

	err = h.service.UpdateMemberRole(r.Context(), orgID, userID, role)
	if err != nil {
		if err.Error() == "member not found" {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				apierrors.MemberNotFound, "", middleware.GetRequestID(r.Context()))
			return
		}
		if err.Error() == "cannot demote the last admin" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.MemberLastAdmin, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.MemberUpdateFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"message": "Role updated"})
}

// @Summary Remove a member from an organization
// @Tags org-members,internal
// @ID org_members.remove
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param userId path int true "User id"
// @Success 200 {object} map[string]any "message: Member removed"
// @Failure 400 {object} modelerrors.ErrorResponse "Self-removal or last-admin"
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/members/{userId} [delete]
// RemoveMember removes a member from an organization.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", middleware.GetRequestID(r.Context()))
		return
	}

	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	userID, err := strconv.Atoi(chi.URLParam(r, "userId"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.MemberUpdateInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	err = h.service.RemoveMember(r.Context(), orgID, userID, claims.UserID)
	if err != nil {
		if err.Error() == "cannot remove yourself" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.MemberSelfRemoval, "", middleware.GetRequestID(r.Context()))
			return
		}
		if err.Error() == "member not found" {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				apierrors.MemberNotFound, "", middleware.GetRequestID(r.Context()))
			return
		}
		if err.Error() == "cannot remove the last admin" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.MemberLastAdmin, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.MemberRemoveFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"message": "Member removed"})
}
