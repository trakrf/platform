package twiliosms

import (
	"errors"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Provider classifications must survive customized keywords and take
// precedence over body fallback; HELP must never change consent.
func TestInbound_UsesVerifiedOptOutType(t *testing.T) {
	for _, tc := range []struct {
		name, body, providerType, want string
		status                         int
	}{
		{"localized stop", "ARRET", "STOP", "STOP", 204},
		{"custom start", "RESUME", "START", "START", 204},
		{"type wins over body", "STOP", "START", "START", 204},
		{"help does not change consent", "STOP", "HELP", "", 204},
		{"unknown type", "START", "CUSTOM", "", 400},
		{"present empty type", "START", "", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			consumer := &inboundConsumer{}
			form := inboundKeywordForm(tc.body)
			form.Set("OptOutType", tc.providerType)
			rec := httptest.NewRecorder()
			newInboundTestHandler(t, consumer, time.Time{}).Inbound(rec, signedInboundRequest(t, form))
			require.Equal(t, tc.status, rec.Code)
			if tc.want == "" {
				require.Empty(t, consumer.keywords)
			} else {
				require.Len(t, consumer.keywords, 1)
				require.Equal(t, tc.want, consumer.keywords[0].Keyword)
			}
		})
	}
}

// Provider-classified custom keywords must pass through the same signature,
// identity, and durable handoff gates as standard body keywords.
func TestInbound_OptOutTypeRequiresValidHandoff(t *testing.T) {
	for _, tc := range []struct {
		name           string
		mutate         func(url.Values)
		forged         bool
		consumerErr    error
		status, events int
	}{
		{name: "missing message", mutate: func(v url.Values) { v.Del("MessageSid") }, status: 400},
		{name: "missing from", mutate: func(v url.Values) { v.Del("From") }, status: 400},
		{name: "missing to", mutate: func(v url.Values) { v.Del("To") }, status: 400},
		{name: "duplicate classification", mutate: func(v url.Values) { v.Add("OptOutType", "START") }, status: 400},
		{name: "forged", forged: true, status: 403},
		{name: "consumer failure", consumerErr: errors.New("storage unavailable"), status: 500, events: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := inboundKeywordForm("ARRET")
			form.Set("OptOutType", "STOP")
			if tc.mutate != nil {
				tc.mutate(form)
			}
			consumer := &inboundConsumer{err: tc.consumerErr}
			req := signedInboundRequest(t, form)
			if tc.forged {
				req.Header.Set("X-Twilio-Signature", "forged")
			}
			rec := httptest.NewRecorder()
			newInboundTestHandler(t, consumer, time.Time{}).Inbound(rec, req)
			require.Equal(t, tc.status, rec.Code)
			require.Len(t, consumer.keywords, tc.events)
		})
	}
}

func TestInbound_RepeatedOptOutTypeReachesConsumer(t *testing.T) {
	consumer := &inboundConsumer{}
	handler := newInboundTestHandler(t, consumer, time.Time{})
	form := inboundKeywordForm("ARRET")
	form.Set("OptOutType", "STOP")
	for range 2 {
		rec := httptest.NewRecorder()
		handler.Inbound(rec, signedInboundRequest(t, form))
		require.Equal(t, 204, rec.Code)
	}
	require.Len(t, consumer.keywords, 2)
}
