//go:build integration
// +build integration

// TRA-628: Default list scope must apply temporal-validity predicate;
// path-{id} GET must override it.

package locations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	locationmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupTemporalRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/locations", handler.ListLocations)
	r.Get("/api/v1/locations/{location_id}", handler.GetLocation)
	return r
}

func withTemporalOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra628-loc@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

// seedLocationWithWindow inserts a location with the given temporal window.
// valid_from is NOT NULL; valid_to is nullable.
func seedLocationWithWindow(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string, validFrom time.Time, validTo *time.Time) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, description, valid_from, valid_to, is_active)
		VALUES ($1, $2, $2, '', $3, $4, true) RETURNING id
	`, orgID, externalKey, validFrom, validTo).Scan(&id)
	require.NoError(t, err)
	return id
}

type locListResp struct {
	Data       []locationmodel.PublicLocationView `json:"data"`
	TotalCount int                                `json:"total_count"`
}

func doLocListReq(t *testing.T, router *chi.Mux, orgID int, query string) (int, locListResp) {
	t.Helper()
	url := "/api/v1/locations"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withTemporalOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return w.Code, locListResp{}
	}
	var resp locListResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func doLocGetByIDReq(t *testing.T, router *chi.Mux, orgID, id int) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", id), nil)
	req = withTemporalOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

func locExternalKeysOf(items []locationmodel.PublicLocationView) []string {
	out := make([]string, 0, len(items))
	for _, a := range items {
		out = append(out, a.ExternalKey)
	}
	return out
}

func TestListLocations_TemporalValidity_DefaultScopeExcludesExpiredAndFuture(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)
	weekHence := now.Add(7 * 24 * time.Hour)

	effectiveID := seedLocationWithWindow(t, pool, orgID, "L-EFFECTIVE", yesterday, nil)
	boundedID := seedLocationWithWindow(t, pool, orgID, "L-BOUNDED", yesterday, &weekHence)
	expiredID := seedLocationWithWindow(t, pool, orgID, "L-EXPIRED", weekAgo, &yesterday)
	futureID := seedLocationWithWindow(t, pool, orgID, "L-FUTURE", tomorrow, &weekHence)

	handler := NewHandler(store)
	router := setupTemporalRouter(handler)

	code, resp := doLocListReq(t, router, orgID, "")
	require.Equal(t, http.StatusOK, code)
	keys := locExternalKeysOf(resp.Data)
	assert.Contains(t, keys, "L-EFFECTIVE")
	assert.Contains(t, keys, "L-BOUNDED")
	assert.NotContains(t, keys, "L-EXPIRED")
	assert.NotContains(t, keys, "L-FUTURE")

	for _, id := range []int{effectiveID, boundedID, expiredID, futureID} {
		assert.Equal(t, http.StatusOK, doLocGetByIDReq(t, router, orgID, id), "GET by id should ignore predicate (id=%d)", id)
	}

	_, resp = doLocListReq(t, router, orgID, "external_key=L-EXPIRED")
	assert.Empty(t, resp.Data, "?external_key=L-EXPIRED should return no rows")

	_, resp = doLocListReq(t, router, orgID, "external_key=L-EFFECTIVE")
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "L-EFFECTIVE", resp.Data[0].ExternalKey)
}
