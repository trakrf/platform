package notification

import (
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/trakrf/platform/backend/internal/handlers/resendemail"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/resend"
)

const EmailCallbackPath = "/api/v1/notifications/resend/events"

// RouteRegistrar lets the backend compose independent notification providers.
type RouteRegistrar interface{ RegisterRoutes(chi.Router) }

// EmailRuntime exposes the provider-neutral sender for future workflows and
// owns verified callbacks. Construction never sends; disabled means no sender
// and no route. Transactional email and SMS have independent lifecycles.
type EmailRuntime struct {
	Sender    email.Sender
	callbacks *resendemail.Handler
}

func NewEmailRuntime(config resend.Config, consumer email.CallbackConsumer, registry prometheus.Registerer) (*EmailRuntime, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return &EmailRuntime{}, nil
	}
	callbacks, err := resendemail.NewHandler(config, consumer)
	if err != nil {
		return nil, err
	}
	sender, err := resend.NewSender(config)
	if err != nil {
		return nil, err
	}
	metrics, err := resend.NewMetrics(registry)
	if err != nil {
		return nil, err
	}
	return &EmailRuntime{Sender: &observedEmailSender{sender: sender, metrics: metrics}, callbacks: callbacks.WithMetrics(metrics)}, nil
}

func (runtime *EmailRuntime) RegisterRoutes(router chi.Router) {
	if runtime != nil && runtime.callbacks != nil {
		router.Handle(EmailCallbackPath, runtime.callbacks)
	}
}
