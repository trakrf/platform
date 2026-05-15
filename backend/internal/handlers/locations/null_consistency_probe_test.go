//go:build integration
// +build integration

// TRA-732 R2 probe: every non-nullable field on locations must emit
// `invalid_value` (not `required` or `too_short`) for explicit JSON null.
// Locations parallel to the assets null-consistency probe.

package locations

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

type locFieldProbe struct {
	field        string
	body         string
	expectedCode string
}

func TestPostLocation_NullOnNonNullable_AllInvalidValue(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	cases := []locFieldProbe{
		{"name", `{"external_key":"NULL-NAME","name":null}`, "invalid_value"},
		{"external_key", `{"external_key":null,"name":"n"}`, "invalid_value"},
		{"valid_from", `{"external_key":"NULL-VF","name":"n","valid_from":null}`, "invalid_value"},
		{"valid_to", `{"external_key":"NULL-VT","name":"n","valid_to":null}`, ""},
		{"is_active", `{"external_key":"NULL-IA","name":"n","is_active":null}`, "invalid_value"},
		{"description", `{"external_key":"NULL-DESC","name":"n","description":null}`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req = withLocationRoundTripOrgContext(req, orgID)
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

func TestPatchLocation_NullOnNonNullable_AllInvalidValue(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	cases := []locFieldProbe{
		{"name", `{"name":null}`, "invalid_value"},
		{"valid_from", `{"valid_from":null}`, "invalid_value"},
		{"is_active", `{"is_active":null}`, "invalid_value"},
		{"valid_to", `{"valid_to":null}`, ""},
		{"description", `{"description":null}`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			id := seedLocationRoundTripWithParent(t, pool, orgID,
				fmt.Sprintf("LOC-PNULL-%s", tc.field), "n", nil)

			req := httptest.NewRequest(http.MethodPatch,
				fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req = withLocationRoundTripOrgContext(req, orgID)
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
