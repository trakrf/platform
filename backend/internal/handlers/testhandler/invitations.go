// Package testhandler provides test-only HTTP endpoints.
// These endpoints are only registered when APP_ENV != "production".
package testhandler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// Handler provides test-only endpoints for E2E testing.
type Handler struct {
	storage *storage.Storage
}

// NewHandler creates a new test handler.
func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// GetInvitationToken generates a new token for an invitation and returns it.
// This enables E2E tests to accept invitations without email.
//
// GET /test/invitations/{id}/token
// Returns: {"token": "abc123..."}
func (h *Handler) GetInvitationToken(w http.ResponseWriter, r *http.Request) {
	inviteID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid invitation ID", http.StatusBadRequest)
		return
	}

	// Verify invitation exists and is still pending
	inv, err := h.storage.GetInvitationByID(r.Context(), inviteID)
	if err != nil {
		http.Error(w, "Failed to get invitation", http.StatusInternalServerError)
		return
	}
	if inv == nil {
		http.Error(w, "Invitation not found", http.StatusNotFound)
		return
	}

	// Generate new token (32 random bytes -> 64-char hex)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}
	rawToken := hex.EncodeToString(tokenBytes)

	// Hash token for storage
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	// Update the invitation with new token and extend expiry
	newExpiry := time.Now().Add(7 * 24 * time.Hour)
	if err := h.storage.UpdateInvitationToken(r.Context(), inviteID, tokenHash, newExpiry); err != nil {
		http.Error(w, "Failed to update token", http.StatusInternalServerError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"token": rawToken})
}

// SentryTest triggers a test panic to verify Sentry integration.
// GET /test/sentry
func (h *Handler) SentryTest(w http.ResponseWriter, r *http.Request) {
	panic("Sentry test panic - this should appear in Sentry dashboard")
}

// SentryCapture explicitly captures an error and flushes to verify Sentry.
// GET /test/sentry-capture
func (h *Handler) SentryCapture(w http.ResponseWriter, r *http.Request) {
	eventID := sentry.CaptureException(errors.New("Sentry test capture - explicit error"))
	sentry.Flush(2 * time.Second)

	if eventID != nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{
			"status":   "captured",
			"event_id": string(*eventID),
		})
	} else {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "sentry_disabled",
		})
	}
}

// RegisterRoutes registers test routes on the given router.
// Should only be called when APP_ENV != "production".
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/test", func(r chi.Router) {
		r.Get("/invitations/{id}/token", h.GetInvitationToken)
		r.Get("/sentry", h.SentryTest)
		r.Get("/sentry-capture", h.SentryCapture)
	})
}
