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
	r.Put("/api/v1/locations/{identifier}", handler.Update)
	r.Delete("/api/v1/locations/{identifier}", handler.Delete)
	r.Post("/api/v1/locations/{identifier}/identifiers", handler.AddIdentifier)
	r.Delete("/api/v1/locations/{identifier}/identifiers/{identifierId}", handler.RemoveIdentifier)
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

	vfFD := shared.FlexibleDate{Time: validFrom}
	active := true
	body, err := json.Marshal(locmodel.CreateLocationWithIdentifiersRequest{
		CreateLocationRequest: locmodel.CreateLocationRequest{
			Name:       "Duplicate",
			Identifier: "dup-loc",
			ValidFrom:  &vfFD,
			IsActive:   &active,
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

	vfFD2 := shared.FlexibleDate{Time: validFrom}
	active2 := true
	body, err := json.Marshal(locmodel.CreateLocationWithIdentifiersRequest{
		CreateLocationRequest: locmodel.CreateLocationRequest{
			Name:       "Duplicate",
			Identifier: "dup-loc-ids",
			ValidFrom:  &vfFD2,
			IsActive:   &active2,
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

	_, err = store.CreateLocation(context.Background(), locmodel.Location{
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

	req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/loc-b", bytes.NewReader(body))
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

// TRA-407 item 1 — POST /locations with duplicate identifier → 409, not 500.
func TestLocationsCreate_DuplicateIdentifier_Returns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "tra407-dup-loc",
		Name:       "First",
		ValidFrom:  validFrom,
		IsActive:   true,
	})
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	vfFD3 := shared.FlexibleDate{Time: validFrom}
	active3 := true
	body, err := json.Marshal(locmodel.CreateLocationWithIdentifiersRequest{
		CreateLocationRequest: locmodel.CreateLocationRequest{
			Name:       "Second",
			Identifier: "tra407-dup-loc",
			ValidFrom:  &vfFD3,
			IsActive:   &active3,
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
	assert.Equal(t, "conflict", resp.Error.Type)
}

// TRA-482: POST /api/v1/locations/{identifier}/identifiers with a tag value
// already attached to a different location must return 409 Conflict.
// Mirror of the assets test. See the assets-side comment for schema context.
func TestLocationsAddIdentifier_DuplicateValue_Returns409(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra482-loc-host", Name: "Host",
		ValidFrom: validFrom, IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "tra482-loc-host2", Name: "Host2",
		ValidFrom: validFrom, IsActive: true,
	})
	require.NoError(t, err)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	// Attach an identifier to loc host2 via the handler.
	body := `{"type":"rfid","value":"TRA-482-LOC-IDENT-DUP"}`
	reqHost2 := httptest.NewRequest(http.MethodPost, "/api/v1/locations/tra482-loc-host2/identifiers", bytes.NewBufferString(body))
	reqHost2.Header.Set("Content-Type", "application/json")
	reqHost2 = withOrgContext(reqHost2, orgID)
	wHost2 := httptest.NewRecorder()
	router.ServeHTTP(wHost2, reqHost2)
	require.Equal(t, http.StatusCreated, wHost2.Code, wHost2.Body.String())

	// Act: attach same value to the first location.
	reqHost := httptest.NewRequest(http.MethodPost, "/api/v1/locations/tra482-loc-host/identifiers", bytes.NewBufferString(body))
	reqHost.Header.Set("Content-Type", "application/json")
	reqHost = withOrgContext(reqHost, orgID)
	wHost := httptest.NewRecorder()
	router.ServeHTTP(wHost, reqHost)

	require.Equal(t, http.StatusConflict, wHost.Code, wHost.Body.String())
	var resp map[string]map[string]any
	require.NoError(t, json.Unmarshal(wHost.Body.Bytes(), &resp))
	assert.Equal(t, "conflict", resp["error"]["type"])
}

// TRA-407 item 2 — POST /locations with bad body returns fields[] envelope.
func TestLocationsCreate_BadBody_FieldsEnvelope(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	// Act: POST /api/v1/locations with a body missing required fields (empty body "{}").
	req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	req = withOrgContext(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert: 400, body.error.type == "validation_error", body.error.fields populated
	// with snake_case names and mapped codes (e.g. "required").
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var resp map[string]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp["error"]["type"])
	fields, ok := resp["error"]["fields"].([]any)
	require.True(t, ok, "fields should be an array, got: %v", resp["error"]["fields"])
	require.NotEmpty(t, fields, "fields should be non-empty")
	// Verify at least one field has snake_case name and "required" code.
	firstField := fields[0].(map[string]any)
	assert.Equal(t, "required", firstField["code"], "field code should be 'required'")
}

// TestLocationWriteResponses_OmitInternalFields defends the public contract:
// POST and PUT responses MUST NOT contain "id", "org_id", or "parent_location_id"
// keys (TRA-429). If this test breaks, either the handler regressed or the shape
// definition did.
//
// Decoding into map[string]any deliberately bypasses the typed PublicLocationView
// decoder so that leaks of unknown internal fields show up in the assertion rather
// than silently being dropped.
func TestLocationWriteResponses_OmitInternalFields(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupTestRouter(handler)

	// Seed a parent location so we can exercise the Parent path.
	parent, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgID,
		Identifier: "tra429-parent-loc",
		Name:       "TRA-429 Parent",
		Path:       "tra429-parent-loc",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	// assertNoLeaks checks a single write response body for the forbidden
	// internal fields and confirms the public surrogate_id is present+non-zero.
	assertNoLeaks := func(t *testing.T, raw []byte) map[string]any {
		t.Helper()
		var envelope map[string]any
		require.NoError(t, json.Unmarshal(raw, &envelope))

		data, ok := envelope["data"].(map[string]any)
		require.True(t, ok, "data must be an object; got: %v", envelope["data"])

		// Forbidden internal fields — these MUST NOT appear on the wire.
		assert.NotContains(t, data, "id", "response leaks internal surrogate id as 'id'")
		assert.NotContains(t, data, "org_id", "response leaks org_id")
		assert.NotContains(t, data, "parent_location_id", "response leaks raw FK parent_location_id")

		// Required public fields.
		require.Contains(t, data, "surrogate_id", "response missing surrogate_id")
		surrID, ok := data["surrogate_id"].(float64)
		require.True(t, ok, "surrogate_id must be numeric; got: %T", data["surrogate_id"])
		assert.Greater(t, surrID, float64(0), "surrogate_id must be non-zero")

		return data
	}

	validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("POST_NoParent", func(t *testing.T) {
		vfFD4 := shared.FlexibleDate{Time: validFrom}
		active4 := true
		reqBody := locmodel.CreateLocationWithIdentifiersRequest{
			CreateLocationRequest: locmodel.CreateLocationRequest{
				Name:       "TRA-429 Leak Guard",
				Identifier: "tra429-no-parent",
				ValidFrom:  &vfFD4,
				IsActive:   &active4,
			},
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withOrgContext(req, orgID)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		data := assertNoLeaks(t, w.Body.Bytes())
		assert.NotContains(t, data, "parent", "parent must be omitted entirely when no parent_location_id")
	})

	t.Run("POST_WithParent", func(t *testing.T) {
		vfFD5 := shared.FlexibleDate{Time: validFrom}
		active5 := true
		reqBody := locmodel.CreateLocationWithIdentifiersRequest{
			CreateLocationRequest: locmodel.CreateLocationRequest{
				Name:             "TRA-429 With Parent",
				Identifier:       "tra429-with-parent",
				ParentLocationID: &parent.ID,
				ValidFrom:        &vfFD5,
				IsActive:         &active5,
			},
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withOrgContext(req, orgID)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		data := assertNoLeaks(t, w.Body.Bytes())

		// When a parent is present, the public shape exposes it as the parent's
		// natural key under "parent".
		assert.Equal(t, "tra429-parent-loc", data["parent"],
			"parent must be the parent's natural identifier")
	})

	t.Run("PUT_Update", func(t *testing.T) {
		// Seed a location to update.
		_, err := store.CreateLocation(context.Background(), locmodel.Location{
			OrgID:      orgID,
			Identifier: "tra429-update-target",
			Name:       "Before",
			Path:       "tra429-update-target",
			ValidFrom:  time.Now(),
			IsActive:   true,
		})
		require.NoError(t, err)

		newName := "After"
		body, err := json.Marshal(locmodel.UpdateLocationRequest{Name: &newName})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/locations/tra429-update-target", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withOrgContext(req, orgID)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		data := assertNoLeaks(t, w.Body.Bytes())
		assert.Equal(t, "After", data["name"])
	})
}
