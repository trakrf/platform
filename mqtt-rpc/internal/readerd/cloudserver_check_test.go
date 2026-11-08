package readerd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var checkNow = time.Date(2026, 11, 8, 17, 0, 0, 0, time.UTC)

// testCert is a generated certificate and its key, for building bundles.
type testCert struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	der  []byte
}

// newCert issues a CA certificate named cn. A nil parent makes it self-signed.
func newCert(t *testing.T, cn string, parent *testCert, notAfter time.Time) *testCert {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             checkNow.Add(-24 * time.Hour),
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCert{cert: c, key: key, der: der}
}

func pemOf(certs ...*testCert) []byte {
	var b []byte
	for _, c := range certs {
		b = append(b, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.der})...)
	}
	return b
}

// csFixture writes a CloudServer file holding one golden MQTT entry, edited by
// mutate, and a bundle named by it. A nil bundle leaves the named file absent.
func csFixture(t *testing.T, mutate func(map[string]any), bundle []byte) CloudServerSource {
	t.Helper()
	dir := t.TempDir()
	certDir := filepath.Join(dir, "cert")
	const id = "TrakRF mqtt-rpc MQTT Server"
	entry := map[string]any{
		"serverId": id, "type": "MQTT", "path": "broker.example", "port": 8883,
		"clientId": "cs463-x", "username": "u", "password": "p", "topic": "o/cs463-x/reads",
		"enableSSL": true, "sslVersion": "TLSv1.2", "qos": 0, "cleanSession": true,
		"serverCertFile": "bundle.pem",
	}
	if mutate != nil {
		mutate(entry)
	}
	data, err := json.Marshal([]any{entry})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "CloudServer")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if bundle != nil {
		if err := os.MkdirAll(filepath.Join(certDir, id), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(certDir, id, "bundle.pem"), bundle, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return CloudServerSource{File: file, ServerID: id, CertDir: certDir}
}

// wantOneFault asserts exactly one fault, containing every substring.
func wantOneFault(t *testing.T, faults []string, subs ...string) {
	t.Helper()
	if len(faults) != 1 {
		t.Fatalf("faults = %q, want exactly one", faults)
	}
	for _, s := range subs {
		if !strings.Contains(faults[0], s) {
			t.Errorf("fault %q does not mention %q", faults[0], s)
		}
	}
}

func TestCheckCloudServer_GoldenWithAnchorsOnlyBundleIsClean(t *testing.T) {
	x1 := newCert(t, "Root X1", nil, checkNow.AddDate(9, 0, 0))
	x2 := newCert(t, "Root X2", x1, checkNow.AddDate(9, 0, 0)) // a cross-sign may follow the root
	src := csFixture(t, nil, pemOf(x1, x2))
	if faults := CheckCloudServer(src, checkNow); len(faults) != 0 {
		t.Fatalf("faults = %q, want none", faults)
	}
}

func TestCheckCloudServer_IntermediateFirstIsAFault(t *testing.T) {
	// The 2026-08 outage: the bundle led with the leaf's issuing intermediate.
	root := newCert(t, "Root X1", nil, checkNow.AddDate(9, 0, 0))
	ye2 := newCert(t, "YE2", root, checkNow.AddDate(1, 0, 0))
	src := csFixture(t, nil, pemOf(ye2, root))
	wantOneFault(t, CheckCloudServer(src, checkNow), "YE2", "not a self-signed root", "first certificate")
}

func TestCheckCloudServer_MissingBundleIsAFault(t *testing.T) {
	// The silent row: config names a file that is not there. The reader logs nothing.
	src := csFixture(t, nil, nil)
	wantOneFault(t, CheckCloudServer(src, checkNow), "bundle.pem", "missing", "logs nothing")
}

func TestCheckCloudServer_EmptyBundleIsAFault(t *testing.T) {
	src := csFixture(t, nil, []byte("not a pem\n"))
	wantOneFault(t, CheckCloudServer(src, checkNow), "no certificate")
}

func TestCheckCloudServer_ExpiredRootIsAFault(t *testing.T) {
	old := newCert(t, "Old Root", nil, checkNow.Add(-time.Hour))
	src := csFixture(t, nil, pemOf(old))
	wantOneFault(t, CheckCloudServer(src, checkNow), "Old Root", "expired")
}

func TestCheckCloudServer_SessionAndQoSAndTLSDrift(t *testing.T) {
	root := pemOf(newCert(t, "Root X1", nil, checkNow.AddDate(9, 0, 0)))
	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"cleanSession false", func(e map[string]any) { e["cleanSession"] = false }, "cleanSession"},
		{"cleanSession absent", func(e map[string]any) { delete(e, "cleanSession") }, "cleanSession"},
		{"qos 1", func(e map[string]any) { e["qos"] = 1 }, "qos"},
		{"sslVersion TLSv1", func(e map[string]any) { e["sslVersion"] = "TLSv1" }, "TLSv1.2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantOneFault(t, CheckCloudServer(csFixture(t, c.mutate, root), checkNow), c.want)
		})
	}
}

