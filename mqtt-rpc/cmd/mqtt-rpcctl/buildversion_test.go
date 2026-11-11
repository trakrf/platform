package main

import (
	"crypto/ed25519"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildVersioned(t *testing.T, version string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "bin")
	cmd := exec.Command("go", "build", "-ldflags", "-X main.version="+version, "-o", out, "./testdata/versioned")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	b, err := cmd.CombinedOutput()
	require.NoError(t, err, string(b))
	return out
}

// A release signed under a label its binary does not carry would install, run
// as a different version than the reader expects, and never be able to commit.
func TestSignRefusesABinaryBuiltWithAnotherVersion(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	bin := buildVersioned(t, "v1.5.0-300-gabc1234")

	_, err := signRelease(priv, "v1.5.0-300-gabc1234", bin, "https://x/y", true)
	require.NoError(t, err)

	_, err = signRelease(priv, "v1.5.0-301-gdef5678", bin, "https://x/y", true)
	assert.ErrorContains(t, err, "v1.5.0-300-gabc1234")
}

// The release builds (install.sh, the CI workflow) must not use -trimpath: Go
// then drops -ldflags from build info and the version check cannot read it.
func TestSignExplainsATrimpathBuild(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	out := filepath.Join(t.TempDir(), "bin")
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-X main.version=v1", "-o", out, "./testdata/versioned")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm", "GOARM=7")
	b, err := cmd.CombinedOutput()
	require.NoError(t, err, string(b))

	_, err = signRelease(priv, "v1", out, "https://x/y", true)
	assert.ErrorContains(t, err, "-trimpath")
}

func TestSignRefusesABinaryWithNoVersionUnlessToldNotToCheck(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	bin := filepath.Join(t.TempDir(), "random")
	require.NoError(t, os.WriteFile(bin, []byte("not a go binary"), 0o755))

	_, err := signRelease(priv, "v2", bin, "https://x/y", true)
	assert.Error(t, err)
	_, err = signRelease(priv, "v2", bin, "https://x/y", false)
	assert.NoError(t, err, "bench fixtures (deliberately broken builds) opt out explicitly")
}
