package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

const testSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testKey(t *testing.T, id string) (TrustedKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return TrustedKey{ID: id, Key: pub}, priv
}

func signedParams(priv ed25519.PrivateKey, version, sha string) readerrpc.DaemonUpdateParams {
	return readerrpc.DaemonUpdateParams{
		Version:   version,
		URL:       "https://example.test/mqtt-rpcd",
		SHA256:    sha,
		Signature: Sign(priv, version, sha),
	}
}

func TestSignedMessageIsVersionedAndBindsVersionToHash(t *testing.T) {
	assert.Equal(t,
		"trakrf-mqtt-rpcd-release-v1\nversion=v1.5.0-240-gabc1234\nsha256="+testSHA+"\n",
		string(SignedMessage("v1.5.0-240-gabc1234", testSHA)))
}

func TestVerifyAcceptsAReleaseSignedByAnyTrustedKey(t *testing.T) {
	k1, _ := testKey(t, "ci")
	k2, priv2 := testKey(t, "offline")
	require.NoError(t, Verify([]TrustedKey{k1, k2}, signedParams(priv2, "v2", testSHA)))
}

func TestVerifyRejectsTampering(t *testing.T) {
	k, priv := testKey(t, "ci")
	keys := []TrustedKey{k}
	good := signedParams(priv, "v2", testSHA)

	relabelled := good
	relabelled.Version = "v3"
	assert.Error(t, Verify(keys, relabelled), "a signature must not transfer to another version")

	otherBytes := good
	otherBytes.SHA256 = strings.Repeat("f", 64)
	assert.Error(t, Verify(keys, otherBytes), "a signature must not transfer to other bytes")

	badSig := good
	badSig.Signature = Sign(priv, "v2", strings.Repeat("e", 64))
	assert.Error(t, Verify(keys, badSig))
}

func TestVerifyRejectsAnUntrustedKey(t *testing.T) {
	trusted, _ := testKey(t, "ci")
	_, stranger := testKey(t, "stranger")
	assert.Error(t, Verify([]TrustedKey{trusted}, signedParams(stranger, "v2", testSHA)))
}

func TestVerifyRejectsUnsignedAndMalformed(t *testing.T) {
	k, priv := testKey(t, "ci")
	keys := []TrustedKey{k}

	unsigned := signedParams(priv, "v2", testSHA)
	unsigned.Signature = ""
	assert.Error(t, Verify(keys, unsigned))

	notBase64 := signedParams(priv, "v2", testSHA)
	notBase64.Signature = "!!!"
	assert.Error(t, Verify(keys, notBase64))

	for _, sha := range []string{"", "xyz", testSHA[:63], strings.ToUpper(testSHA), testSHA + "0"} {
		p := signedParams(priv, "v2", sha)
		assert.Error(t, Verify(keys, p), "sha %q", sha)
	}

	assert.Error(t, Verify(nil, signedParams(priv, "v2", testSHA)), "no trusted keys means nothing verifies")
}

func TestValidVersion(t *testing.T) {
	for _, ok := range []string{"v1.5.0-240-gabc1234", "v1.5.0-240-gabc1234-dirty", "dev", "1.2.3+build_7"} {
		assert.True(t, ValidVersion(ok), ok)
	}
	for _, bad := range []string{"", "a b", "a/b", "../x", "$(id)", "a\nb", strings.Repeat("a", 65), "v1;rm"} {
		assert.False(t, ValidVersion(bad), "%q", bad)
	}
}

func TestEmbeddedTrustedKeysDecode(t *testing.T) {
	require.Len(t, TrustedKeys, 2)
	ids := map[string]bool{}
	for _, k := range TrustedKeys {
		assert.Len(t, k.Key, ed25519.PublicKeySize, k.ID)
		ids[k.ID] = true
	}
	assert.True(t, ids["ci-2026-11"])
	assert.True(t, ids["offline-2026-11"])
}
