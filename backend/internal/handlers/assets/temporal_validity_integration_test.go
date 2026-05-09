//go:build integration
// +build integration

// TRA-628: Default list scope must apply temporal-validity predicate;
// path-{id} GET must override it.

package assets

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
	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupTemporalRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/assets", handler.ListAssets)
	r.Get("/api/v1/assets/{asset_id}", handler.GetAsset)
	return r
}

func withTemporalOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra628@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

// seedAssetWithWindow inserts an asset with the given temporal window.
// valid_from is NOT NULL in the schema (defaults to CURRENT_TIMESTAMP); valid_to is nullable.
func seedAssetWithWindow(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey string, validFrom time.Time, validTo *time.Time) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, valid_to, is_active)
		VALUES ($1, $2, $2, '', $3, $4, true) RETURNING id
	`, orgID, externalKey, validFrom, validTo).Scan(&id)
	require.NoError(t, err)
	return id
}

type listResp struct {
	Data       []assetmodel.PublicAssetView `json:"data"`
	TotalCount int                          `json:"total_count"`
}

func doListReq(t *testing.T, router *chi.Mux, orgID int, query string) (int, listResp) {
	t.Helper()
	url := "/api/v1/assets"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withTemporalOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return w.Code, listResp{}
	}
	var resp listResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return w.Code, resp
}

func doGetByIDReq(t *testing.T, router *chi.Mux, orgID, id int) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", id), nil)
	req = withTemporalOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

func externalKeysOf(items []assetmodel.PublicAssetView) []string {
	out := make([]string, 0, len(items))
	for _, a := range items {
		out = append(out, a.ExternalKey)
	}
	return out
}

func TestListAssets_TemporalValidity_DefaultScopeExcludesExpiredAndFuture(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)
	weekHence := now.Add(7 * 24 * time.Hour)

	effectiveID := seedAssetWithWindow(t, pool, orgID, "EFFECTIVE", yesterday, nil)
	boundedID := seedAssetWithWindow(t, pool, orgID, "BOUNDED", yesterday, &weekHence)
	expiredID := seedAssetWithWindow(t, pool, orgID, "EXPIRED", weekAgo, &yesterday)
	futureID := seedAssetWithWindow(t, pool, orgID, "FUTURE", tomorrow, &weekHence)

	handler := NewHandler(store)
	router := setupTemporalRouter(handler)

	code, resp := doListReq(t, router, orgID, "")
	require.Equal(t, http.StatusOK, code)
	keys := externalKeysOf(resp.Data)
	assert.Contains(t, keys, "EFFECTIVE")
	assert.Contains(t, keys, "BOUNDED")
	assert.NotContains(t, keys, "EXPIRED")
	assert.NotContains(t, keys, "FUTURE")

	for _, id := range []int{effectiveID, boundedID, expiredID, futureID} {
		assert.Equal(t, http.StatusOK, doGetByIDReq(t, router, orgID, id), "GET by id should ignore predicate (id=%d)", id)
	}

	_, resp = doListReq(t, router, orgID, "external_key=EXPIRED")
	assert.Empty(t, resp.Data, "?external_key=EXPIRED should return no rows")

	_, resp = doListReq(t, router, orgID, "external_key=EFFECTIVE")
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "EFFECTIVE", resp.Data[0].ExternalKey)
}

func seedTagOnAsset(t *testing.T, pool *pgxpool.Pool, orgID, assetID int, tagType, value string, validFrom time.Time, validTo *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO trakrf.tags (org_id, asset_id, type, value, is_active, valid_from, valid_to)
		VALUES ($1, $2, $3, $4, true, $5, $6)
	`, orgID, assetID, tagType, value, validFrom, validTo)
	require.NoError(t, err)
}

func TestGetAsset_TemporalValidity_EmbeddedTagsFilterPredicate(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)

	assetID := seedAssetWithWindow(t, pool, orgID, "TAG-HOST", yesterday, nil)
	seedTagOnAsset(t, pool, orgID, assetID, "rfid", "EFFECTIVE-TAG", yesterday, nil)
	seedTagOnAsset(t, pool, orgID, assetID, "rfid", "EXPIRED-TAG", weekAgo, &yesterday)

	handler := NewHandler(store)
	router := setupTemporalRouter(handler)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/assets/%d", assetID), nil)
	req = withTemporalOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var envelope struct {
		Data assetmodel.PublicAssetView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))

	tagValues := make([]string, 0, len(envelope.Data.Tags))
	for _, tag := range envelope.Data.Tags {
		tagValues = append(tagValues, tag.Value)
	}
	assert.Contains(t, tagValues, "EFFECTIVE-TAG")
	assert.NotContains(t, tagValues, "EXPIRED-TAG", "embedded tags must respect temporal predicate")
}

func TestListAssets_TemporalValidity_IsActiveIndependentOfPredicate(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)

	_, err := pool.Exec(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, 'ACT-TRUE', 'ACT-TRUE', '', $2, true), ($1, 'ACT-FALSE', 'ACT-FALSE', '', $2, false)
	`, orgID, yesterday)
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupTemporalRouter(handler)

	_, resp := doListReq(t, router, orgID, "")
	keys := externalKeysOf(resp.Data)
	assert.Contains(t, keys, "ACT-TRUE")
	assert.Contains(t, keys, "ACT-FALSE")

	_, resp = doListReq(t, router, orgID, "is_active=true")
	keys = externalKeysOf(resp.Data)
	assert.Contains(t, keys, "ACT-TRUE")
	assert.NotContains(t, keys, "ACT-FALSE")

	_, resp = doListReq(t, router, orgID, "is_active=false")
	keys = externalKeysOf(resp.Data)
	assert.NotContains(t, keys, "ACT-TRUE")
	assert.Contains(t, keys, "ACT-FALSE")
}
