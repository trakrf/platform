//go:build integration
// +build integration

// TRA-739 / BB42 F2: POST /api/v1/locations/{location_id}/tags must reject a
// body that omits tag_type — see the asset-side test for the full rationale.

package locations

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

func setupLocationTagTypeRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations/{location_id}/tags", handler.AddTag)
	return r
}

func withLocationTagTypeOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "tra739-loc@t.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func TestAddLocationTag_OmittedTagType_Returns400Required(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	locID := seedLocationTagLocationLoc(t, pool, orgID, "ZONE-739", "Zone 739")

	handler := NewHandler(store)
	router := setupLocationTagTypeRouter(handler)

	body := strings.NewReader(`{"value":"E2-739-LOC-NO-TYPE"}`)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/locations/%d/tags", locID), body)
	req.Header.Set("Content-Type", "application/json")
	req = withLocationTagTypeOrgContext(req, orgID)
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
	assert.Equal(t, "required", resp.Error.Fields[0].Code)
}
