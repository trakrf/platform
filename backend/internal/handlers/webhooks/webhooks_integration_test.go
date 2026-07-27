//go:build integration

package webhooks_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/handlers/webhooks"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
	"github.com/trakrf/platform/backend/internal/webhook"
)

// passThrough stands in for paidGate/adminGate. Entitlement and role checks are
// shared middleware with their own tests; these exercise handler logic.
func passThrough(next http.Handler) http.Handler { return next }

func withOrg(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra1043@t.com", CurrentOrgID: &orgID}
	return req.WithContext(context.WithValue(req.Context(), middleware.UserClaimsKey, claims))
}

func newTestServer(t *testing.T) (*chi.Mux, int) {
	t.Helper()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// allowPrivateTargets=true so httptest endpoints on 127.0.0.1 are reachable,
	// matching what APP_ENV=test gives the real wiring.
	webhooks.NewHandler(db.Store, webhook.NewClient(true)).RegisterRoutes(r, passThrough, passThrough)
	return r, orgID
}

func do(t *testing.T, r *chi.Mux, orgID int, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withOrg(req, orgID))
	return rec
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), rec.Body.String())
	return env.Data
}

func TestWebhooksHandler_RoundTrip(t *testing.T) {
	r, orgID := newTestServer(t)

	// Empty to start.
	rec := do(t, r, orgID, http.MethodGet, "/api/v1/webhooks", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Empty(t, list.Data)

	// Create.
	rec = do(t, r, orgID, http.MethodPost, "/api/v1/webhooks", map[string]any{
		"url": "https://example.com/trakrf/hooks",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	created := decodeData(t, rec)
	require.Equal(t, "https://example.com/trakrf/hooks", created["url"])
	require.Equal(t, true, created["enabled"])
	secret, _ := created["secret"].(string)
	require.True(t, strings.HasPrefix(secret, "whsec_"))
	require.Len(t, secret, len("whsec_")+64, "the create response carries the full cleartext secret")
	require.NotEmpty(t, rec.Header().Get("Location"))

	id := int(created["id"].(float64))

	// The secret is never readable again.
	rec = do(t, r, orgID, http.MethodGet, "/api/v1/webhooks", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	require.NotEqual(t, secret, list.Data[0]["secret"])
	require.Contains(t, list.Data[0]["secret"], "…")
	require.NotContains(t, rec.Body.String(), secret)

	rec = do(t, r, orgID, http.MethodGet, "/api/v1/webhooks/"+strconv.Itoa(id), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), secret)

	// Update url + enabled.
	rec = do(t, r, orgID, http.MethodPatch, "/api/v1/webhooks/"+strconv.Itoa(id), map[string]any{
		"url": "https://example.com/v2", "enabled": false,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	updated := decodeData(t, rec)
	require.Equal(t, "https://example.com/v2", updated["url"])
	require.Equal(t, false, updated["enabled"])

	// Delete.
	rec = do(t, r, orgID, http.MethodDelete, "/api/v1/webhooks/"+strconv.Itoa(id), nil)
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = do(t, r, orgID, http.MethodDelete, "/api/v1/webhooks/"+strconv.Itoa(id), nil)
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec = do(t, r, orgID, http.MethodGet, "/api/v1/webhooks", nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Empty(t, list.Data)
}

func TestSecondCreateConflicts(t *testing.T) {
	r, orgID := newTestServer(t)

	rec := do(t, r, orgID, http.MethodPost, "/api/v1/webhooks", map[string]any{"url": "https://example.com/a"})
	require.Equal(t, http.StatusCreated, rec.Code)

	rec = do(t, r, orgID, http.MethodPost, "/api/v1/webhooks", map[string]any{"url": "https://example.com/b"})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}

func TestCreateRejectsBadURL(t *testing.T) {
	r, orgID := newTestServer(t)

	for _, bad := range []string{"", "not a url", "ftp://example.com/x", "/relative"} {
		rec := do(t, r, orgID, http.MethodPost, "/api/v1/webhooks", map[string]any{"url": bad})
		require.Equal(t, http.StatusBadRequest, rec.Code, "url %q must be rejected: %s", bad, rec.Body.String())
	}
}

// Rotation is TRA-398 Phase 2. Silently ignoring a `secret` field would leave
// the caller believing their secret changed.
func TestUpdateRejectsSecretField(t *testing.T) {
	r, orgID := newTestServer(t)
	rec := do(t, r, orgID, http.MethodPost, "/api/v1/webhooks", map[string]any{"url": "https://example.com/a"})
	require.Equal(t, http.StatusCreated, rec.Code)
	id := int(decodeData(t, rec)["id"].(float64))

	rec = do(t, r, orgID, http.MethodPatch, "/api/v1/webhooks/"+strconv.Itoa(id), map[string]any{"secret": "whsec_mine"})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestGetUnknownIDIs404(t *testing.T) {
	r, orgID := newTestServer(t)
	rec := do(t, r, orgID, http.MethodGet, "/api/v1/webhooks/99999999", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTestFireReportsStatusCode(t *testing.T) {
	r, orgID := newTestServer(t)

	var gotEvent, gotSignature string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotEvent = req.Header.Get("X-TrakRF-Event")
		gotSignature = req.Header.Get("X-TrakRF-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rec := do(t, r, orgID, http.MethodPost, "/api/v1/webhooks", map[string]any{"url": srv.URL})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	id := int(decodeData(t, rec)["id"].(float64))

	rec = do(t, r, orgID, http.MethodPost, "/api/v1/webhooks/"+strconv.Itoa(id)+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	result := decodeData(t, rec)
	require.Equal(t, float64(http.StatusOK), result["status_code"])
	require.NotContains(t, result, "error")

	require.Equal(t, "asset.moved", gotEvent)
	require.True(t, strings.HasPrefix(gotSignature, "sha256="))
}

// A failed delivery is a successful diagnostic: the operator needs to see what
// their endpoint said, not a generic API error.
func TestTestFireReportsDeliveryFailure(t *testing.T) {
	r, orgID := newTestServer(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	rec := do(t, r, orgID, http.MethodPost, "/api/v1/webhooks", map[string]any{"url": srv.URL})
	require.Equal(t, http.StatusCreated, rec.Code)
	id := int(decodeData(t, rec)["id"].(float64))

	rec = do(t, r, orgID, http.MethodPost, "/api/v1/webhooks/"+strconv.Itoa(id)+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	result := decodeData(t, rec)
	require.Equal(t, float64(http.StatusBadGateway), result["status_code"])
	require.NotEmpty(t, result["error"])
}

// A disabled webhook can still be test-fired: that is how an operator validates
// an endpoint before switching delivery on.
func TestTestFireWorksWhileDisabled(t *testing.T) {
	r, orgID := newTestServer(t)

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rec := do(t, r, orgID, http.MethodPost, "/api/v1/webhooks", map[string]any{"url": srv.URL, "enabled": false})
	require.Equal(t, http.StatusCreated, rec.Code)
	id := int(decodeData(t, rec)["id"].(float64))

	rec = do(t, r, orgID, http.MethodPost, "/api/v1/webhooks/"+strconv.Itoa(id)+"/test", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, hits)
}
