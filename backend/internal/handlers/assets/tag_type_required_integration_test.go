//go:build integration
// +build integration

// TRA-739 / BB42 F2: POST /api/v1/assets/{asset_id}/tags must reject a body
// that omits tag_type. The spec's three discriminated subtypes
// (RfidTagRequest / BleTagRequest / BarcodeTagRequest) all mark tag_type as
// required and use it as the oneOf discriminator; the service used to
// silently default an omitted tag_type to rfid, which diverges from
// strict-typed clients (Pydantic, Jackson with FAIL_ON_MISSING_CREATOR_
// PROPERTIES) that reject the same body before the wire. Tightening the
// service to spec also closes the future-variant footgun: a request that
// meant to target a not-yet-implemented kind no longer silently lands on
// RFID.

package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupAssetTagTypeRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets/{asset_id}/tags", handler.AddTag)
	return r
}

func withAssetTagTypeOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra739@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func TestAddAssetTag_OmittedTagType_Returns400Required(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	assetID := seedTagLocationAsset(t, pool, orgID, "FORK-739", "Forklift 739")

	handler := NewHandler(store)
	router := setupAssetTagTypeRouter(handler)

	body := strings.NewReader(`{"value":"E2-739-NO-TYPE"}`)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/assets/%d/tags", assetID), body)
	req.Header.Set("Content-Type", "application/json")
	req = withAssetTagTypeOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

	assert.Equal(t, "validation_error", resp.Error.Type)
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "tag_type", resp.Error.Fields[0].Field)
	assert.Equal(t, "required", resp.Error.Fields[0].Code,
		"omitted tag_type must surface as code=required per the spec's discriminator requirement")
}

func TestAddAssetTag_ExplicitNullTagType_Returns400Required(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	assetID := seedTagLocationAsset(t, pool, orgID, "FORK-739B", "Forklift 739B")

	handler := NewHandler(store)
	router := setupAssetTagTypeRouter(handler)

	body := strings.NewReader(`{"tag_type":null,"value":"E2-739-NULL-TYPE"}`)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/assets/%d/tags", assetID), body)
	req.Header.Set("Content-Type", "application/json")
	req = withAssetTagTypeOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())

	var resp struct {
		Error struct {
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Len(t, resp.Error.Fields, 1)
	assert.Equal(t, "tag_type", resp.Error.Fields[0].Field)
	assert.Equal(t, "required", resp.Error.Fields[0].Code)
}
