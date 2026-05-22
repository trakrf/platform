//go:build integration
// +build integration

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
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupTagConflictRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets/{asset_id}/tags", handler.AddTag)
	return r
}

func withTagConflictOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra806@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func TestAddAssetTag_DuplicateValue_Returns409NamingConflictingEntity(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	assetA := testutil.CreateTestAsset(t, pool, orgID, "AST-A")
	assetB := testutil.CreateTestAsset(t, pool, orgID, "AST-B")

	tagType := "rfid"
	value := "E2000000HANDLER01"
	_, err := store.AddTagToAsset(context.Background(), orgID, assetA.ID,
		shared.TagRequest{TagType: &tagType, Value: value})
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupTagConflictRouter(handler)

	body := strings.NewReader(fmt.Sprintf(`{"tag_type":"rfid","value":%q}`, value))
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/assets/%d/tags", assetB.ID), body)
	req.Header.Set("Content-Type", "application/json")
	req = withTagConflictOrgContext(req, orgID)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())

	var resp struct {
		Error struct {
			Type   string `json:"type"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp.Error.Type)
	assert.Contains(t, resp.Error.Detail, "AST-A",
		"409 detail must name the conflicting asset")
}
