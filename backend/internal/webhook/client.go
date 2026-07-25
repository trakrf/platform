package webhook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"syscall"
	"time"

	"github.com/trakrf/platform/backend/internal/assetevent"
)

const (
	// deliveryTimeout caps a single POST end to end. A customer's slow endpoint
	// must not tie up a worker.
	deliveryTimeout = 5 * time.Second
	// dialTimeout caps connection setup separately, so a black-holed IP fails
	// well before the overall budget.
	dialTimeout = 3 * time.Second
	// maxResponseBytes is how much of a response body we read before giving up.
	// We only need the status code; the body is drained so the connection can be
	// reused, and bounded so a hostile endpoint can't stream at us forever.
	maxResponseBytes = 4 << 10
)

// localTargetEnvs lists the APP_ENV values where a webhook may point at a
// private address or a plain-http URL: local development, CI, and the preview
// proving ground, where httptest servers and lab endpoints live on 127.0.0.1.
//
// Fail-CLOSED, mirroring serve.testAffordancesAllowed (TRA-861): production —
// "prod" (the deploy chart's key) or "production" — and any UNRECOGNIZED env
// get the full guard. A new env key defaults to the safe side.
var localTargetEnvs = map[string]bool{
	"":            true, // local development (APP_ENV unset)
	"test":        true, // CI / integration harness
	"preview":     true, // preview proving ground (infra env key)
	"development": true,
	"dev":         true,
	"local":       true,
}

// AllowPrivateTargets reports whether webhook targets on private addresses (and
// plain http) are permitted in the given APP_ENV.
func AllowPrivateTargets(appEnv string) bool {
	return localTargetEnvs[appEnv]
}

// ErrBlockedTarget is returned when a target resolves into a blocked range.
var ErrBlockedTarget = errors.New("webhook target resolves to a blocked address")

// Client posts signed deliveries to customer endpoints.
//
// It is deliberately unforgiving about where it will connect:
//
//   - https only (an http URL leaks the payload and the signature).
//   - The SSRF guard runs in the dialer's Control hook, so it inspects the
//     RESOLVED IP. A pre-flight URL parse is not enough: a hostname the customer
//     controls can resolve to 169.254.169.254 (DNS rebinding), and on GKE the
//     metadata server is a live, credential-bearing target.
//   - Redirects are refused outright, so a 302 cannot walk a permitted host into
//     a blocked range after the guard has already passed.
type Client struct {
	http         *http.Client
	allowPrivate bool
}

// NewClient builds a delivery client. allowPrivateTargets relaxes both the
// https requirement and the address guard; pass AllowPrivateTargets(APP_ENV) so
// it is only ever true in dev/CI/preview.
func NewClient(allowPrivateTargets bool) *Client {
	d := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
	if !allowPrivateTargets {
		d.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("%w: unparseable address %q", ErrBlockedTarget, address)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("%w: unresolvable address %q", ErrBlockedTarget, host)
			}
			if isBlockedIP(ip) {
				return fmt.Errorf("%w: %s", ErrBlockedTarget, host)
			}
			return nil
		}
	}
	return &Client{
		allowPrivate: allowPrivateTargets,
		http: &http.Client{
			Timeout:   deliveryTimeout,
			Transport: &http.Transport{DialContext: d.DialContext},
			CheckRedirect: func(req *http.Request, _ []*http.Request) error {
				return fmt.Errorf("webhook target returned a redirect to %s; redirects are not followed", req.URL.Redacted())
			},
		},
	}
}

// cgnat is 100.64.0.0/10 (RFC 6598). Not covered by IsPrivate, but it is
// carrier/cluster-internal space, so a customer endpoint has no business there.
var cgnat = net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// isBlockedIP reports whether an address is in a range a customer endpoint has
// no business being in. Covers the cloud metadata endpoint (169.254.169.254 —
// link-local, and a live credential-bearing target on GKE), the cluster's own
// service and pod networks (RFC1918, ULA, CGNAT), and anything that would loop
// the request back into this process.
//
// The stdlib predicates all normalize IPv4-mapped IPv6 (::ffff:10.0.0.1) via
// To4() internally, so a mapped private address cannot slip past them.
func isBlockedIP(ip net.IP) bool {
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && cgnat.Contains(v4) {
		return true
	}
	return false
}

// ValidateTargetURL reports why a URL is unusable as a webhook target, or nil
// when it is acceptable. Used at registration time so a customer learns about a
// bad URL immediately rather than through silent delivery failures.
//
// This is a usability check, not the security boundary — the dialer guard is.
// A hostname's resolution can change after registration, which is exactly why
// the real check happens per connection.
func (c *Client) ValidateTargetURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url is not a valid URL: %w", err)
	}
	if u.Host == "" {
		return errors.New("url must be absolute and include a host")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if c.allowPrivate {
			return nil
		}
		return errors.New("url must use https")
	default:
		return errors.New("url must use https")
	}
}

// Deliver signs and posts one event. It returns the response status code (0
// when the request never completed) and an error for anything that is not a
// 2xx.
//
// A non-2xx is an error so the dispatcher retries it; the caller decides how
// many times.
func (c *Client) Deliver(ctx context.Context, target, secret string, ev assetevent.AssetMoved) (int, error) {
	if err := c.ValidateTargetURL(target); err != nil {
		return 0, err
	}

	body, err := Encode(ev)
	if err != nil {
		return 0, fmt.Errorf("failed to encode webhook payload: %w", err)
	}

	timestamp := strconv.FormatInt(ev.OccurredAt.UTC().Unix(), 10)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TrakRF-Webhooks/1")
	req.Header.Set("X-TrakRF-Event", assetevent.EventAssetMoved)
	req.Header.Set("X-TrakRF-Delivery", ev.DeliveryID)
	req.Header.Set("X-TrakRF-Timestamp", timestamp)
	req.Header.Set("X-TrakRF-Signature", Sign(secret, timestamp, body))

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("webhook delivery failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Errorf("webhook endpoint returned %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}
