package readerd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

const testBrokerName = "broker.test"

// newLeaf issues a server certificate for testBrokerName, signed by parent.
func newLeaf(t *testing.T, parent *testCert) *testCert {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: testBrokerName},
		DNSNames:     []string{testBrokerName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent.cert, &key.PublicKey, parent.key)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCert{cert: c, key: key, der: der}
}

// pki is root → intermediate → leaf, all valid now.
func newPKI(t *testing.T) (root, inter, leaf *testCert) {
	t.Helper()
	root = newCert(t, "Test Root", nil, time.Now().AddDate(5, 0, 0))
	inter = newCert(t, "Test Intermediate", root, time.Now().AddDate(1, 0, 0))
	return root, inter, newLeaf(t, inter)
}

func TestAnchorCheck_ChainReachingTheFirstCertificateIsClean(t *testing.T) {
	root, inter, leaf := newPKI(t)
	var a anchorCheck
	a.observe(root.cert, []*x509.Certificate{leaf.cert, inter.cert}, time.Now())
	if f := a.Fault(); f != "" {
		t.Fatalf("fault = %q, want none", f)
	}
}

func TestAnchorCheck_ChainStoppingShortOfTheFirstCertificateIsAFault(t *testing.T) {
	// The broker stops sending the intermediate (or a cross-sign) the anchor
	// needs. The daemon, trusting every certificate in the bundle, still
	// connects; CSL, trusting only the first, cannot.
	root, _, leaf := newPKI(t)
	var a anchorCheck
	a.observe(root.cert, []*x509.Certificate{leaf.cert}, time.Now())
	f := a.Fault()
	for _, want := range []string{testBrokerName, "Test Intermediate", "Test Root", "cannot connect"} {
		if !strings.Contains(f, want) {
			t.Errorf("fault %q does not mention %q", f, want)
		}
	}
}

func TestAnchorCheck_LaterGoodHandshakeClearsTheFault(t *testing.T) {
	root, inter, leaf := newPKI(t)
	var a anchorCheck
	a.observe(root.cert, []*x509.Certificate{leaf.cert}, time.Now())
	a.observe(root.cert, []*x509.Certificate{leaf.cert, inter.cert}, time.Now())
	if f := a.Fault(); f != "" {
		t.Fatalf("fault = %q, want it cleared by the later handshake", f)
	}
}

// handshake runs a real TLS handshake: the server presents chain, the client
// uses brokerTLS on bundle. It returns the client-side handshake error.
func handshake(t *testing.T, bundle []byte, chain []*testCert, a *anchorCheck) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bundle.pem")
	if err := os.WriteFile(path, bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	log := zerolog.Nop()
	cfg := brokerTLS(path, &log, a)
	cfg.ServerName = testBrokerName

	srvCert := tls.Certificate{PrivateKey: chain[0].key}
	for _, c := range chain {
		srvCert.Certificate = append(srvCert.Certificate, c.der)
	}
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()
	go func() {
		_ = tls.Server(srv, &tls.Config{Certificates: []tls.Certificate{srvCert}}).Handshake()
	}()
	_ = cli.SetDeadline(time.Now().Add(5 * time.Second))
	return tls.Client(cli, cfg).Handshake()
}

func TestBrokerTLS_HandshakeRecordsWhetherTheAnchorIsReached(t *testing.T) {
	root, inter, leaf := newPKI(t)

	t.Run("full chain reaches the first certificate", func(t *testing.T) {
		var a anchorCheck
		if err := handshake(t, pemOf(root), []*testCert{leaf, inter}, &a); err != nil {
			t.Fatalf("handshake: %v", err)
		}
		if f := a.Fault(); f != "" {
			t.Fatalf("fault = %q, want none", f)
		}
	})

	t.Run("short chain: the daemon connects, CSL would not", func(t *testing.T) {
		var a anchorCheck
		// The bundle holds the intermediate too, so the daemon's own handshake
		// succeeds; only the first certificate counts for CSL.
		if err := handshake(t, pemOf(root, inter), []*testCert{leaf}, &a); err != nil {
			t.Fatalf("handshake must not fail over the anchor check: %v", err)
		}
		if f := a.Fault(); !strings.Contains(f, "cannot connect") {
			t.Fatalf("fault = %q, want the anchor fault", f)
		}
	})
}
