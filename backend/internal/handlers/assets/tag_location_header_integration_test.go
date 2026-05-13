//go:build integration
// +build integration

// TRA-707 / BB32 C2: POST /api/v1/assets/{asset_id}/tags returns 201 with a
// Location header pointing at the newly created tag subresource. The header
// was missing on this endpoint while POST /api/v1/assets emitted one — the
// spec divergence pushed integrators to either parse the JSON body for the
// id or re-issue a list/get to learn it, both of which cost a round-trip.
//
// RFC 7231 §7.1.2 encourages a Location response on 201 Created; we match
// the pattern already used by POST /api/v1/assets.

package assets

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

func setupTagLocationHeaderRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets/{asset_id}/tags", handler.AddTag)
	return r
}

func withTagLocationOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra707@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedTagLocationAsset(t *testing.T, pool *pgxpool.Pool, orgID int, extKey, name string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, extKey, name, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestAddAssetTag_201_EmitsLocationHeader(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	assetID := seedTagLocationAsset(t, pool, orgID, "FORK-707", "Forklift 707")

	handler := NewHandler(store)
	router := setupTagLocationHeaderRouter(handler)

	body := strings.NewReader(`{"tag_type":"rfid","value":"E2-007707"}`)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/assets/%d/tags", assetID), body)
	req.Header.Set("Content-Type", "application/json")
	req = withTagLocationOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var resp struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotZero(t, resp.Data.ID, "response must echo tag id so the Location header can be cross-verified")

	loc := rr.Header().Get("Location")
	wantLoc := fmt.Sprintf("/api/v1/assets/%d/tags/%d", assetID, resp.Data.ID)
	assert.Equal(t, wantLoc, loc,
		"Location header must point at the canonical subresource URL — RFC 7231 §7.1.2")
}
