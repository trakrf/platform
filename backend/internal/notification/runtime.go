// Package notification composes the SMS provider and its callback receiver.
package notification

import (
	"errors"
	"reflect"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/trakrf/platform/backend/internal/handlers/twiliosms"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
)

// Runtime owns a configured SMS sender and signed callback routes. A disabled
// runtime has no sender and registers no routes. Notification workflows inject
// Sender into their delivery worker; construction itself never sends traffic.
type Runtime struct {
	Sender    sms.Sender
	callbacks *twiliosms.Handler
}

func NewRuntime(config twilio.Config, consumer sms.CallbackConsumer, registry prometheus.Registerer) (*Runtime, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled() {
		return &Runtime{}, nil
	}
	if missingConsumer(consumer) {
		return nil, errors.New("SMS callback consumer is required")
	}
	metrics, err := twilio.NewMetrics(registry)
	if err != nil {
		return nil, err
	}
	sender, err := twilio.NewSenderWithMetrics(config, metrics)
	if err != nil {
		return nil, err
	}
	callbacks, err := twiliosms.NewHandlerWithMetrics(config, consumer, metrics)
	if err != nil {
		return nil, err
	}
	return &Runtime{Sender: sender, callbacks: callbacks}, nil
}

func (runtime *Runtime) RegisterRoutes(router chi.Router) {
	if runtime != nil && runtime.callbacks != nil {
		runtime.callbacks.RegisterRoutes(router)
	}
}

func missingConsumer(consumer sms.CallbackConsumer) bool {
	if consumer == nil {
		return true
	}
	v := reflect.ValueOf(consumer)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
