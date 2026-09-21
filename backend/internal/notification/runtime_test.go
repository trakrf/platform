package notification

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
)

// An unconfigured deployment must boot without credentials, network calls,
// callback routes, or a sender that could accidentally send traffic.
func TestRuntime_Disabled(t *testing.T) {
	runtime, err := NewRuntime(twilio.Config{}, nil, prometheus.NewRegistry())
	require.NoError(t, err)
	require.Nil(t, runtime.Sender)
	router := chi.NewRouter()
	runtime.RegisterRoutes(router)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/notifications/twilio/status", nil))
	require.Equal(t, 404, rec.Code)
}

func TestRuntime_RequiresCompleteConfigurationAndConsumer(t *testing.T) {
	for _, config := range []twilio.Config{
		{APIKeySID: "SKpartial"},
		runtimeConfig(),
	} {
		runtime, err := NewRuntime(config, nil, prometheus.NewRegistry())
		require.Error(t, err)
		require.Nil(t, runtime)
	}
	var typedNil *runtimeConsumer
	runtime, err := NewRuntime(runtimeConfig(), typedNil, prometheus.NewRegistry())
	require.Error(t, err)
	require.Nil(t, runtime)
}

func TestRuntime_ConfiguredRegistersCallbacksWithoutCallingProvider(t *testing.T) {
	runtime, err := NewRuntime(runtimeConfig(), &runtimeConsumer{}, prometheus.NewRegistry())
	require.NoError(t, err)
	require.NotNil(t, runtime.Sender)
	router := chi.NewRouter()
	runtime.RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/twilio/status", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, 403, rec.Code)
}

type runtimeConsumer struct{}

func (*runtimeConsumer) HandleStatus(context.Context, sms.ProviderStatus) error  { return nil }
func (*runtimeConsumer) HandleKeyword(context.Context, sms.InboundKeyword) error { return nil }

func runtimeConfig() twilio.Config {
	return twilio.Config{AccountSID: "ACtest", APIKeySID: "SKtest", APIKeySecret: "secret", AuthToken: "token", MessagingServiceSID: "MGtest", PublicBaseURL: "https://callbacks.example.com"}
}
