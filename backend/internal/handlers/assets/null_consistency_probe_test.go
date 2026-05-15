//go:build integration
// +build integration

// TRA-732 R2 probe: every non-nullable field must emit `invalid_value` (not
// `required`) when the client sends explicit JSON null. Companion to BB39 F4
// — the errors-page edit will document `invalid_value` as the canonical code
// for null-on-non-nullable, so the service must actually be uniform.

package assets

import (
	"bytes"
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

type fieldProbe struct {
	field        string
	body         string
	expectedCode string
}

// POST /api/v1/assets — explicit null on every required-or-non-nullable field
// must surface as `invalid_value`, not `required`. `required` is reserved
// for the absent-key case.
func TestPostAsset_NullOnNonNullable_AllInvalidValue(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/assets", handler.Create)

	cases := []fieldProbe{
		{"name", `{"external_key":"NULL-NAME","name":null}`, "invalid_value"},
		{"external_key", `{"external_key":null,"name":"n"}`, "invalid_value"},
		{"valid_from", `{"external_key":"NULL-VF","name":"n","valid_from":null}`, "invalid_value"},
		{"valid_to", `{"external_key":"NULL-VT","name":"n","valid_to":null}`, ""}, // nullable on POST
		{"is_active", `{"external_key":"NULL-IA","name":"n","is_active":null}`, "invalid_value"},
		{"metadata", `{"external_key":"NULL-MD","name":"n","metadata":null}`, "invalid_value"},
		{"description", `{"external_key":"NULL-DESC","name":"n","description":null}`, ""}, // nullable on POST
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req = withRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if tc.expectedCode == "" {
				if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
					return
				}
				t.Fatalf("%s null accepted but POST not 2xx (got %d): %s",
					tc.field, rec.Code, rec.Body.String())
			}

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"%s null must be 400 (got %d): %s", tc.field, rec.Code, rec.Body.String())

			var resp struct {
				Error struct {
					Fields []struct {
						Field string `json:"field"`
						Code  string `json:"code"`
					} `json:"fields"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			var codeForField string
			for _, f := range resp.Error.Fields {
				if f.Field == tc.field {
					codeForField = f.Code
					break
				}
			}
			assert.Equal(t, tc.expectedCode, codeForField,
				"%s null must emit %s, got %s: fields=%+v", tc.field, tc.expectedCode, codeForField, resp.Error.Fields)
		})
	}
}

// PATCH /api/v1/assets/{id} — same uniformity contract on the update surface.
func TestPatchAsset_NullOnNonNullable_AllInvalidValue(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupRoundTripRouter(handler)

	cases := []fieldProbe{
		{"name", `{"name":null}`, "invalid_value"},
		{"valid_from", `{"valid_from":null}`, "invalid_value"},
		{"is_active", `{"is_active":null}`, "invalid_value"},
		{"metadata", `{"metadata":null}`, "invalid_value"},
		{"valid_to", `{"valid_to":null}`, ""},       // nullable: clears
		{"description", `{"description":null}`, ""}, // nullable: clears
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			id := seedRoundTripAsset(t, pool, orgID, fmt.Sprintf("ASSET-PNULL-%s", tc.field), "n")

			req := httptest.NewRequest(http.MethodPatch,
				fmt.Sprintf("/api/v1/assets/%d", id), bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req = withRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if tc.expectedCode == "" {
				require.Equal(t, http.StatusOK, rec.Code,
					"%s null on PATCH must be 200 (nullable clears) (got %d): %s",
					tc.field, rec.Code, rec.Body.String())
				return
			}

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"%s null must be 400 (got %d): %s", tc.field, rec.Code, rec.Body.String())

			var resp struct {
				Error struct {
					Fields []struct {
						Field string `json:"field"`
						Code  string `json:"code"`
					} `json:"fields"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			var codeForField string
			for _, f := range resp.Error.Fields {
				if f.Field == tc.field {
					codeForField = f.Code
					break
				}
			}
			assert.Equal(t, tc.expectedCode, codeForField,
				"%s null must emit %s, got %s: fields=%+v", tc.field, tc.expectedCode, codeForField, resp.Error.Fields)
		})
	}
}
