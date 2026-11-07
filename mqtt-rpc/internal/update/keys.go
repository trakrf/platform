package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
)

// trustedKeyB64 are the Ed25519 public keys a release must be signed by. They
// are compiled in: a reader trusts exactly the keys of the build it is running,
// so the set can only change by shipping a build signed with a key already in it.
//
//   - ci-2026-11: the private half is the GitHub Actions secret
//     MQTT_RPCD_SIGNING_KEY, used by .github/workflows/mqtt-rpcd-release.yml.
//   - offline-2026-11: the private half lives offline (password manager), never
//     in CI. It signs releases if the CI key is lost or must be revoked.
//
// Rotation: ship a build trusting {old, new} signed by old; once every reader
// runs it, ship one trusting {new, offline} signed by new. See mqtt-rpc/README.md.
var trustedKeyB64 = []struct{ id, key string }{
	{"ci-2026-11", "BmAhRVNG8vqBUYqH/SWbwqf6UmdYsx6eJLz+cGJv0Oo="},
	{"offline-2026-11", "dmdQcHG26CItK9FLf2Lt+8YgsUVMQyhQFquIYgz9LQQ="},
}

// TrustedKeys is trustedKeyB64 decoded. A malformed constant panics at init: a
// build that cannot verify anything must never ship, and the unit test catches it.
var TrustedKeys = mustDecodeKeys()

func mustDecodeKeys() []TrustedKey {
	out := make([]TrustedKey, 0, len(trustedKeyB64))
	for _, k := range trustedKeyB64 {
		raw, err := base64.StdEncoding.DecodeString(k.key)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			panic(fmt.Sprintf("update: trusted key %s is malformed", k.id))
		}
		out = append(out, TrustedKey{ID: k.id, Key: ed25519.PublicKey(raw)})
	}
	return out
}
