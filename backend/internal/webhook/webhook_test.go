package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/assetevent"
	webhookmodel "github.com/trakrf/platform/backend/internal/models/webhook"
)

func testLogger() *zerolog.Logger {
	l := zerolog.Nop()
	return &l
}

func readAll(r *http.Request) ([]byte, error) { return io.ReadAll(r.Body) }

func sampleEvent() assetevent.AssetMoved {
	from := assetevent.Location{ID: 10, Name: "Receiving"}
	return assetevent.AssetMoved{
		DeliveryID: "9f2c8e14-0000-0000-0000-000000000001",
		OccurredAt: time.Unix(1753452192, 0).UTC(),
		OrgID:      42,
		Asset:      assetevent.Asset{ID: 555, ExternalKey: "FORK-7", Name: "Forklift 7"},
		From:       &from,
		To:         assetevent.Location{ID: 20, Name: "Bay 3"},
	}
}

// --- signing -----------------------------------------------------------------

func TestSignKnownVector(t *testing.T) {
	// Pinned so a refactor of the signed material (currently
	// timestamp + "." + body) breaks loudly rather than silently invalidating
	// every integrator's verification.
	got := Sign("whsec_test", "1753452192", []byte(`{"a":1}`))
	require.Equal(t, "sha256=d9a5a690a7b324dc43a07dcefcbf7274464107255792917d25cf423d84c3f1bc", got)
}

func TestSignChangesWithTimestampBodyAndSecret(t *testing.T) {
	base := Sign("secret", "100", []byte("body"))
	require.NotEqual(t, base, Sign("secret", "101", []byte("body")), "timestamp must be inside the signature (replay defense)")
	require.NotEqual(t, base, Sign("secret", "100", []byte("bodyy")))
	require.NotEqual(t, base, Sign("other", "100", []byte("body")))
	require.True(t, strings.HasPrefix(base, "sha256="))
}

// --- payload -----------------------------------------------------------------

func TestEnvelopeIsLogicalDataOnly(t *testing.T) {
	body, err := Encode(sampleEvent())
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(body, &raw))
	require.Equal(t, "asset.moved", raw["event"])
	require.NotEmpty(t, raw["delivery_id"])
	require.NotContains(t, raw, "sequence", "no sequence field: delivery order is not guaranteed")

	data := raw["data"].(map[string]any)
	asset := data["asset"].(map[string]any)
	require.Equal(t, float64(555), asset["id"])
	require.Equal(t, "FORK-7", asset["external_key"])
	require.Equal(t, "Forklift 7", asset["name"])
	require.Equal(t, float64(20), data["to_location"].(map[string]any)["id"])
	require.Equal(t, float64(10), data["from_location"].(map[string]any)["id"])

	// Physical layer must never appear on the wire.
	s := string(body)
	for _, forbidden := range []string{"epc", "EPC", "scan_point", "tag_scan", "rssi"} {
		require.NotContains(t, s, forbidden)
	}
}

func TestEnvelopeNullOriginOnFirstSighting(t *testing.T) {
	ev := sampleEvent()
	ev.From = nil
	body, err := Encode(ev)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(body, &raw))
	data := raw["data"].(map[string]any)
	require.Contains(t, data, "from_location", "the key is present and explicitly null, not omitted")
	require.Nil(t, data["from_location"])
}

func TestSyntheticEventIsObviouslySynthetic(t *testing.T) {
	ev := SyntheticEvent(42)
	require.Equal(t, 42, ev.OrgID)
	require.Equal(t, "TEST-ASSET", ev.Asset.ExternalKey)
	require.NotEmpty(t, ev.DeliveryID)
	require.NotNil(t, ev.From)
}

// --- target guard -------------------------------------------------------------

func TestAllowPrivateTargetsFailsClosed(t *testing.T) {
	for _, env := range []string{"prod", "production", "staging", "unknown-env"} {
		require.False(t, AllowPrivateTargets(env), "APP_ENV=%q must get the full guard", env)
	}
	for _, env := range []string{"", "test", "preview", "development", "dev", "local"} {
		require.True(t, AllowPrivateTargets(env), "APP_ENV=%q is a lab env", env)
	}
}

func TestValidateTargetURL(t *testing.T) {
	strict := NewClient(false)
	require.NoError(t, strict.ValidateTargetURL("https://example.com/hook"))
	require.Error(t, strict.ValidateTargetURL("http://example.com/hook"), "http leaks payload and signature")
	require.Error(t, strict.ValidateTargetURL("ftp://example.com/hook"))
	require.Error(t, strict.ValidateTargetURL("/relative/path"))
	require.Error(t, strict.ValidateTargetURL("https://"))

	relaxed := NewClient(true)
	require.NoError(t, relaxed.ValidateTargetURL("http://127.0.0.1:8080/hook"))
}

