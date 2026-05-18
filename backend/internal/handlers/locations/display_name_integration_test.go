//go:build integration
// +build integration

// TRA-778 (BB62-1 F1): Location.name must reject whitespace-only values,
// embedded newlines (\n), carriage returns (\r), and tab characters at the
// validator boundary. description keeps the multi-line-tolerant pattern.

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

type displayNameCase struct {
	label     string
	body      string
	expect4xx bool
}

// POST /api/v1/locations — name validator scenarios.
func TestPostLocation_NameDisplayValidator(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/api/v1/locations", handler.Create)

	cases := []displayNameCase{
		{"whitespace_only_spaces", `{"external_key":"DN-WSP","name":"   "}`, true},
		{"tab_only", `{"external_key":"DN-TAB","name":"\t"}`, true},
		{"newline_only", `{"external_key":"DN-LF","name":"\n"}`, true},
		{"cr_only", `{"external_key":"DN-CR","name":"\r"}`, true},
		{"embedded_newline", `{"external_key":"DN-EMB-LF","name":"line1\nline2"}`, true},
		{"embedded_cr", `{"external_key":"DN-EMB-CR","name":"line1\rline2"}`, true},
		{"embedded_tab", `{"external_key":"DN-EMB-TAB","name":"col1\tcol2"}`, true},
		{"leading_space", `{"external_key":"DN-LEAD","name":" leading"}`, true},
		{"trailing_space", `{"external_key":"DN-TRAIL","name":"trailing "}`, true},

		// Regression — normal names still pass; multi-line description accepted.
		{"single_char", `{"external_key":"DN-SINGLE","name":"X"}`, false},
		{"internal_space", `{"external_key":"DN-INT","name":"Warehouse 1"}`, false},
		{"multiline_description_ok", `{"external_key":"DN-MLD","name":"Warehouse 2","description":"Line 1\nLine 2"}`, false},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/locations", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req = withLocationRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if !tc.expect4xx {
				require.Truef(t, rec.Code == http.StatusCreated || rec.Code == http.StatusOK,
					"%s: expected 2xx, got %d: %s", tc.label, rec.Code, rec.Body.String())
				return
			}

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"%s: expected 400, got %d: %s", tc.label, rec.Code, rec.Body.String())

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
			var nameCode string
			for _, f := range resp.Error.Fields {
				if f.Field == "name" {
					nameCode = f.Code
					break
				}
			}
			assert.Equal(t, "invalid_value", nameCode,
				"%s: expected name=invalid_value, got fields=%+v", tc.label, resp.Error.Fields)
		})
	}
}

// PATCH /api/v1/locations/{id} — same rules on the update surface.
func TestPatchLocation_NameDisplayValidator(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	handler := NewHandler(store)
	router := setupLocationRoundTripRouter(handler)

	cases := []displayNameCase{
		{"whitespace_only", `{"name":"   "}`, true},
		{"embedded_newline", `{"name":"a\nb"}`, true},
		{"embedded_tab", `{"name":"a\tb"}`, true},
		{"trailing_space", `{"name":"a "}`, true},
		{"normal", `{"name":"Renamed Location"}`, false},
		{"single_char", `{"name":"Y"}`, false},
		{"description_multiline_ok", `{"description":"line1\nline2"}`, false},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			id := seedLocationRoundTrip(t, pool, orgID, fmt.Sprintf("LOC-DN-%s", tc.label), "seed")

			req := httptest.NewRequest(http.MethodPatch,
				fmt.Sprintf("/api/v1/locations/%d", id), bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req = withLocationRoundTripOrgContext(req, orgID)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if !tc.expect4xx {
				require.Equalf(t, http.StatusOK, rec.Code,
					"%s: expected 200, got %d: %s", tc.label, rec.Code, rec.Body.String())
				return
			}

			require.Equal(t, http.StatusBadRequest, rec.Code,
				"%s: expected 400, got %d: %s", tc.label, rec.Code, rec.Body.String())
		})
	}
}
