package readerd

import (
	"bytes"
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// A bundle the reader's own app cannot use must not take the daemon down with
// it: the daemon is the only remote path to the reader, and the fault it would
// report is useless if it cannot connect to report it.
func TestBrokerTLS_UnusableBundleFallsBackToSystemRoots(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.pem")
	if err := os.WriteFile(empty, []byte("not a pem\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"missing": filepath.Join(t.TempDir(), "gone.pem"),
		"empty":   empty,
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			log := zerolog.New(&logs)
			cfg := brokerTLS(path, &log, nil)
			if cfg == nil {
				t.Fatal("brokerTLS returned nil")
			}
			if cfg.RootCAs != nil {
				t.Error("RootCAs set; want nil so the system roots are used")
			}
			if cfg.MinVersion != tls.VersionTLS12 {
				t.Errorf("MinVersion = %x, want TLS 1.2", cfg.MinVersion)
			}
			if !strings.Contains(logs.String(), `"level":"error"`) || !strings.Contains(logs.String(), "system roots") {
				t.Errorf("fallback not logged as an error: %s", logs.String())
			}
		})
	}
}

func TestBrokerTLS_UsableBundleIsTrusted(t *testing.T) {
	log := zerolog.Nop()
	cfg := brokerTLS(filepath.Join("testdata", "le-anchors-only.pem"), &log, nil)
	if cfg == nil || cfg.RootCAs == nil {
		t.Fatalf("brokerTLS = %+v, want RootCAs from the bundle", cfg)
	}
}
