package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/update"
)

func TestSignReleaseProducesAManifestTheDaemonAccepts(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "mqtt-rpcd")
	require.NoError(t, os.WriteFile(bin, []byte("binary bytes"), 0o755))

	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	priv, err := parseSeed(base64.StdEncoding.EncodeToString(seed) + "\n")
	require.NoError(t, err)

	m, err := signRelease(priv, "v1.5.0-240-gabc1234", bin, "https://example.test/mqtt-rpcd")
	require.NoError(t, err)
	assert.Equal(t, "v1.5.0-240-gabc1234", m.Version)
	assert.Equal(t, "https://example.test/mqtt-rpcd", m.URL)
	assert.Len(t, m.SHA256, 64)

	pub := priv.Public().(ed25519.PublicKey)
	require.NoError(t, update.Verify([]update.TrustedKey{{ID: "t", Key: pub}}, m))

	raw, err := json.Marshal(m)
	require.NoError(t, err)
	path := filepath.Join(dir, "manifest.json")
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	back, err := loadManifest(path)
	require.NoError(t, err)
	assert.Equal(t, m, back)
}

func TestSignReleaseRefusesABadVersion(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	bin := filepath.Join(t.TempDir(), "b")
	require.NoError(t, os.WriteFile(bin, []byte("x"), 0o755))
	_, err := signRelease(priv, "v1 bad", bin, "https://x/y")
	assert.Error(t, err)
}

func TestParseSeedRejectsGarbage(t *testing.T) {
	_, err := parseSeed("not base64!")
	assert.Error(t, err)
	_, err = parseSeed(base64.StdEncoding.EncodeToString([]byte("short")))
	assert.Error(t, err)
}
