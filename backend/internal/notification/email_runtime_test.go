package notification

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/resend"
)

func TestEmailRuntime_Disabled(t *testing.T) {
	runtime, err := NewEmailRuntime(resend.Config{}, nil, nil)
	require.NoError(t, err)
	require.Nil(t, runtime.Sender)
	router := chi.NewRouter()
	runtime.RegisterRoutes(router)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, EmailCallbackPath, nil))
	require.Equal(t, 404, rec.Code)
}

func TestEmailRuntime_ConfigurationAndConsumer(t *testing.T) {
	config := resend.Config{Enabled: true, APIKey: "test-key", From: "notify@example.com", WebhookSecret: "whsec_c2VjcmV0", Timeout: time.Second}
	var typedNil *emailRuntimeConsumer
	for _, consumer := range []email.CallbackConsumer{nil, typedNil} {
		runtime, err := NewEmailRuntime(config, consumer, prometheus.NewRegistry())
		require.Error(t, err)
		require.Nil(t, runtime)
	}
	consumer := &emailRuntimeConsumer{}
	_, err := NewEmailRuntime(resend.Config{Enabled: true}, consumer, prometheus.NewRegistry())
	require.Error(t, err)
	_, err = NewEmailRuntime(config, consumer, nil)
	require.Error(t, err)
	registry := prometheus.NewRegistry()
	runtime, err := NewEmailRuntime(config, consumer, registry)
	require.NoError(t, err)
	require.NotNil(t, runtime.Sender)
	_, err = NewEmailRuntime(config, consumer, registry)
	require.Error(t, err, "duplicate metrics must return an error, not panic")
	router := chi.NewRouter()
	runtime.RegisterRoutes(router)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, EmailCallbackPath, nil))
	require.Equal(t, 403, rec.Code)
}

type emailRuntimeConsumer struct{}

func (*emailRuntimeConsumer) HandleEvent(context.Context, email.CallbackEvent) error { return nil }
