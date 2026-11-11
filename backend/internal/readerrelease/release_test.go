package readerrelease

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/readerrpc"
)

const goodManifest = `{"version":"v1.5.0-300-gabc1234","url":"https://github.com/trakrf/platform/releases/download/mqtt-rpcd-v1.5.0-300-gabc1234/mqtt-rpcd-linux-armv7","sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","signature":"c2ln"}`

func serve(t *testing.T, path, body string, status int) *Source {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Source{BaseURL: srv.URL, HTTP: srv.Client()}
}

func TestFetchReadsTheReleaseManifest(t *testing.T) {
	s := serve(t, "/mqtt-rpcd-v1.5.0-300-gabc1234/manifest.json", goodManifest, http.StatusOK)
	p, err := s.Fetch(context.Background(), "v1.5.0-300-gabc1234")
	require.NoError(t, err)
	assert.Equal(t, readerrpc.DaemonUpdateParams{
		Version:   "v1.5.0-300-gabc1234",
		URL:       "https://github.com/trakrf/platform/releases/download/mqtt-rpcd-v1.5.0-300-gabc1234/mqtt-rpcd-linux-armv7",
		SHA256:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Signature: "c2ln",
	}, p)
}

func TestFetchUnknownVersionIsNotFound(t *testing.T) {
	s := serve(t, "/mqtt-rpcd-v1/manifest.json", goodManifest, http.StatusOK)
	_, err := s.Fetch(context.Background(), "v2")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestFetchRejectsAManifestForAnotherVersion(t *testing.T) {
	s := serve(t, "/mqtt-rpcd-v9/manifest.json", goodManifest, http.StatusOK)
	_, err := s.Fetch(context.Background(), "v9")
	assert.Error(t, err)
}

func TestFetchRejectsIncompleteAndServerErrors(t *testing.T) {
	s := serve(t, "/mqtt-rpcd-v1.5.0-300-gabc1234/manifest.json", `{"version":"v1.5.0-300-gabc1234"}`, http.StatusOK)
	_, err := s.Fetch(context.Background(), "v1.5.0-300-gabc1234")
	assert.Error(t, err)

	s = serve(t, "/mqtt-rpcd-v1/manifest.json", "boom", http.StatusInternalServerError)
	_, err = s.Fetch(context.Background(), "v1")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
}

func TestFetchRejectsAnUnsafeVersionWithoutARequest(t *testing.T) {
	s := &Source{BaseURL: "http://127.0.0.1:1"}
	for _, v := range []string{"", "../x", "a b", "v1?x=1"} {
		_, err := s.Fetch(context.Background(), v)
		assert.ErrorIs(t, err, ErrInvalidVersion, v)
	}
}

func TestNewSourceDefaultsToGitHubReleases(t *testing.T) {
	assert.Equal(t, "https://github.com/trakrf/platform/releases/download", NewSource("").BaseURL)
	assert.Equal(t, "https://example.test/r", NewSource("https://example.test/r/").BaseURL)
}
