package twiliosms

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
)

type inboundConsumer struct {
	keywords []sms.InboundKeyword
	err      error
}

func (*inboundConsumer) HandleStatus(context.Context, sms.ProviderStatus) error {
	return nil
}

func (c *inboundConsumer) HandleKeyword(_ context.Context, keyword sms.InboundKeyword) error {
	c.keywords = append(c.keywords, keyword)
	return c.err
}

// This fails if a recognized signed consent message is not normalized into
// exactly one provider-neutral keyword event with a UTC receipt time.
func TestInbound_NormalizesStandardConsentKeywords(t *testing.T) {
	receivedAt := time.Date(2026, time.August, 30, 10, 11, 12, 0, time.FixedZone("callback", -4*60*60))
	utcReceivedAt := time.Date(2026, time.August, 30, 14, 11, 12, 0, time.UTC)

	for _, test := range []struct {
		name    string
		body    string
		keyword string
	}{
		{name: "stop", body: " stop ", keyword: "STOP"},
		{name: "cancel", body: "CaNcEl", keyword: "STOP"},
		{name: "unsubscribe", body: "\tUNSUBSCRIBE\n", keyword: "STOP"},
		{name: "end", body: " End ", keyword: "STOP"},
		{name: "quit", body: "quit", keyword: "STOP"},
		{name: "stopall", body: " STOPALL ", keyword: "STOP"},
		{name: "revoke", body: " ReVoKe ", keyword: "STOP"},
		{name: "optout", body: "\tOpToUt\n", keyword: "STOP"},
		{name: "start", body: " start ", keyword: "START"},
		{name: "unstop", body: "UnStOp", keyword: "START"},
	} {
		t.Run(test.name, func(t *testing.T) {
			consumer := &inboundConsumer{}
			handler := newInboundTestHandler(t, consumer, receivedAt)
			rec := httptest.NewRecorder()

			handler.Inbound(rec, signedInboundRequest(t, url.Values{
				"MessageSid": {"SM123"},
				"From":       {"+15550001111"},
				"To":         {"+15550002222"},
				"Body":       {test.body},
			}))

			require.Equal(t, http.StatusNoContent, rec.Code)
			require.Equal(t, []sms.InboundKeyword{{
				ProviderMessageID: "SM123",
				FromE164:          "+15550001111",
				ToE164:            "+15550002222",
				Keyword:           test.keyword,
				ReceivedAt:        utcReceivedAt,
			}}, consumer.keywords)
		})
	}
}

// This fails if a signed recognized keyword missing an event identity field is
// acknowledged or handed to the consumer.
func TestInbound_RejectsSignedRecognizedKeywordMissingRequiredField(t *testing.T) {
	for _, test := range []struct {
		name string
		form url.Values
	}{
		{name: "missing message sid", form: url.Values{"From": {"+15550001111"}, "To": {"+15550002222"}, "Body": {"STOP"}}},
		{name: "empty message sid", form: url.Values{"MessageSid": {""}, "From": {"+15550001111"}, "To": {"+15550002222"}, "Body": {"STOP"}}},
		{name: "missing from", form: url.Values{"MessageSid": {"SM123"}, "To": {"+15550002222"}, "Body": {"STOP"}}},
		{name: "empty from", form: url.Values{"MessageSid": {"SM123"}, "From": {""}, "To": {"+15550002222"}, "Body": {"STOP"}}},
		{name: "missing to", form: url.Values{"MessageSid": {"SM123"}, "From": {"+15550001111"}, "Body": {"STOP"}}},
		{name: "empty to", form: url.Values{"MessageSid": {"SM123"}, "From": {"+15550001111"}, "To": {""}, "Body": {"STOP"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			consumer := &inboundConsumer{}
			rec := httptest.NewRecorder()

			newInboundTestHandler(t, consumer, time.Time{}).Inbound(rec, signedInboundRequest(t, test.form))

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Empty(t, consumer.keywords)
		})
	}
}

// This fails if a forged callback with sensitive text reaches the consumer or
// exposes its body in the HTTP response.
func TestInbound_RejectsInvalidSignatureWithoutEventOrBodyLeak(t *testing.T) {
	consumer := &inboundConsumer{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/twilio/inbound", strings.NewReader("MessageSid=SM123&From=%2B15550001111&To=%2B15550002222&Body=private+message"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", "forged")
	rec := httptest.NewRecorder()

	newInboundTestHandler(t, consumer, time.Time{}).Inbound(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Empty(t, consumer.keywords)
	require.NotContains(t, rec.Body.String(), "private message")
}

// This fails if a malformed callback bypasses the shared form boundary or is
// acknowledged before the consumer can be protected from it.
func TestInbound_RejectsMalformedCallbackWithoutEvent(t *testing.T) {
	for _, test := range []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "wrong content type", contentType: "application/json", body: `{"Body":"STOP"}`},
		{name: "malformed form escape", contentType: "application/x-www-form-urlencoded", body: "Body=%ZZ"},
	} {
		t.Run(test.name, func(t *testing.T) {
			consumer := &inboundConsumer{}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/twilio/inbound", strings.NewReader(test.body))
			req.Header.Set("Content-Type", test.contentType)
			req.Header.Set("X-Twilio-Signature", "forged")
			rec := httptest.NewRecorder()

			newInboundTestHandler(t, consumer, time.Time{}).Inbound(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Empty(t, consumer.keywords)
		})
	}
}

// This fails if unrecognized signed text is persisted, emitted, or reflected
// instead of being acknowledged without a consent event.
func TestInbound_AcknowledgesUnrelatedTextWithoutEventOrBodyLeak(t *testing.T) {
	consumer := &inboundConsumer{}
	rec := httptest.NewRecorder()

	newInboundTestHandler(t, consumer, time.Time{}).Inbound(rec, signedInboundRequest(t, url.Values{
		"Body": {"private message that is not a keyword"},
	}))

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, consumer.keywords)
	require.Empty(t, rec.Body.String())
}

