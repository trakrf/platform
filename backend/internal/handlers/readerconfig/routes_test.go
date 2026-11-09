package readerconfig

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// A reader route can take longer than the server's WriteTimeout: a lost reply
// alone is 10 s after the publish. The response written after that must still
// reach the client. Through a real server, since a recorder has no deadlines.
func TestRegisterRoutes_ReaderRoutesOutlastTheServerWriteTimeout(t *testing.T) {
	const writeTimeout = 100 * time.Millisecond
	slow := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(3 * writeTimeout) // the reader's reply never came
			w.WriteHeader(http.StatusGatewayTimeout)
		})
	}
	r := chi.NewRouter()
	NewHandler(nil, nil).RegisterRoutes(r, slow, slow)
	srv := httptest.NewUnstartedServer(r)
	srv.Config.ReadTimeout = writeTimeout
	srv.Config.WriteTimeout = writeTimeout
	srv.Start()
	defer srv.Close()

	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/scan-devices/1/reader-software/update"},
		{http.MethodPatch, "/api/v1/scan-devices/1/reader-config"},
	} {
		req, err := http.NewRequest(c.method, srv.URL+c.path, bytes.NewBufferString(`{}`))
		require.NoError(t, err)
		resp, err := srv.Client().Do(req)
		require.NoError(t, err, "%s %s: the response was lost to the write timeout", c.method, c.path)
		resp.Body.Close()
		assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode, c.path)
	}
}
