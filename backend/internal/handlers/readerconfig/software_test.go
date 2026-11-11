package readerconfig

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/readerrpc"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// fakeDaemon is the software endpoints' reader seam.
type fakeDaemon struct {
	caps    readerrpc.Capabilities
	capsErr error
	updRes  readerrpc.DaemonUpdateResult
	updErr  error
	got     []readerrpc.DaemonUpdateParams
}

func (f *fakeDaemon) GetCapabilities(context.Context, string) (readerrpc.Capabilities, error) {
	return f.caps, f.capsErr
}
func (f *fakeDaemon) UpdateDaemon(_ context.Context, _ string, p readerrpc.DaemonUpdateParams) (readerrpc.DaemonUpdateResult, error) {
	f.got = append(f.got, p)
	return f.updRes, f.updErr
}

type fakeReleases struct {
	m     readerrpc.DaemonUpdateParams
	err   error
	asked []string
}

func (f *fakeReleases) Fetch(_ context.Context, v string) (readerrpc.DaemonUpdateParams, error) {
	f.asked = append(f.asked, v)
	return f.m, f.err
}

func softwareRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/api/v1/scan-devices/{scan_device_id}/reader-software", h.GetSoftware)
	r.Post("/api/v1/scan-devices/{scan_device_id}/reader-software/update", h.UpdateSoftware)
	return r
}

func withOrg(req *http.Request, orgID int) *http.Request {
	claims := &jwt.Claims{UserID: 1, Email: "software@t.com", CurrentOrgID: &orgID}
	return req.WithContext(context.WithValue(req.Context(), middleware.UserClaimsKey, claims))
}

func TestSoftwareEndpoints_503WithoutReaderControl(t *testing.T) {
	r := softwareRouter(NewHandler(nil, nil))
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/scan-devices/1/reader-software", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/scan-devices/1/reader-software/update", bytes.NewBufferString(`{"version":"v2"}`)),
	} {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, withOrg(req, 1))
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code, req.URL.Path)
	}
}

func TestUpdateSoftware_RejectsABadVersionBeforeAnyLookup(t *testing.T) {
	rel := &fakeReleases{}
	d := &fakeDaemon{}
	r := softwareRouter(NewHandler(nil, nil).WithSoftware(d, rel))
	for _, body := range []string{`{"version":""}`, `{"version":"../x"}`, `{}`, `{"version":"v2","extra":1}`} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scan-devices/1/reader-software/update", bytes.NewBufferString(body))
		r.ServeHTTP(rr, withOrg(req, 1))
		assert.Equal(t, http.StatusBadRequest, rr.Code, body)
	}
	assert.Empty(t, rel.asked)
	assert.Empty(t, d.got)
}
