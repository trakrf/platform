// Package webhook delivers assetevent domain events to a customer's HTTPS
// endpoint (TRA-1043). It is one Sink among the ones assetevent can fan out to;
// detection lives entirely in assetevent.
//
// Delivery is at-most-once and deliberately hostile to the outside world:
// https-only, redirects refused, a hard timeout, and an SSRF guard that
// inspects the resolved IP rather than the URL string.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignaturePrefix names the algorithm in the header value so a future
// algorithm change is distinguishable on the wire.
const SignaturePrefix = "sha256="

// Sign returns the X-TrakRF-Signature value for a delivery: HMAC-SHA256 over
// `timestamp + "." + body`, hex-encoded.
//
// The timestamp is inside the signed material on purpose — signing the body
// alone would let an observer replay a captured delivery indefinitely. A
// receiver checks the signature AND that X-TrakRF-Timestamp is recent.
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return SignaturePrefix + hex.EncodeToString(mac.Sum(nil))
}
