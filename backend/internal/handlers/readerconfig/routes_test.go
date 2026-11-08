package readerconfig

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

// Pushing a daemon update (or a downgrade) restarts reader software across a
// site, so it is an admin action; reading the version is not.
func TestRegisterRoutes_UpdateIsAdminGatedAndReadIsNot(t *testing.T) {
	pass := func(next http.Handler) http.Handler { return next }
	adminDenied := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	}
	r := chi.NewRouter()
	NewHandler(nil, nil).RegisterRoutes(r, pass, adminDenied)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/scan-devices/1/reader-software/update", bytes.NewBufferString(`{"version":"v2"}`)))
	assert.Equal(t, http.StatusForbidden, rr.Code)

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, withOrg(httptest.NewRequest(http.MethodGet, "/api/v1/scan-devices/1/reader-software", nil), 1))
	assert.NotEqual(t, http.StatusForbidden, rr.Code, "reading the version needs no admin")
}