// This fails if the receiver reports a consumer failure as a successful
// callback acknowledgement.
func TestInbound_ReturnsServerErrorWhenConsumerFails(t *testing.T) {
	consumer := &inboundConsumer{err: errors.New("durable handoff unavailable")}
	rec := httptest.NewRecorder()

	newInboundTestHandler(t, consumer, time.Time{}).Inbound(rec, signedInboundRequest(t, inboundKeywordForm("STOP")))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, []sms.InboundKeyword{{
		ProviderMessageID: "SM123",
		FromE164:          "+15550001111",
		ToE164:            "+15550002222",
		Keyword:           "STOP",
		ReceivedAt:        time.Time{},
	}}, consumer.keywords)
}

// This fails if a retry of the same valid Twilio keyword callback is silently
// dropped before the injected downstream consumer can apply its durable policy.
func TestInbound_HandsEachRepeatedValidCallbackToConsumer(t *testing.T) {
	consumer := &inboundConsumer{}
	handler := newInboundTestHandler(t, consumer, time.Time{})
	form := inboundKeywordForm("STOP")

	for range 2 {
		rec := httptest.NewRecorder()
		handler.Inbound(rec, signedInboundRequest(t, form))
		require.Equal(t, http.StatusNoContent, rec.Code)
	}

	require.Equal(t, []sms.InboundKeyword{
		{ProviderMessageID: "SM123", FromE164: "+15550001111", ToE164: "+15550002222", Keyword: "STOP", ReceivedAt: time.Time{}},
		{ProviderMessageID: "SM123", FromE164: "+15550001111", ToE164: "+15550002222", Keyword: "STOP", ReceivedAt: time.Time{}},
	}, consumer.keywords)
}

// This fails if a recognized callback without a downstream consumer panics or
// acknowledges a callback it cannot hand off.
func TestInbound_FailsClosedWithoutConsumer(t *testing.T) {
	var typedNilConsumer *inboundConsumer
	for _, test := range []struct {
		name     string
		consumer sms.CallbackConsumer
	}{
		{name: "nil interface", consumer: nil},
		{name: "typed nil", consumer: typedNilConsumer},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newInboundTestHandler(t, test.consumer, time.Time{})
			rec := httptest.NewRecorder()

			handler.Inbound(rec, signedInboundRequest(t, inboundKeywordForm("STOP")))
			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	}
}

func newInboundTestHandler(t *testing.T, consumer sms.CallbackConsumer, now time.Time) *Handler {
	t.Helper()
	handler, err := NewHandler(newSignatureTestConfig(), consumer)
	require.NoError(t, err)
	handler.now = func() time.Time { return now }
	return handler
}

func inboundKeywordForm(body string) url.Values {
	return url.Values{
		"MessageSid": {"SM123"},
		"From":       {"+15550001111"},
		"To":         {"+15550002222"},
		"Body":       {body},
	}
}

func signedInboundRequest(t *testing.T, form url.Values) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/twilio/inbound", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", twilioFormSignature(testPublicBaseURL+req.URL.EscapedPath(), form))
	return req
}
