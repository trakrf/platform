package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
	"github.com/trakrf/platform/mqtt-rpc/internal/update"
)

// A release manifest is exactly the Daemon.Update params, so it can be sent as is.
type manifest = readerrpc.DaemonUpdateParams

// parseSeed decodes a base64 Ed25519 seed (the format keygen writes and the CI
// secret MQTT_RPCD_SIGNING_KEY holds).
func parseSeed(s string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("signing key is not base64: %w", err)
	}
	if len(raw) != ed25519.SeedSize {
		return nil, fmt.Errorf("signing key must be a %d-byte Ed25519 seed, got %d bytes", ed25519.SeedSize, len(raw))
	}
	return ed25519.NewKeyFromSeed(raw), nil
}

var ldflagsVersionRE = regexp.MustCompile(`main\.version=([^\s'"]+)`)

// builtVersion reads the -X main.version a Go binary was linked with.
func builtVersion(path string) (string, error) {
	bi, err := buildinfo.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s is not a Go binary with build info: %w", path, err)
	}
	trimpath := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "-ldflags":
			if m := ldflagsVersionRE.FindStringSubmatch(s.Value); m != nil {
				return m[1], nil
			}
		case "-trimpath":
			trimpath = s.Value == "true"
		}
	}
	if trimpath {
		return "", fmt.Errorf("%s was built with -trimpath, which drops -ldflags from build info; build release binaries without it", path)
	}
	return "", fmt.Errorf("%s was built without -X main.version", path)
}

// signRelease hashes the binary at path and signs it as version, to be served at
// url. With checkVersion, the binary must have been linked with exactly that
// version: a reader installs a release under its label and the daemon reports its
// compiled-in version, so a mismatch is a build that can never commit.
func signRelease(priv ed25519.PrivateKey, version, path, url string, checkVersion bool) (manifest, error) {
	if !update.ValidVersion(version) {
		return manifest{}, fmt.Errorf("invalid version %q", version)
	}
	if checkVersion {
		built, err := builtVersion(path)
		if err != nil {
			return manifest{}, err
		}
		if built != version {
			return manifest{}, fmt.Errorf("binary was built as %s, not %s", built, version)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return manifest{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return manifest{}, err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	return manifest{Version: version, URL: url, SHA256: sum, Signature: update.Sign(priv, version, sum)}, nil
}

// loadManifest reads a manifest from a file path or an https URL.
func loadManifest(src string) (manifest, error) {
	var raw []byte
	if strings.HasPrefix(src, "https://") {
		c := &http.Client{Timeout: 30 * time.Second}
		resp, err := c.Get(src)
		if err != nil {
			return manifest{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return manifest{}, fmt.Errorf("fetch %s: status %d", src, resp.StatusCode)
		}
		if raw, err = io.ReadAll(io.LimitReader(resp.Body, 1<<16)); err != nil {
			return manifest{}, err
		}
	} else {
		var err error
		if raw, err = os.ReadFile(src); err != nil {
			return manifest{}, err
		}
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Version == "" || m.URL == "" || m.SHA256 == "" || m.Signature == "" {
		return manifest{}, errors.New("manifest is missing version, url, sha256 or signature")
	}
	return m, nil
}

// keygen writes <dir>/<id>.key (base64 seed, 0600) and <dir>/<id>.pub, refusing
// to overwrite. It prints the public key for internal/update/keys.go.
func keygen(dir, id string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	keyPath := filepath.Join(dir, id+".key")
	if _, err := os.Stat(keyPath); err == nil {
		return fmt.Errorf("%s exists; refusing to overwrite a signing key", keyPath)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		return err
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	if err := os.WriteFile(filepath.Join(dir, id+".pub"), []byte(pubB64+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s %s\n", id, pubB64)
	return nil
}
