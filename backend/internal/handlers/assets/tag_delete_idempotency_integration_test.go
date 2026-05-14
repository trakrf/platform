//go:build integration
// +build integration

// TRA-719 / BB35 A3: tag subresource DELETE returns 404 on second call,
// matching top-level resource DELETE semantics (docs/api/errors documents
// this universal contract). Previously the tag subresource returned 204
// regardless — the divergence was unintentional and surfaced as an
// integrator-facing contract mismatch in BB35.

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
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupTagDeleteIdempotencyRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Delete("/api/v1/assets/{asset_id}/tags/{tag_id}", handler.RemoveTag)
	return r
}

func withTagDeleteIdempotencyOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra719@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedTagDeleteIdempAsset(t *testing.T, pool *pgxpool.Pool, orgID int, extKey string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, extKey, extKey, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedTagDeleteIdempTag(t *testing.T, pool *pgxpool.Pool, orgID, assetID int, value string) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.tags (org_id, asset_id, type, value, is_active, valid_from)
		VALUES ($1, $2, 'rfid', $3, true, $4) RETURNING id
	`, orgID, assetID, value, time.Now().UTC()).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestRemoveAssetTag_FirstCall_204_SecondCall_404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	assetID := seedTagDeleteIdempAsset(t, pool, orgID, "TRA719-A3-ASSET")
	tagID := seedTagDeleteIdempTag(t, pool, orgID, assetID, "TRA719-A3-VALUE")

	handler := NewHandler(store)
	router := setupTagDeleteIdempotencyRouter(handler)

	url := fmt.Sprintf("/api/v1/assets/%d/tags/%d", assetID, tagID)

	// First call: 204
	req1 := httptest.NewRequest(http.MethodDelete, url, nil)
	req1 = withTagDeleteIdempotencyOrgContext(req1, orgID)
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)
	require.Equal(t, http.StatusNoContent, rr1.Code, rr1.Body.String())

	// Second call: 404 (TRA-719 A3 — was 204 before)
	req2 := httptest.NewRequest(http.MethodDelete, url, nil)
	req2 = withTagDeleteIdempotencyOrgContext(req2, orgID)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	require.Equal(t, http.StatusNotFound, rr2.Code, rr2.Body.String())

	var envelope struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rr2.Body.Bytes(), &envelope))
	assert.Equal(t, "not_found", envelope.Error.Type,
		"second-call DELETE must emit the standard not_found error type")
}

func TestRemoveAssetTag_NonExistentTag_404(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	assetID := seedTagDeleteIdempAsset(t, pool, orgID, "TRA719-A3-ASSET-NEXIST")

	handler := NewHandler(store)
	router := setupTagDeleteIdempotencyRouter(handler)

	// A tag id that has no row at all: 404 (same as second-call DELETE).
	url := fmt.Sprintf("/api/v1/assets/%d/tags/%d", assetID, 999_999_999)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req = withTagDeleteIdempotencyOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
}
