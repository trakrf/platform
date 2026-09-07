package twiliosms

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
)

const (
	testAuthToken     = "task-6-auth-token"
	testPublicBaseURL = "https://callbacks.example.com"
)

func newSignatureTestConfig() twilio.Config {
	return twilio.Config{
		AccountSID:          "AC123",
		APIKeySID:           "SK123",
		APIKeySecret:        "api-key-secret",
		AuthToken:           testAuthToken,
		MessagingServiceSID: "MG123",
		PublicBaseURL:       testPublicBaseURL,
	}
}

func newSignatureTestHandler(t *testing.T) *Handler {
	t.Helper()
	handler, err := NewHandler(newSignatureTestConfig(), nil)
	require.NoError(t, err)
	return handler
}

// This fails if callback validation uses the proxy-provided authority, loses
// an escaped request path, or normalizes the raw query before validating.
func TestSignature_AcceptsSignedFormAgainstConfiguredPublicURL(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"http://internal-proxy:8080/api/v1/notifications/twilio/status/%E2%9C%93?trace=a%2Bb&z=last",
		strings.NewReader("MessageSid=SM123&Body=hello%2Bworld"),
	)
	req.Host = "internal-proxy:8080"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("X-Twilio-Signature", "p5QOW5MVdWNIHIgvKft7Y9TsP4E=")

	form, err := newSignatureTestHandler(t).verifiedForm(httptest.NewRecorder(), req)

	require.NoError(t, err)
	require.Equal(t, "SM123", form.Get("MessageSid"))
	require.Equal(t, "hello+world", form.Get("Body"))
}

// This fails if an arbitrary signature is accepted as a Twilio callback.
func TestSignature_RejectsInvalidSignature(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/notifications/twilio/status",
		strings.NewReader("MessageSid=SM123"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", "not-a-valid-signature")

	form, err := newSignatureTestHandler(t).verifiedForm(httptest.NewRecorder(), req)

	require.Error(t, err)
	require.Nil(t, form)
}

// This fails if callbacks without the Twilio signature header are accepted.
func TestSignature_RejectsMissingSignature(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/notifications/twilio/status",
		strings.NewReader("MessageSid=SM123"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	form, err := newSignatureTestHandler(t).verifiedForm(httptest.NewRecorder(), req)

	require.Error(t, err)
	require.Nil(t, form)
}

// This fails if a signature covering a first form value permits an appended,
// unsigned value for the same key.
func TestSignature_RejectsDuplicateFormValuesBeforeValidation(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/notifications/twilio/status",
		strings.NewReader("MessageSid=SM123&MessageSid=unsigned-duplicate"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", "GEeHG+TlwALBCutqb6698wCo8pI=")

	form, err := newSignatureTestHandler(t).verifiedForm(httptest.NewRecorder(), req)

	require.ErrorIs(t, err, errMalformedForm)
	require.Nil(t, form)
}

// This fails if disabled, partial, or non-canonical configuration can create
// a callback boundary that later route wiring could use.
func TestNewHandler_RejectsConfigurationThatCannotSecureCallbacks(t *testing.T) {
	completeConfig := newSignatureTestConfig()

	for _, test := range []struct {
		name   string
		config twilio.Config
	}{
		{name: "disabled", config: twilio.Config{}},
		{name: "partial", config: twilio.Config{AuthToken: testAuthToken, PublicBaseURL: testPublicBaseURL}},
		{name: "empty auth token", config: twilio.Config{AccountSID: "AC123", APIKeySID: "SK123", APIKeySecret: "api-key-secret", MessagingServiceSID: "MG123", PublicBaseURL: testPublicBaseURL}},
		{name: "non-canonical origin", config: func() twilio.Config {
			c := completeConfig
			c.PublicBaseURL = "https://callbacks.example.com/"
			return c
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, err := NewHandler(test.config, nil)

			require.Error(t, err)
			require.Nil(t, handler)
			require.NotContains(t, err.Error(), testAuthToken)
			require.NotContains(t, err.Error(), "api-key-secret")
		})
	}
}

// This fails if malformed or oversized form bodies are accepted, or if their
// contents are exposed through a parse error.
func TestSignature_RejectsMalformedOrOversizedForm(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "malformed escape", body: "Body=%ZZ"},
		{name: "oversized", body: "Body=" + strings.Repeat("sensitive-input-", 1<<17)},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/twilio/status", strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("X-Twilio-Signature", "not-a-valid-signature")

			form, err := newSignatureTestHandler(t).verifiedForm(httptest.NewRecorder(), req)

			require.Error(t, err)
			require.Nil(t, form)
			require.NotContains(t, err.Error(), "sensitive-input-")
		})
	}
}
