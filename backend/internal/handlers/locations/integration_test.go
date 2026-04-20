//go:build integration
// +build integration

// TRA-404: Create/Update return 409 on duplicate identifier (not 500)

package locations

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func setupTestRouter(handler *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// Public write + identifier routes are registered in cmd/serve/router.go
	// under the public-write group (TRA-397). Wire them here directly so these
	// handler-level tests continue to exercise the same handler paths.
	r.Post("/api/v1/locations", handler.Create)
	r.Put("/api/v1/locations/{id}", handler.Update)
	r.Delete("/api/v1/locations/{id}", handler.Delete)
	r.Post("/api/v1/locations/{id}/identifiers", handler.AddIdentifier)
	r.Delete("/api/v1/locations/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
	handler.RegisterRoutes(r)
	return r
}

func withOrgContext(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "test@test.com", CurrentOrgID: &orgID}
	ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
	return req.WithContext(ctx)
}

func TestCreateLocation_DuplicateIdentifierReturns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "dup-loc",
		Name:       "Existing",
		ValidFrom:  validFrom,
		IsActive:   true,
	})
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	body, err := json.Marshal(locmodel.CreateLocationWithIdentifiersRequest{
		CreateLocationRequest: locmodel.CreateLocationRequest{
			Name:       "Duplicate",
			Identifier: "dup-loc",
			ValidFrom:  shared.FlexibleDate{Time: validFrom},
			IsActive:   true,
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	var resp modelerrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, string(modelerrors.ErrConflict), resp.Error.Type)
}

func TestCreateLocationWithIdentifiers_DuplicateReturns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "dup-loc-ids",
		Name:       "Existing",
		ValidFrom:  validFrom,
		IsActive:   true,
	})
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	body, err := json.Marshal(locmodel.CreateLocationWithIdentifiersRequest{
		CreateLocationRequest: locmodel.CreateLocationRequest{
			Name:       "Duplicate",
			Identifier: "dup-loc-ids",
			ValidFrom:  shared.FlexibleDate{Time: validFrom},
			IsActive:   true,
		},
		Identifiers: []shared.TagIdentifierRequest{
			{Type: "rfid", Value: "E2000000DEADBEEF"},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestUpdateLocation_DuplicateIdentifierReturns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "loc-a",
		Name:       "A",
		ValidFrom:  validFrom,
		IsActive:   true,
	})
	require.NoError(t, err)

	second, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "loc-b",
		Name:       "B",
		ValidFrom:  validFrom,
		IsActive:   true,
	})
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	collide := "loc-a"
	body, err := json.Marshal(locmodel.UpdateLocationRequest{Identifier: &collide})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/"+strconv.Itoa(second.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	var resp modelerrors.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, string(modelerrors.ErrConflict), resp.Error.Type)
	assert.Contains(t, resp.Error.Detail, "already exists")
}
