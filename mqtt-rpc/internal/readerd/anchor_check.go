package readerd

// CheckCloudServer can see that the bundle leads with a root, but not that the
// broker's chain still reaches that root. Both have to hold for CSL, which
// trusts only the bundle's first certificate. The broker side can change with
// no change on the reader: a renewal under another intermediate, or the broker
// no longer sending a cross-sign. The daemon connects to the same broker, so on
// every handshake it asks whether the chain the broker presented reaches the
// bundle's first certificate alone. The daemon's own connection trusts the whole
// bundle and never fails over this.

import (
	"crypto/x509"
	"fmt"
	"sync"
	"time"
)

// anchorCheck holds the result of the last handshake's anchor check.
type anchorCheck struct {
	mu    sync.Mutex
	fault string
}

// Fault is "" until a handshake's chain fails to reach the anchor.
func (a *anchorCheck) Fault() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.fault
}

// observe verifies the presented chain (leaf first) against anchor alone.
func (a *anchorCheck) observe(anchor *x509.Certificate, peers []*x509.Certificate, now time.Time) {
	if len(peers) == 0 {
		return
	}
	roots := x509.NewCertPool()
	roots.AddCert(anchor)
	inters := x509.NewCertPool()
	for _, c := range peers[1:] {
		inters.AddCert(c)
	}
	fault := ""
	leaf := peers[0]
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: inters, CurrentTime: now,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		fault = fmt.Sprintf("the broker's certificate %q, issued by %q, does not chain to %q, the only certificate CSL trusts in the bundle: the reader cannot connect (%v)",
			leaf.Subject.CommonName, leaf.Issuer.CommonName, anchor.Subject.CommonName, err)
	}
	a.mu.Lock()
	a.fault = fault
	a.mu.Unlock()
}
