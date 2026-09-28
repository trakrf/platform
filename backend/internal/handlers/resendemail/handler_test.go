package resendemail

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/resend"
)

const secret = "fixture-signing-key"
const fixture = `{
 "id": "unauthenticated-body-id",
 "type": "%s",
 "created_at": "2026-01-02T03:04:05.123456Z",
 "data": {"email_id":"email-123", "created_at":"2026-01-01T00:00:00Z", "to":["private@example.com"], "subject":"private subject", "bounce":{"message":"private reason"}}
}`

type consumeFunc func(context.Context, email.CallbackEvent) error

func (f consumeFunc) HandleEvent(ctx context.Context, e email.CallbackEvent) error { return f(ctx, e) }
func config() resend.Config {
	return resend.Config{Enabled: true, APIKey: "re_fixture", From: "alerts@example.com", WebhookSecret: "whsec_" + base64.StdEncoding.EncodeToString([]byte(secret)), Timeout: time.Second}
}
func signed(body string, at time.Time) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("svix-id", "evt-authenticated")
	r.Header.Set("svix-timestamp", strconv.FormatInt(at.Unix(), 10))
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s.%s.%s", r.Header.Get("svix-id"), r.Header.Get("svix-timestamp"), body)
	r.Header.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return r
}
func TestWebhookNormalizesVerifiedEvents(t *testing.T) {
	for providerType, kind := range map[string]email.EventType{"email.delivered": email.EventDelivered, "email.failed": email.EventFailed, "email.bounced": email.EventBounced, "email.complained": email.EventComplained} {
		t.Run(providerType, func(t *testing.T) {
			var events []email.CallbackEvent
			h, err := NewHandler(config(), consumeFunc(func(ctx context.Context, e email.CallbackEvent) error { events = append(events, e); return nil }))
			require.NoError(t, err)
			before := time.Now().UTC()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, signed(fmt.Sprintf(fixture, providerType), time.Now()))
			require.Equal(t, http.StatusNoContent, w.Code)
			require.Len(t, events, 1)
			e := events[0]
			require.Equal(t, "resend", e.Provider)
			require.Equal(t, "evt-authenticated", e.ProviderEventID)
			require.Equal(t, "email-123", e.ProviderMessageID)
			require.Equal(t, kind, e.Type)
			require.Equal(t, "2026-01-02T03:04:05.123456Z", e.OccurredAt.Format(time.RFC3339Nano))
			require.False(t, e.ReceivedAt.Before(before))
			require.False(t, e.ReceivedAt.After(time.Now()))
			require.NotContains(t, fmt.Sprintf("%+v", e), "private")
		})
	}
}

func TestWebhookRejectsUnverifiedRequests(t *testing.T) {
	body := fmt.Sprintf(fixture, "email.delivered")
	for name, mutate := range map[string]func(*http.Request){
		"missing id":        func(r *http.Request) { r.Header.Del("svix-id") },
		"changed id":        func(r *http.Request) { r.Header.Set("svix-id", "forged") },
		"missing timestamp": func(r *http.Request) { r.Header.Del("svix-timestamp") },
		"invalid timestamp": func(r *http.Request) { r.Header.Set("svix-timestamp", "bad") },
		"missing signature": func(r *http.Request) { r.Header.Del("svix-signature") },
		"invalid signature": func(r *http.Request) { r.Header.Set("svix-signature", "v1,forged") },
		"unknown version": func(r *http.Request) {
			r.Header.Set("svix-signature", strings.Replace(r.Header.Get("svix-signature"), "v1,", "v2,", 1))
		},
		"changed bytes":      func(r *http.Request) { r.Body = http.NoBody },
		"changed whitespace": func(r *http.Request) { r.Body = signed(body+" ", time.Now()).Body },
		"expired":            func(r *http.Request) { *r = *signed(body, time.Now().Add(-10*time.Minute)) },
		"future":             func(r *http.Request) { *r = *signed(body, time.Now().Add(10*time.Minute)) },
	} {
		t.Run(name, func(t *testing.T) {
			h, err := NewHandler(config(), consumeFunc(func(context.Context, email.CallbackEvent) error { t.Fatal("unverified event consumed"); return nil }))
			require.NoError(t, err)
			r := signed(body, time.Now())
			mutate(r)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			require.Equal(t, http.StatusForbidden, w.Code)
			require.NotContains(t, w.Body.String(), "private")
		})
	}
}

