//go:build integration
// +build integration

// TRA-739 / BB42 F1: the read-only rejection message for location_id /
// location_external_key on POST and PATCH /api/v1/assets cites
// https://docs.trakrf.id/docs/api/data-model for further reading. The
// fields[].message path carried the substituted URL correctly after
// TRA-734, but the top-level error.detail surfaced as "https://[internal]"
// because the module-path sanitizer (httputil.sanitizeDetail) collided
// with any host/path-shaped substring including legitimate URLs.
// Integrators who follow the documented guidance to surface error.detail
// to humans would render a broken URL.

package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
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

const docsURL = "https://docs.trakrf.id/docs/api/data-model"

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

func TestCreateAsset_LocationID_ErrorDetailPreservesDocsURL(t *testing.T) {
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
	assert.Contains(t, resp.Error.Fields[0].Message, docsURL,
		"fields[].message must cite the docs URL verbatim")
	assert.Contains(t, resp.Error.Detail, docsURL,
		"top-level error.detail must cite the docs URL verbatim — the sanitizer must not collapse it to [internal]")
	assert.NotContains(t, resp.Error.Detail, "[internal]",
		"the over-aggressive module-path scrubber regression must not leak [internal] into integrator-visible detail")
}

func TestPatchAsset_LocationExternalKey_ErrorDetailPreservesDocsURL(t *testing.T) {
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
	assert.Contains(t, resp.Error.Detail, docsURL,
		"top-level error.detail must cite the docs URL verbatim on PATCH as well")
	assert.NotContains(t, resp.Error.Detail, "[internal]")
}

// TRA-748 (BB45 F1): the URL the read_only error envelope cites must
// actually resolve on the docs origin. BB45 caught the service emitting
// https://docs.trakrf.id/api/data-model (404 — missing Docusaurus /docs/
// base path) where the correct URL is https://docs.trakrf.id/docs/api/
// data-model. /docs/api/errors tells integrators the `message` field is
// safe to show end users, so a 404 link surfaced via the error envelope
// is a real contract breakage. This test couples the service's emitted
// URL to the live docs reality across repos: if the docs site relocates
// the page, or the service template literal drifts, this test fails.
func TestCreateAsset_LocationID_EmittedDocsURLResolves(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupErrorDetailURLRouter(handler)

	body := strings.NewReader(`{"name":"DocsURLProbe Asset","location_id":42}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", body)
	req.Header.Set("Content-Type", "application/json")
	req = withErrorDetailURLOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())

	var resp errorDetailResp
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	urlPattern := regexp.MustCompile(`https://[^\s"]+`)
	emitted := urlPattern.FindString(resp.Error.Detail)
	require.NotEmpty(t, emitted, "expected emitted docs URL in error.detail, got %q", resp.Error.Detail)

	client := &http.Client{Timeout: 10 * time.Second}
	getReq, err := http.NewRequest(http.MethodGet, emitted, nil)
	require.NoError(t, err)
	probe, err := client.Do(getReq)
	require.NoError(t, err, "GET %s failed; the docs URL emitted in error envelopes must be reachable from the network this test runs in", emitted)
	defer probe.Body.Close()

	assert.Equal(t, http.StatusOK, probe.StatusCode,
		"service emits %s in error envelopes (treated as customer-safe per /docs/api/errors), but the URL did not return 200 — either the service template literal drifted or the docs page was renamed/removed", emitted)
}
