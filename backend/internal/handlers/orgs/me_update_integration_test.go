//go:build integration
// +build integration

package orgs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/handlers/orgs"
	"github.com/trakrf/platform/backend/internal/middleware"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// newMeRouter wires the /users/me routes the way production does: session auth
// plus the content-type gate, registered through the orgs handler.
func newMeRouter(t *testing.T, store *storage.Storage) *chi.Mux {
	t.Helper()
	pool := store.Pool().(*pgxpool.Pool)
	service := orgsservice.NewService(pool, store, nil)
	handler := orgs.NewHandler(store, service, nil)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.ContentType)
		handler.RegisterMeRoutes(r)
	})
	return r
}

func patchMe(t *testing.T, router *chi.Mux, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TRA-958: the self route takes its user id from the session claims, never
// from the request, so no shape of body edits somebody else's account.
func TestUpdateMe_ChangesOwnName(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-update-me")
	store := testutil.SetupTestDatabase(t)
	pool := store.Pool().(*pgxpool.Pool)

	token := seedSessionUser(t, pool, "self-tra958@example.com", false)
	rec := patchMe(t, newMeRouter(t, store), token, `{"name":"Renamed Operator"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "Renamed Operator", body.Data.Name)
	assert.Equal(t, "self-tra958@example.com", body.Data.Email)
}

func TestUpdateMe_ChangesOwnEmail(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-update-me")
	store := testutil.SetupTestDatabase(t)
	pool := store.Pool().(*pgxpool.Pool)

	token := seedSessionUser(t, pool, "before-tra958@example.com", false)
	rec := patchMe(t, newMeRouter(t, store), token, `{"email":"after-tra958@example.com"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data struct {
			Email string `json:"email"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "after-tra958@example.com", body.Data.Email)
}

func TestUpdateMe_RequiresSession(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-update-me")
	store := testutil.SetupTestDatabase(t)

	rec := patchMe(t, newMeRouter(t, store), "", `{"name":"Nope"}`)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
}

func TestUpdateMe_DuplicateEmailConflicts(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-update-me")
	store := testutil.SetupTestDatabase(t)
	pool := store.Pool().(*pgxpool.Pool)

	seedSessionUser(t, pool, "occupied-tra958@example.com", false)
	token := seedSessionUser(t, pool, "mover2-tra958@example.com", false)

	rec := patchMe(t, newMeRouter(t, store), token, `{"email":"occupied-tra958@example.com"}`)

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}

func TestUpdateMe_RejectsBlankName(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-update-me")
	store := testutil.SetupTestDatabase(t)
	pool := store.Pool().(*pgxpool.Pool)

	token := seedSessionUser(t, pool, "blank-tra958@example.com", false)
	rec := patchMe(t, newMeRouter(t, store), token, `{"name":"   "}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestUpdateMe_RejectsMalformedEmail(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-update-me")
	store := testutil.SetupTestDatabase(t)
	pool := store.Pool().(*pgxpool.Pool)

	token := seedSessionUser(t, pool, "bademail-tra958@example.com", false)
	rec := patchMe(t, newMeRouter(t, store), token, `{"email":"not-an-email"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// An empty patch is a no-op, not an error: the storage layer short-circuits to
// a plain read, and the caller still gets its own profile back.
func TestUpdateMe_EmptyPatchIsNoOp(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-update-me")
	store := testutil.SetupTestDatabase(t)
	pool := store.Pool().(*pgxpool.Pool)

	token := seedSessionUser(t, pool, "noop-tra958@example.com", false)
	rec := patchMe(t, newMeRouter(t, store), token, `{}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data struct {
			Email string `json:"email"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "noop-tra958@example.com", body.Data.Email)
}