// The guard has to run on the RESOLVED IP, not the URL string: a hostname the
// customer controls can resolve to the metadata endpoint. Each of these
// literals also proves the corresponding predicate is wired up.
func TestSSRFBlockedTargets(t *testing.T) {
	c := NewClient(false)
	for _, target := range []string{
		"https://127.0.0.1/hook",       // loopback
		"https://[::1]/hook",           // loopback v6
		"https://10.0.0.1/hook",        // RFC1918
		"https://192.168.1.5/hook",     // RFC1918
		"https://172.16.4.4/hook",      // RFC1918
		"https://169.254.169.254/hook", // cloud metadata — live target on GKE
		"https://[fd00::1]/hook",       // ULA
		"https://100.64.0.1/hook",      // CGNAT
		"https://0.0.0.0/hook",         // unspecified
		"https://localhost/hook",       // resolves to loopback
	} {
		status, err := c.Deliver(context.Background(), target, "whsec_x", sampleEvent())
		require.Error(t, err, "target %s must be blocked", target)
		require.Equal(t, 0, status)
	}
}

func TestSSRFGuardRelaxedInLabEnvs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// The strict client refuses the same httptest target the relaxed one accepts.
	_, err := NewClient(false).Deliver(context.Background(), srv.URL, "whsec_x", sampleEvent())
	require.Error(t, err)

	status, err := NewClient(true).Deliver(context.Background(), srv.URL, "whsec_x", sampleEvent())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
}

// A permitted host must not be able to bounce the request into a blocked range
// after the dialer guard has already passed.
func TestRedirectRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewClient(true).Deliver(context.Background(), srv.URL, "whsec_x", sampleEvent())
	require.Error(t, err)
	require.Contains(t, err.Error(), "redirect")
}

// --- delivery -----------------------------------------------------------------

func TestDeliverSendsSignedPayload(t *testing.T) {
	var gotBody []byte
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotBody, _ = readAll(r)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	ev := sampleEvent()
	status, err := NewClient(true).Deliver(context.Background(), srv.URL, "whsec_shared", ev)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, status)

	require.Equal(t, "asset.moved", gotHeaders.Get("X-TrakRF-Event"))
	require.Equal(t, ev.DeliveryID, gotHeaders.Get("X-TrakRF-Delivery"))
	require.Equal(t, "application/json", gotHeaders.Get("Content-Type"))

	ts := gotHeaders.Get("X-TrakRF-Timestamp")
	require.Equal(t, strconv.FormatInt(ev.OccurredAt.Unix(), 10), ts)

	// The receiver's verification, performed here: recompute and compare.
	require.Equal(t, Sign("whsec_shared", ts, gotBody), gotHeaders.Get("X-TrakRF-Signature"))
	require.NotEqual(t, Sign("whsec_wrong", ts, gotBody), gotHeaders.Get("X-TrakRF-Signature"))
}

func TestNon2xxIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	status, err := NewClient(true).Deliver(context.Background(), srv.URL, "whsec_x", sampleEvent())
	require.Error(t, err)
	require.Equal(t, http.StatusInternalServerError, status, "the status is reported even on failure, for the test-fire UI")
}

// --- sink ---------------------------------------------------------------------

type fakeSinkStore struct {
	wh       *webhookmodel.Webhook
	entitled bool
	err      error
	calls    int
}

func (f *fakeSinkStore) GetWebhookForDelivery(_ context.Context, _ int) (*webhookmodel.Webhook, bool, error) {
	f.calls++
	return f.wh, f.entitled, f.err
}

func newSinkFixture(t *testing.T, store *fakeSinkStore) (*Sink, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	if store.wh != nil && store.wh.URL == "" {
		store.wh.URL = srv.URL
	}
	return NewSink(store, NewClient(true), testLogger()), &hits
}

func TestSinkSkipsWhenNoWebhook(t *testing.T) {
	store := &fakeSinkStore{entitled: true}
	s, hits := newSinkFixture(t, store)
	require.NoError(t, s.Deliver(context.Background(), sampleEvent()))
	require.Zero(t, *hits)
}

func TestSinkSkipsWhenDisabled(t *testing.T) {
	store := &fakeSinkStore{wh: &webhookmodel.Webhook{Secret: "whsec_x", Enabled: false}, entitled: true}
	s, hits := newSinkFixture(t, store)
	require.NoError(t, s.Deliver(context.Background(), sampleEvent()))
	require.Zero(t, *hits)
}

// The abandoned-trial case: the row is live and enabled, but the org stopped
// paying. No middleware sees an outbound POST, so this is the only gate.
func TestSinkSkipsWhenUnentitled(t *testing.T) {
	store := &fakeSinkStore{wh: &webhookmodel.Webhook{Secret: "whsec_x", Enabled: true}, entitled: false}
	s, hits := newSinkFixture(t, store)
	require.NoError(t, s.Deliver(context.Background(), sampleEvent()), "a skip is not a retryable failure")
	require.Zero(t, *hits)
}

func TestSinkDeliversWhenEnabledAndEntitled(t *testing.T) {
	store := &fakeSinkStore{wh: &webhookmodel.Webhook{Secret: "whsec_x", Enabled: true}, entitled: true}
	s, hits := newSinkFixture(t, store)
	require.NoError(t, s.Deliver(context.Background(), sampleEvent()))
	require.Equal(t, 1, *hits)
}

func TestSinkPropagatesLookupError(t *testing.T) {
	store := &fakeSinkStore{err: errors.New("db down")}
	s, _ := newSinkFixture(t, store)
	require.Error(t, s.Deliver(context.Background(), sampleEvent()), "a lookup failure is transient and worth retrying")
}
