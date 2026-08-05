//go:build integration
// +build integration

package users_test

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
	"github.com/trakrf/platform/backend/internal/handlers/users"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// TRA-1103: the /api/v1/users CRUD surface is superadmin-only back-office.
// Before this ticket it carried no authorization at all — any session could
// enumerate every user in every org and, via PUT, retarget another account's
// email to run the password-reset flow against it. These tests pin both halves:
// a superadmin is served, and every other authenticated principal gets 403.

const usersJWTSecret = "test-secret-users-authz"

// seedUser inserts a user and returns its id plus a session JWT for it.
func seedUser(t *testing.T, pool *pgxpool.Pool, email string, superadmin bool) (int, string) {
	t.Helper()
	var userID int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash, is_superadmin)
        VALUES ($1, $2, 'stub', $3) RETURNING id`,
		email, email, superadmin,
	).Scan(&userID)
	require.NoError(t, err)
	token, err := jwt.Generate(userID, email, nil)
	require.NoError(t, err)
	return userID, token
}

// newUsersRouter wires the users routes the way production does: session auth
// plus the superadmin gate threaded in at registration.
func newUsersRouter(t *testing.T, store *storage.Storage) *chi.Mux {
	t.Helper()
	handler := users.NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)
		r.Use(middleware.ContentType)
		handler.RegisterRoutes(r, middleware.RequireSuperadmin(store))
	})
	return r
}

// call issues a request against the users router with the given session token.
func call(t *testing.T, store *storage.Storage, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Buffer
	if body == "" {
		reader = bytes.NewBuffer(nil)
	} else {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	newUsersRouter(t, store).ServeHTTP(w, req)
	return w
}

// usersRoute is one entry in the five-route CRUD surface, parameterised by the
// id of a victim user so the {id} routes act on a real row.
type usersRoute struct {
	name   string
	method string
	path   func(victimID int) string
	body   string
}

func usersRoutes() []usersRoute {
	return []usersRoute{
		{"list", http.MethodGet, func(int) string { return "/api/v1/users" }, ""},
		{"get", http.MethodGet, func(id int) string { return fmt.Sprintf("/api/v1/users/%d", id) }, ""},
		{"update", http.MethodPut, func(id int) string { return fmt.Sprintf("/api/v1/users/%d", id) },
			`{"name":"Renamed","email":"attacker@example.com"}`},
		{"delete", http.MethodDelete, func(id int) string { return fmt.Sprintf("/api/v1/users/%d", id) }, ""},
	}
}

func TestUsersRoutes_NonSuperadmin403(t *testing.T) {
	t.Setenv("JWT_SECRET", usersJWTSecret)
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	victimID, _ := seedUser(t, pool, "victim@example.com", false)
	_, token := seedUser(t, pool, "regular@example.com", false)

	for _, route := range usersRoutes() {
		t.Run(route.name, func(t *testing.T) {
			w := call(t, store, token, route.method, route.path(victimID), route.body)
			assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		})
	}
}

// An org admin is still not a superadmin: this surface is cross-org back
// office, so org-level privilege must not open it.
func TestUsersRoutes_OrgAdminStill403(t *testing.T) {
	t.Setenv("JWT_SECRET", usersJWTSecret)
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	ctx := context.Background()
	var orgID int
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
         VALUES ('Authz Org', 'authz-org', true) RETURNING id`).Scan(&orgID))

	victimID, _ := seedUser(t, pool, "victim2@example.com", false)
	adminID, token := seedUser(t, pool, "orgadmin@example.com", false)
	_, err := pool.Exec(ctx,
		`INSERT INTO trakrf.org_users (org_id, user_id, role) VALUES ($1, $2, 'admin')`,
		orgID, adminID)
	require.NoError(t, err)

	for _, route := range usersRoutes() {
		t.Run(route.name, func(t *testing.T) {
			w := call(t, store, token, route.method, route.path(victimID), route.body)
			assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		})
	}
}

func TestUsersRoutes_Unauthenticated401(t *testing.T) {
	t.Setenv("JWT_SECRET", usersJWTSecret)
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	victimID, _ := seedUser(t, pool, "victim3@example.com", false)

	for _, route := range usersRoutes() {
		t.Run(route.name, func(t *testing.T) {
			w := call(t, store, "", route.method, route.path(victimID), route.body)
			assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
		})
	}
}

// The authorized principal: a superadmin still gets the full surface, so the
// gate closes the hole without removing the back-office capability.
func TestUsersRoutes_Superadmin(t *testing.T) {
	t.Setenv("JWT_SECRET", usersJWTSecret)
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	targetID, _ := seedUser(t, pool, "target@example.com", false)
	_, token := seedUser(t, pool, "super@example.com", true)

	t.Run("list", func(t *testing.T) {
		w := call(t, store, token, http.MethodGet, "/api/v1/users", "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body struct {
			Data []map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.NotEmpty(t, body.Data)
	})

	t.Run("get", func(t *testing.T) {
		w := call(t, store, token, http.MethodGet, fmt.Sprintf("/api/v1/users/%d", targetID), "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("update", func(t *testing.T) {
		w := call(t, store, token, http.MethodPut, fmt.Sprintf("/api/v1/users/%d", targetID),
			`{"name":"Target Renamed","email":"target-renamed@example.com"}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("delete", func(t *testing.T) {
		w := call(t, store, token, http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", targetID), "")
		require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	})
}

// The takeover chain from the ticket, pinned end to end: a non-superadmin
// aiming PUT /api/v1/users/{id} at someone else's account must not be able to
// move that account's email to an address they control. Asserting the row is
// unchanged matters as much as the status code — a 403 that still wrote would
// pass a status-only test.
func TestUsersUpdate_CannotRetargetAnotherUsersEmail(t *testing.T) {
	t.Setenv("JWT_SECRET", usersJWTSecret)
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	victimID, _ := seedUser(t, pool, "ceo@example.com", true)
	_, token := seedUser(t, pool, "attacker@example.com", false)

	w := call(t, store, token, http.MethodPut, fmt.Sprintf("/api/v1/users/%d", victimID),
		`{"name":"CEO","email":"attacker-inbox@example.com"}`)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	var email string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT email FROM trakrf.users WHERE id = $1`, victimID).Scan(&email))
	assert.Equal(t, "ceo@example.com", email, "victim email must be untouched")
}
