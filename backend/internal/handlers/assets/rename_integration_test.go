//go:build integration
// +build integration

// TRA-664 / BB26 D7: external_key is immutable on PATCH; the dedicated
// POST /api/v1/assets/{asset_id}/rename operation is the only path that
// can mutate it. The PATCH rejection surfaces as 400 validation_error
// with code=immutable_field and a detail pointing at the rename operation,
// so an integrator hitting the wrong path gets an actionable error
// instead of a silent drop or generic "unknown field" 400.

package assets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func setupRenameAssetRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Patch("/api/v1/assets/{asset_id}", handler.Update)
	r.Post("/api/v1/assets/{asset_id}/rename", handler.Rename)
	return r
}

// PATCH must reject any body that contains `external_key`, value or null,
// with 400 validation_error + code=immutable_field. The detail string
// names the rename operation so an SDK consumer can branch on
// fields[].code and surface a useful message.
func TestPatchAsset_ExternalKeyImmutable_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "AST-IMMUT", "ImmutableAsset")

	handler := NewHandler(store)
	r := setupRenameAssetRouter(handler)

	cases := []struct {
		name string
		body string
	}{
		{"explicit value", `{"external_key":"AST-RENAMED"}`},
		{"explicit null", `{"external_key":null}`},
		{"with other fields", `{"name":"x","external_key":"AST-OTHER"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch,
				fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req = withRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"PATCH with external_key must be 400 (got %d): %s", rec.Code, rec.Body.String())

			var resp struct {
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
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "validation_error", resp.Error.Type)
			require.Len(t, resp.Error.Fields, 1)
			assert.Equal(t, "external_key", resp.Error.Fields[0].Field)
			assert.Equal(t, "immutable_field", resp.Error.Fields[0].Code)
			assert.Contains(t, resp.Error.Detail, "rename",
				"detail must point at the rename operation: %q", resp.Error.Detail)
			assert.Contains(t, resp.Error.Fields[0].Message, "rename",
				"field message must point at the rename operation: %q", resp.Error.Fields[0].Message)
		})
	}
}

// POST /api/v1/assets/{id}/rename with a valid new external_key returns 200
// and the updated AssetView reflects the rename. The DB-side updated_at
// changes; external_key persists across a GET.
func TestRenameAsset_Success(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "AST-OLD", "Renamable")

	handler := NewHandler(store)
	r := setupRenameAssetRouter(handler)

	body := []byte(`{"external_key":"AST-NEW"}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/assets/%d/rename", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "rename must be 200: %s", rec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "AST-NEW", resp.Data["external_key"], "external_key must reflect rename")

	// Persistence check: the row in trakrf.assets really moved.
	var dbExtKey string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT external_key FROM trakrf.assets WHERE id = $1`, id).Scan(&dbExtKey))
	assert.Equal(t, "AST-NEW", dbExtKey)
}

// POST /rename with a duplicate external_key (already in use within the
// org) surfaces as 409 conflict, matching how create handles the same
// uniqueness collision.
func TestRenameAsset_Duplicate_Conflict409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	_ = seedRoundTripAsset(t, pool, orgID, "AST-EXISTS", "ExistingAsset")
	otherID := seedRoundTripAsset(t, pool, orgID, "AST-OTHER", "OtherAsset")

	handler := NewHandler(store)
	r := setupRenameAssetRouter(handler)

	body := []byte(`{"external_key":"AST-EXISTS"}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/assets/%d/rename", otherID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code,
		"duplicate external_key must be 409 (got %d): %s", rec.Code, rec.Body.String())

	var resp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp.Error.Type)
}

// POST /rename with the same value returns 200 idempotently. updated_at
// advances (the SQL UPDATE fires) but external_key stays.
func TestRenameAsset_SameValue_NoOp200(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "AST-SAME", "SameKey")

	handler := NewHandler(store)
	r := setupRenameAssetRouter(handler)

	body := []byte(`{"external_key":"AST-SAME"}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/assets/%d/rename", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withRoundTripOrgContext(req, orgID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"same-value rename must be 200: %s", rec.Body.String())

	var resp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "AST-SAME", resp.Data["external_key"])
}

// POST /rename with a malformed external_key (reserved punctuation, empty)
// returns 400 validation_error with the standard pattern-violation codes —
// the rename operation enforces the same external_key_pattern the create
// path does.
func TestRenameAsset_BadPattern_Rejected400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	id := seedRoundTripAsset(t, pool, orgID, "AST-PAT", "Pattern")

	handler := NewHandler(store)
	r := setupRenameAssetRouter(handler)

	cases := []struct {
		name string
		body string
	}{
		{"space", `{"external_key":"AST WITH SPACE"}`},
		{"empty", `{"external_key":""}`},
		{"missing field", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost,
				fmt.Sprintf("/api/v1/assets/%d/rename", id), bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req = withRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"bad pattern %q must be 400 (got %d): %s", tc.name, rec.Code, rec.Body.String())

			var resp struct {
				Error struct {
					Type   string `json:"type"`
					Fields []struct {
						Field string `json:"field"`
						Code  string `json:"code"`
					} `json:"fields"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "validation_error", resp.Error.Type)
		})
	}
}
