//go:build integration
// +build integration

// TRA-707 / BB32 C2: POST /api/v1/locations/{location_id}/tags returns 201
// with a Location header pointing at the newly created tag subresource —
// matching the pattern on POST /api/v1/locations. See the asset-side test
// for the full rationale.

package locations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupLocationTagLocationHeaderRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations/{location_id}/tags", handler.AddTag)
	return r
}

func withLocationTagLocationOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra707-loc@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedLocationTagLocationLoc(t *testing.T, pool *pgxpool.Pool, orgID int, extKey, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from)
		VALUES ($1, $2, $3, '', $4) RETURNING id
	`, orgID, extKey, name, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestAddLocationTag_201_EmitsLocationHeader(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	locID := seedLocationTagLocationLoc(t, pool, orgID, "ZONE-707", "Zone 707")

	handler := NewHandler(store)
	router := setupLocationTagLocationHeaderRouter(handler)

	body := strings.NewReader(`{"tag_type":"rfid","value":"E2-707-LOC"}`)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/locations/%d/tags", locID), body)
	req.Header.Set("Content-Type", "application/json")
	req = withLocationTagLocationOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var resp struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotZero(t, resp.Data.ID)

	loc := rr.Header().Get("Location")
	wantLoc := fmt.Sprintf("/api/v1/locations/%d/tags/%d", locID, resp.Data.ID)
	assert.Equal(t, wantLoc, loc,
		"Location header must point at the canonical subresource URL — RFC 7231 §7.1.2")
}
