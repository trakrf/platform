package orgs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// ListInvitations returns pending invitations for an organization.
func (h *Handler) ListInvitations(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	invitations, err := h.service.ListPendingInvitations(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.InvitationListFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": invitations})
}

// CreateInvitation creates an invitation and sends an email.
func (h *Handler) CreateInvitation(w http.ResponseWriter, r *http.Request) {
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

	var req organization.CreateInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.InvitationCreateInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(req); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.InvitationCreateValidation, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	resp, err := h.service.CreateInvitation(r.Context(), orgID, req, claims.UserID)
	if err != nil {
		switch err.Error() {
		case "already_member":
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				fmt.Sprintf(apierrors.InvitationAlreadyMember, req.Email), "", middleware.GetRequestID(r.Context()))
		case "already_pending":
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				fmt.Sprintf(apierrors.InvitationAlreadyPending, req.Email), "", middleware.GetRequestID(r.Context()))
		default:
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
				apierrors.InvitationCreateFailed, "", middleware.GetRequestID(r.Context()))
		}
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": resp})
}

// CancelInvitation cancels a pending invitation.
func (h *Handler) CancelInvitation(w http.ResponseWriter, r *http.Request) {
	inviteID, err := strconv.Atoi(chi.URLParam(r, "inviteId"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.InvitationInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	if err := h.service.CancelInvitation(r.Context(), inviteID); err != nil {
		if err.Error() == "invitation not found or already cancelled/accepted" {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				apierrors.InvitationNotFound, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.InvitationCancelFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"message": "Invitation cancelled"})
}

// ResendInvitation generates a new token and resends the email.
func (h *Handler) ResendInvitation(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	inviteID, err := strconv.Atoi(chi.URLParam(r, "inviteId"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.InvitationInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	newExpiry, err := h.service.ResendInvitation(r.Context(), inviteID, orgID)
	if err != nil {
		if err.Error() == "invitation not found" {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				apierrors.InvitationNotFound, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.InvitationResendFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"message":    "Invitation resent",
		"expires_at": newExpiry,
	})
}