func TestWebhookMalformedAndUnsupported(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"invalid json", "{", 400}, {"null", "null", 400}, {"empty object", "{}", 400},
		{"no message", `{"type":"email.delivered","created_at":"2026-01-02T03:04:05Z","data":{}}`, 400},
		{"bad date", `{"type":"email.failed","created_at":"invalid","data":{"email_id":"email"}}`, 400},
		{"no date", `{"type":"email.failed","data":{"email_id":"email"}}`, 400},
		{"whitespace message", `{"type":"email.failed","created_at":"2026-01-02T03:04:05Z","data":{"email_id":" "}}`, 400},
		{"trailing json", fmt.Sprintf(fixture, "email.delivered") + "{}", 400},
		{"unsupported", fmt.Sprintf(fixture, "email.opened"), 204},
		{"future type", `{"type":"contact.future","data":{"unrecognized":true}}`, 204},
		{"oversize", strings.Repeat(" ", maxBodyBytes+1), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := NewHandler(config(), consumeFunc(func(context.Context, email.CallbackEvent) error { t.Fatal("event should not be consumed"); return nil }))
			require.NoError(t, err)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, signed(tc.body, time.Now()))
			require.Equal(t, tc.status, w.Code)
		})
	}
}

func TestWebhookPersistsBeforeAcknowledgmentAndRetries(t *testing.T) {
	calls := 0
	w := httptest.NewRecorder()
	h, err := NewHandler(config(), consumeFunc(func(ctx context.Context, e email.CallbackEvent) error {
		calls++
		require.False(t, w.Flushed)
		require.Empty(t, w.Body.String())
		if calls == 1 {
			return errors.New("private database detail")
		}
		return nil // durable insert or duplicate
	}))
	require.NoError(t, err)
	for _, status := range []int{503, 204, 204} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, signed(fmt.Sprintf(fixture, "email.delivered"), time.Now()))
		require.Equal(t, status, w.Code)
		require.NotContains(t, w.Body.String(), "private")
	}
	require.Equal(t, 3, calls)
}

func TestWebhookSignatureRotation(t *testing.T) {
	h, err := NewHandler(config(), consumeFunc(func(context.Context, email.CallbackEvent) error { return nil }))
	require.NoError(t, err)
	r := signed(fmt.Sprintf(fixture, "email.delivered"), time.Now())
	r.Header.Set("svix-signature", "v2,unsupported v1,oldkey "+r.Header.Get("svix-signature"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, 204, w.Code)
}

func TestWebhookConstructorFailsClosed(t *testing.T) {
	valid := consumeFunc(func(context.Context, email.CallbackEvent) error { return nil })
	for _, tc := range []struct {
		cfg      resend.Config
		consumer email.CallbackConsumer
	}{
		{resend.Config{}, valid}, {config(), nil}, {config(), consumeFunc(nil)},
	} {
		h, err := NewHandler(tc.cfg, tc.consumer)
		require.Error(t, err)
		require.Nil(t, h)
	}
	cfg := config()
	cfg.WebhookSecret = "bad"
	h, err := NewHandler(cfg, valid)
	require.Error(t, err)
	require.Nil(t, h)
}

type trackedWriter struct {
	*httptest.ResponseRecorder
	wroteHeader bool
}

func (w *trackedWriter) WriteHeader(code int) {
	w.wroteHeader = true
	w.ResponseRecorder.WriteHeader(code)
}
func (w *trackedWriter) Write(p []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseRecorder.Write(p)
}

func TestWebhookWaitsForConsumerAndPropagatesContext(t *testing.T) {
	type contextKey struct{}
	w := &trackedWriter{ResponseRecorder: httptest.NewRecorder()}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h, err := NewHandler(config(), consumeFunc(func(ctx context.Context, e email.CallbackEvent) error {
		if ctx.Value(contextKey{}) != "request-value" {
			t.Error("request context lost")
		}
		close(entered)
		<-release
		return nil
	}))
	require.NoError(t, err)
	r := signed(fmt.Sprintf(fixture, "email.delivered"), time.Now())
	r = r.WithContext(context.WithValue(r.Context(), contextKey{}, "request-value"))
	go func() { defer close(done); h.ServeHTTP(w, r) }()
	<-entered
	require.False(t, w.wroteHeader, "no acknowledgment before durable consumer completes")
	close(release)
	<-done
	require.True(t, w.wroteHeader)
	require.Equal(t, 204, w.Code)
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("private read error") }
func (brokenBody) Close() error             { return nil }

func TestWebhookRejectsMethodAndUnreadableBody(t *testing.T) {
	h, err := NewHandler(config(), consumeFunc(func(context.Context, email.CallbackEvent) error { t.Fatal("unexpected consumer call"); return nil }))
	require.NoError(t, err)
	r := signed(fmt.Sprintf(fixture, "email.delivered"), time.Now())
	r.Method = http.MethodGet
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, 405, w.Code)
	require.Equal(t, "POST", w.Header().Get("Allow"))
	r.Method = http.MethodPost
	r.Body = brokenBody{}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, 400, w.Code)
	require.NotContains(t, w.Body.String(), "private")
}
