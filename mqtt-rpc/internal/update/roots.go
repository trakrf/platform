package update

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"

	"golang.org/x/crypto/x509roots/fallback/bundle"
)

// Update downloads trust the reader firmware's CA bundle PLUS the Mozilla roots
// embedded in this binary. The firmware bundle is never refreshed; once it no
// longer validates GitHub, a reader trusting only it could never fetch another
// release, and only a site visit would fix it. Embedded roots ride every update,
// so each update refreshes them.
//
// A union, not a replacement: TLS here buys availability, not integrity — the
// Ed25519 signature and sha256 are the whole check of what gets installed — so
// any root that still works is worth keeping. Scoped to the download client:
// the broker connection keeps its own trust configuration.

// rootsWith returns system plus every embedded root, honouring the bundle's
// distrust-after constraints. system is not modified.
func rootsWith(system *x509.CertPool) *x509.CertPool {
	pool := system.Clone()
	for r := range bundle.Roots() {
		c, err := x509.ParseCertificate(r.Certificate)
		if err != nil {
			continue // one unparseable root must not take the rest down
		}
		if r.Constraint == nil {
			pool.AddCert(c)
		} else {
			pool.AddCertWithConstraint(c, r.Constraint)
		}
	}
	return pool
}

// updateRoots is rootsWith the system pool, or the embedded roots alone when
// the system pool cannot be loaded.
func updateRoots() *x509.CertPool {
	sys, err := x509.SystemCertPool()
	if err != nil || sys == nil {
		sys = x509.NewCertPool()
	}
	return rootsWith(sys)
}

// downloadTransport is http.DefaultTransport's settings with updateRoots.
func downloadTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{RootCAs: updateRoots(), MinVersion: tls.VersionTLS12}
	return t
}
