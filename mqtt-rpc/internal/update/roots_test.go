package update

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/x509roots/fallback/bundle"
)

func firstBundleRoot(t *testing.T) *x509.Certificate {
	t.Helper()
	for r := range bundle.Roots() {
		if r.Constraint != nil {
			continue
		}
		c, err := x509.ParseCertificate(r.Certificate)
		require.NoError(t, err)
		return c
	}
	t.Fatal("the embedded bundle has no unconstrained root")
	return nil
}

// The reader firmware's CA bundle goes stale and is never updated. Embedded
// roots ride every update, so a reader whose own bundle no longer validates
// GitHub can still fetch the next release.
func TestRootsWith_TrustsTheEmbeddedBundleWithAnEmptySystemPool(t *testing.T) {
	root := firstBundleRoot(t)
	_, err := root.Verify(x509.VerifyOptions{Roots: rootsWith(x509.NewCertPool())})
	assert.NoError(t, err)
}

// The system pool is kept, not replaced: a root the firmware trusts that the
// bundle lacks must keep working.
func TestRootsWith_KeepsTheSystemPool(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	srvCert := srv.Certificate() // self-signed, so its own root
	sys := x509.NewCertPool()
	sys.AddCert(srvCert)
	_, err := srvCert.Verify(x509.VerifyOptions{Roots: rootsWith(sys)})
	assert.NoError(t, err)
}

func TestNew_DownloadsTrustTheUpdateRoots(t *testing.T) {
	u := New(Layout{Dir: t.TempDir()}, "v1", func() {}, zerolog.Nop())
	tr, ok := u.HTTP.Transport.(*http.Transport)
	require.True(t, ok, "the download client carries its own transport")
	require.NotNil(t, tr.TLSClientConfig)
	require.NotNil(t, tr.TLSClientConfig.RootCAs)
	assert.GreaterOrEqual(t, tr.TLSClientConfig.MinVersion, uint16(tls.VersionTLS12))
	_, err := firstBundleRoot(t).Verify(x509.VerifyOptions{Roots: tr.TLSClientConfig.RootCAs})
	assert.NoError(t, err)
	assert.NotNil(t, tr.Proxy, "proxy from environment, as http.DefaultTransport")
}
