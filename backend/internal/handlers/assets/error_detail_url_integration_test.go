//go:build integration
// +build integration

// TRA-739 / BB42 F1: the read-only rejection message for location_id /
// location_external_key on POST and PATCH /api/v1/assets is surfaced
// through the top-level error.detail string. The earlier sanitizer regression
// (module-path scrubber colliding with URL-shaped substrings) collapsed
// docs URLs to "[internal]" in detail; integrators following the documented
// guidance to surface error.detail to humans would render a broken value.
//
// TRA-750 / BB46 F3: the trailing "See https://docs.trakrf.id/..." URL was
// dropped from the error template to avoid an env leak (preview → production
// docs origin). The assertions below invert from TRA-748's "URL must be
// emitted and resolve" to "no docs.trakrf.id URL must appear in the detail
// string for any of the four affected routes" — the error envelope's `type`
// field carries the documented entry point per /docs/api/errors, so the
// inline URL was redundant.

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

const docsHost = "docs.trakrf.id"

func setupErrorDetailURLRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)
	r.Patch("/api/v1/assets/{asset_id}", handler.Update)
	return r
}

func withErrorDetailURLOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra739-detail@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func seedErrorDetailURLAsset(t *testing.T, pool *pgxpool.Pool, orgID int, extKey string) int {
	t.Helper()
	var id int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.assets (org_id, external_key, name, description, valid_from, is_active)
		VALUES ($1, $2, $3, '', $4, true) RETURNING id
	`, orgID, extKey, extKey, time.Now().UTC()).Scan(&id))
	return id
}

type errorDetailResp struct {
	Error struct {
		Type   string `json:"type"`
		Detail string `json:"detail"`
		Fields []struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"fields"`
	} `json:"error"`
}

func TestCreateAsset_LocationID_ErrorDetailHasNoEnvLeakedDocsURL(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupErrorDetailURLRouter(handler)

	body := strings.NewReader(`{"name":"DetailURL Asset","location_id":42}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", body)
	req.Header.Set("Content-Type", "application/json")
	req = withErrorDetailURLOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())

	var resp errorDetailResp
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "location_id", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
	assert.NotContains(t, resp.Error.Fields[0].Message, docsHost,
		"fields[].message must not embed a docs.trakrf.id URL — TRA-750 stripped it to avoid env-leaking preview integrators to prod docs")
	assert.NotContains(t, resp.Error.Detail, docsHost,
		"top-level error.detail must not embed a docs.trakrf.id URL — TRA-750 stripped it to avoid env-leaking preview integrators to prod docs")
	assert.NotContains(t, resp.Error.Detail, "[internal]",
		"the over-aggressive module-path scrubber regression must not leak [internal] into integrator-visible detail")
}

func TestPatchAsset_LocationExternalKey_ErrorDetailHasNoEnvLeakedDocsURL(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedErrorDetailURLAsset(t, pool, orgID, "DETAIL-URL-ASSET")

	handler := NewHandler(store)
	router := setupErrorDetailURLRouter(handler)

	body := strings.NewReader(`{"location_external_key":"WHS-OTHER"}`)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/assets/%d", id), body)
	req.Header.Set("Content-Type", "application/json")
	req = withErrorDetailURLOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())

	var resp errorDetailResp
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "location_external_key", resp.Error.Fields[0].Field)
	assert.Equal(t, "read_only", resp.Error.Fields[0].Code)
	assert.NotContains(t, resp.Error.Fields[0].Message, docsHost,
		"fields[].message must not embed a docs.trakrf.id URL on PATCH — TRA-750 stripped it")
	assert.NotContains(t, resp.Error.Detail, docsHost,
		"top-level error.detail must not embed a docs.trakrf.id URL on PATCH — TRA-750 stripped it")
	assert.NotContains(t, resp.Error.Detail, "[internal]")
}
