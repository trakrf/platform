package twiliosms

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
)

type statusConsumer struct {
	statuses []sms.ProviderStatus
	err      error
}

func (c *statusConsumer) HandleStatus(_ context.Context, status sms.ProviderStatus) error {
	c.statuses = append(c.statuses, status)
	return c.err
}

func (*statusConsumer) HandleKeyword(context.Context, sms.InboundKeyword) error {
	return nil
}

// This fails if a valid Twilio status callback is not normalized into exactly
// one provider event with the known provider status and a UTC occurrence time.
func TestStatus_EmitsKnownDeliveryStatuses(t *testing.T) {
	occurredAt := time.Date(2026, time.August, 30, 10, 11, 12, 0, time.FixedZone("callback", -4*60*60))
	utcOccurredAt := time.Date(2026, time.August, 30, 14, 11, 12, 0, time.UTC)

	for _, test := range []struct {
		name      string
		status    string
		errorCode string
	}{
		{name: "queued", status: "queued"},
		{name: "sent", status: "sent"},
		{name: "delivered", status: "delivered"},
		{name: "undelivered with error code", status: "undelivered", errorCode: "30007"},
		{name: "failed", status: "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			consumer := &statusConsumer{}
			handler := newStatusTestHandler(t, consumer, occurredAt)
			rec := httptest.NewRecorder()
			form := url.Values{
				"MessageSid":    {"SM123"},
				"MessageStatus": {test.status},
			}
			if test.errorCode != "" {
				form.Set("ErrorCode", test.errorCode)
			}

			handler.Status(rec, signedStatusRequest(t, form))

			require.Equal(t, http.StatusNoContent, rec.Code)
			require.Equal(t, []sms.ProviderStatus{{
				ProviderMessageID: "SM123",
				Status:            test.status,
				ErrorCode:         test.errorCode,
				OccurredAt:        utcOccurredAt,
			}}, consumer.statuses)
		})
	}
}

// This fails if a signed callback missing required fields or carrying an
// unsupported status is acknowledged or passed to the consumer.
func TestStatus_RejectsSignedMalformedInputWithoutEvent(t *testing.T) {
	for _, test := range []struct {
		name string
		form url.Values
	}{
		{name: "missing message sid", form: url.Values{"MessageStatus": {"queued"}}},
		{name: "empty message sid", form: url.Values{"MessageSid": {""}, "MessageStatus": {"queued"}}},
		{name: "missing message status", form: url.Values{"MessageSid": {"SM123"}}},
		{name: "empty message status", form: url.Values{"MessageSid": {"SM123"}, "MessageStatus": {""}}},
		{name: "unsupported status", form: url.Values{"MessageSid": {"SM123"}, "MessageStatus": {"accepted"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			consumer := &statusConsumer{}
			rec := httptest.NewRecorder()

			newStatusTestHandler(t, consumer, time.Time{}).Status(rec, signedStatusRequest(t, test.form))

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Empty(t, consumer.statuses)
		})
	}
}

// This fails if a forged callback reaches the event consumer.
func TestStatus_RejectsInvalidSignatureWithoutEvent(t *testing.T) {
	consumer := &statusConsumer{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/twilio/status", strings.NewReader("MessageSid=SM123&MessageStatus=delivered"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", "forged")
	rec := httptest.NewRecorder()

	newStatusTestHandler(t, consumer, time.Time{}).Status(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Empty(t, consumer.statuses)
}

// This fails if the receiver reports a consumer failure as a successful
// callback acknowledgement.
func TestStatus_ReturnsServerErrorWhenConsumerFails(t *testing.T) {
	consumer := &statusConsumer{err: errors.New("durable handoff unavailable")}
	rec := httptest.NewRecorder()

	newStatusTestHandler(t, consumer, time.Time{}).Status(rec, signedStatusRequest(t, url.Values{
		"MessageSid":    {"SM123"},
		"MessageStatus": {"delivered"},
	}))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Len(t, consumer.statuses, 1)
}

// This fails if a retry of the same valid Twilio callback is silently dropped
// before the injected downstream consumer can apply its own durable policy.
func TestStatus_HandsEachRepeatedValidCallbackToConsumer(t *testing.T) {
	consumer := &statusConsumer{}
	handler := newStatusTestHandler(t, consumer, time.Time{})
	form := url.Values{"MessageSid": {"SM123"}, "MessageStatus": {"delivered"}}

	for range 2 {
		rec := httptest.NewRecorder()
		handler.Status(rec, signedStatusRequest(t, form))
		require.Equal(t, http.StatusNoContent, rec.Code)
	}

	require.Equal(t, []sms.ProviderStatus{
		{ProviderMessageID: "SM123", Status: "delivered", OccurredAt: time.Time{}},
		{ProviderMessageID: "SM123", Status: "delivered", OccurredAt: time.Time{}},
	}, consumer.statuses)
}

// This fails if a handler without a downstream consumer panics or
// acknowledges a callback it cannot hand off.
func TestStatus_FailsClosedWithoutConsumer(t *testing.T) {
	var typedNilConsumer *statusConsumer
	for _, test := range []struct {
		name     string
		consumer sms.CallbackConsumer
	}{
		{name: "nil interface", consumer: nil},
		{name: "typed nil", consumer: typedNilConsumer},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newStatusTestHandler(t, test.consumer, time.Time{})
			rec := httptest.NewRecorder()

			require.NotPanics(t, func() {
				handler.Status(rec, signedStatusRequest(t, url.Values{
					"MessageSid":    {"SM123"},
					"MessageStatus": {"delivered"},
				}))
			})

			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	}
}

func newStatusTestHandler(t *testing.T, consumer sms.CallbackConsumer, now time.Time) *Handler {
	t.Helper()
	handler, err := NewHandler(newSignatureTestConfig(), consumer)
	require.NoError(t, err)
	handler.now = func() time.Time { return now }
	return handler
}

func signedStatusRequest(t *testing.T, form url.Values) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/twilio/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", twilioFormSignature(testPublicBaseURL+req.URL.EscapedPath(), form))
	return req
}

func twilioFormSignature(callbackURL string, form url.Values) string {
	keys := make([]string, 0, len(form))
	for key := range form {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	mac := hmac.New(sha1.New, []byte(testAuthToken))
	_, _ = mac.Write([]byte(callbackURL))
	for _, key := range keys {
		_, _ = mac.Write([]byte(key + form.Get(key)))
	}
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