func TestCheckCloudServer_PlaintextBrokerIgnoresStaleCertFile(t *testing.T) {
	// The demo box's local Mosquitto: no TLS, so neither the bundle nor the TLS
	// version matters, even when a stale serverCertFile lingers (as parseCloudServer).
	src := csFixture(t, func(e map[string]any) {
		e["enableSSL"] = false
		e["sslVersion"] = ""
	}, nil)
	if faults := CheckCloudServer(src, checkNow); len(faults) != 0 {
		t.Fatalf("faults = %q, want none", faults)
	}
}

func TestCheckCloudServer_NoCertFileUsesJVMDefaults(t *testing.T) {
	// SSL with no serverCertFile: nothing uploaded, nothing to check.
	src := csFixture(t, func(e map[string]any) { e["serverCertFile"] = "" }, nil)
	if faults := CheckCloudServer(src, checkNow); len(faults) != 0 {
		t.Fatalf("faults = %q, want none", faults)
	}
}

func TestCheckCloudServer_EntryAbsentIsAFault(t *testing.T) {
	src := csFixture(t, nil, nil)
	src.ServerID = "Some Other Server"
	wantOneFault(t, CheckCloudServer(src, checkNow), "no MQTT CloudServer", "Some Other Server")
}

func TestCheckCloudServer_RealLetsEncryptBundles(t *testing.T) {
	// Real public certificates. le-anchors-only.pem is the bundle the Frederick
	// readers were commissioned with (ISRG Root X1, then the X2 and Root YE
	// cross-signs). le-intermediate-first.pem leads with the live YE2
	// intermediate, the shape that cut ingestion in 2026-08.
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		file string
		want string // "" = clean
	}{
		{"le-anchors-only.pem", ""},
		{"le-intermediate-first.pem", `"YE2"`},
	} {
		t.Run(c.file, func(t *testing.T) {
			pemData, err := os.ReadFile(filepath.Join("testdata", c.file))
			if err != nil {
				t.Fatal(err)
			}
			faults := CheckCloudServer(csFixture(t, nil, pemData), now)
			if c.want == "" {
				if len(faults) != 0 {
					t.Fatalf("faults = %q, want none", faults)
				}
				return
			}
			wantOneFault(t, faults, c.want, "not a self-signed root")
		})
	}
}

// Fields read only for the report must never stop the daemon: a firmware that
// writes them with another JSON type gets a fault, and broker discovery still
// succeeds.
func TestCheckCloudServer_OddlyTypedGoldenFieldIsAFaultNotAStartupError(t *testing.T) {
	root := pemOf(newCert(t, "Root X1", nil, checkNow.AddDate(9, 0, 0)))
	src := csFixture(t, func(e map[string]any) { e["qos"] = "0" }, root)
	data, err := os.ReadFile(src.File)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseCloudServer(data, src.ServerID, src.CertDir); err != nil {
		t.Fatalf("parseCloudServer: %v — a report-only field must not fail discovery", err)
	}
	wantOneFault(t, CheckCloudServer(src, checkNow), "qos")
}

func TestCheckCloudServer_UnreadableFileIsAFault(t *testing.T) {
	src := CloudServerSource{File: filepath.Join(t.TempDir(), "absent"), ServerID: "x", CertDir: "/nowhere"}
	wantOneFault(t, CheckCloudServer(src, checkNow), "CloudServer")
}
