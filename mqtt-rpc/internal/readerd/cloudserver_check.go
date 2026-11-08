package readerd

// The golden CloudServer is the one entity the daemon does not own: it is
// hand-crafted at commissioning (broker secret + trust bundle) and the reader's
// own CSL app publishes reads through it. Every setting below has, at least
// once, stopped a CS463 publishing while the daemon itself stayed connected, so
// nothing but this check could see it. The daemon reports; it never rewrites the
// entry and never exits over it.

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// goldenSSLVersion is the CS463's TLS ceiling, not a preference: its selector
// offers nothing newer. A broker that requires TLS 1.3 cuts off every CS463.
const goldenSSLVersion = "TLSv1.2"

// CloudServerSource locates the reader's CloudServer config and the per-server
// certificate directory, as LoadConfig auto-discovered them.
type CloudServerSource struct {
	File     string
	ServerID string
	CertDir  string
}

// CheckCloudServer reports every way the golden CloudServer entry would stop
// the reader's own app publishing reads. No faults means nothing was found.
func CheckCloudServer(src CloudServerSource, now time.Time) []string {
	data, err := os.ReadFile(src.File)
	if err != nil {
		return []string{fmt.Sprintf("cannot read the CloudServer config: %v", err)}
	}
	var entries []cloudServerEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return []string{fmt.Sprintf("cannot parse the CloudServer config: %v", err)}
	}
	var e *cloudServerEntry
	for i := range entries {
		if entries[i].ServerID == src.ServerID && entries[i].Type == "MQTT" {
			e = &entries[i]
			break
		}
	}
	if e == nil {
		return []string{fmt.Sprintf("no MQTT CloudServer %q in the CloudServer config", src.ServerID)}
	}

	var faults []string
	if !e.CleanSession {
		faults = append(faults, "cleanSession is false; golden is true")
	}
	if e.QoS != 0 {
		faults = append(faults, fmt.Sprintf("qos is %d; golden is 0", e.QoS))
	}
	if !e.EnableSSL {
		return faults // plaintext broker: no TLS, so no bundle and no TLS version
	}
	if e.SSLVersion != goldenSSLVersion {
		faults = append(faults, fmt.Sprintf("sslVersion is %q; golden is %q, the CS463's ceiling", e.SSLVersion, goldenSSLVersion))
	}
	if e.ServerCertFile != "" {
		faults = append(faults, checkTrustBundle(filepath.Join(src.CertDir, e.ServerID, e.ServerCertFile), now)...)
	}
	return faults
}

// checkTrustBundle judges the bundle the way CSL reads it: CSL trusts the FIRST
// certificate in the file and nothing else. That certificate must be a
// long-lived self-signed root. An intermediate there works until the broker's
// leaf is reissued under a sibling intermediate, and then every handshake fails.
func checkTrustBundle(path string, now time.Time) []string {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{fmt.Sprintf("trust bundle %s is named by the CloudServer but missing: the reader publishes no reads and logs nothing", path)}
	}
	if err != nil {
		return []string{fmt.Sprintf("cannot read trust bundle %s: %v", path, err)}
	}
	first := firstCertificate(data)
	if first == nil {
		return []string{fmt.Sprintf("no certificate in trust bundle %s: the reader cannot verify the broker", path)}
	}
	name := first.Subject.CommonName
	if !bytes.Equal(first.RawSubject, first.RawIssuer) || first.CheckSignatureFrom(first) != nil {
		return []string{fmt.Sprintf("first certificate in trust bundle %s is %q, issued by %q, not a self-signed root: CSL trusts only the first certificate, so reads stop when the broker's certificate moves to another intermediate", path, name, first.Issuer.CommonName)}
	}
	if now.After(first.NotAfter) {
		return []string{fmt.Sprintf("first certificate in trust bundle %s, %q, expired %s", path, name, first.NotAfter.UTC().Format(time.RFC3339))}
	}
	if now.Before(first.NotBefore) {
		return []string{fmt.Sprintf("first certificate in trust bundle %s, %q, is not valid until %s (is the reader clock right?)", path, name, first.NotBefore.UTC().Format(time.RFC3339))}
	}
	return nil
}

// firstCertificate returns the first parseable CERTIFICATE block, or nil.
func firstCertificate(data []byte) *x509.Certificate {
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return nil
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		if c, err := x509.ParseCertificate(block.Bytes); err == nil {
			return c
		}
		return nil // CSL would choke on a corrupt first certificate too
	}
}

// CloudServerFault joins CheckCloudServer's faults for the retained status. A
// nil source (the broker was configured directly, not discovered) checks nothing.
func CloudServerFault(src *CloudServerSource, now time.Time) string {
	if src == nil {
		return ""
	}
	return strings.Join(CheckCloudServer(*src, now), "; ")
}
