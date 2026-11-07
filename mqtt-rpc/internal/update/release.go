// Package update is mqtt-rpcd's remote self-update: verify a signed release,
// download it, swap the active-version symlink, and commit or be rolled back.
//
// The rollback itself is NOT in this package: it lives in the mqtt-rpcd-guard
// shell script run by systemd, because a broken new binary cannot be trusted to
// roll itself back. This package and that script share the on-disk state format
// in state.go.
//
// Nothing here talks to the reader's vendor /API or reads mqtt-rpcd.env: an update
// has to work on a reader whose vendor app is wedged, which is exactly when a fix
// most needs pushing.
package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// TrustedKey is one Ed25519 public key releases may be signed by.
type TrustedKey struct {
	ID  string
	Key ed25519.PublicKey
}

var (
	versionRE = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)
	sha256RE  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidVersion reports whether v is safe to use as a version. Versions become
// file names (versions/mqtt-rpcd-<v>) and values in the state files the shell
// guard parses, so the alphabet is deliberately narrow.
func ValidVersion(v string) bool { return versionRE.MatchString(v) && v != "." && v != ".." }

// SignedMessage is the exact byte string a release signature covers. It binds the
// version to the binary's hash, so a genuinely signed build cannot be presented
// under another version. The first line versions the format itself.
func SignedMessage(version, sha256hex string) []byte {
	return []byte("trakrf-mqtt-rpcd-release-v1\nversion=" + version + "\nsha256=" + sha256hex + "\n")
}

// Sign returns the base64 signature of a release. Used by the release tooling
// (cmd/mqtt-rpcctl, CI) and by tests.
func Sign(priv ed25519.PrivateKey, version, sha256hex string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, SignedMessage(version, sha256hex)))
}

// Verify checks p's signature against keys. It checks only what can be checked
// before downloading: the version and hash formats and the signature over them.
// The downloaded bytes are hashed against p.SHA256 separately.
func Verify(keys []TrustedKey, p readerrpc.DaemonUpdateParams) error {
	if !ValidVersion(p.Version) {
		return fmt.Errorf("invalid version %q", p.Version)
	}
	if !sha256RE.MatchString(p.SHA256) {
		return errors.New("sha256 must be 64 lowercase hex characters")
	}
	if p.Signature == "" {
		return errors.New("release is unsigned")
	}
	sig, err := base64.StdEncoding.DecodeString(p.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("signature is not a base64 Ed25519 signature")
	}
	msg := SignedMessage(p.Version, p.SHA256)
	for _, k := range keys {
		if ed25519.Verify(k.Key, msg, sig) {
			return nil
		}
	}
	return errors.New("signature does not verify against any trusted key")
}
