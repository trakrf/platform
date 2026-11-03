package serve

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/notification/dispatch"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/routing"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/storage"
)

// notificationDeliveryDeps are the collaborators of the delivery stack. A nil
// Email or SMS sender disables that channel; the runtime itself is never nil.
type notificationDeliveryDeps struct {
	Store    *storage.Storage
	Pool     *pgxpool.Pool
	Email    email.Sender
	SMS      sms.Sender
	Routing  routing.Config
	Registry prometheus.Registerer
	Log      zerolog.Logger
}

// newNotificationDelivery builds the inserter, routing metrics, one set of
// outbox metrics (shared by the planner and the runtime, since a second
// registration on the same registry fails), the planner, the dispatch adapter
// and the runtime with the trigger worker registered. It starts nothing.
func newNotificationDelivery(d notificationDeliveryDeps) (*outbox.Runtime, error) {
	templates, err := routing.NewTemplateRenderer()
	if err != nil {
		return nil, err
	}
	jobInserter, err := outbox.NewInserter(d.Pool)
	if err != nil {
		return nil, err
	}
	planningMetrics, err := routing.NewMetrics(d.Registry)
	if err != nil {
		return nil, err
	}
	outboxMetrics, err := outbox.NewMetrics(d.Registry)
	if err != nil {
		return nil, err
	}
	planner, err := routing.NewDeliveryPlanner(routing.Deps{
		Store:         d.Store,
		Jobs:          jobInserter,
		Renderer:      templates,
		Metrics:       planningMetrics,
		OutboxMetrics: outboxMetrics,
		PhoneRegion:   d.Routing.PhoneRegion,
		Log:           d.Log,
	})
	if err != nil {
		return nil, err
	}
	adapter, err := dispatch.New(dispatch.Deps{
		Email:        d.Email,
		SMS:          d.SMS,
		Suppressions: d.Store,
		Log:          d.Log,
	})
	if err != nil {
		return nil, err
	}
	return outbox.NewRuntime(d.Pool, adapter, d.Store, outboxMetrics, routing.RegisterTriggerWorker(planner))
}
