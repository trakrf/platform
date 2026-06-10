// Package topicroute owns the in-memory publish_topic -> ScanRoute map used to
// route incoming MQTT reads, AND the broker subscription set those topics imply.
// One structure, two jobs: the set of map keys is exactly the set of topics the
// subscriber subscribes to. Reconcile() re-derives both from the DB, so the
// subscriber subscribes to exactly the registered reads topics instead of
// vacuuming a broker firehose (TRA-922).
package topicroute

import (
	"context"
	"sync"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/storage"
)

// TopicLister is the storage dependency (satisfied by *storage.Storage). Kept as
// an interface so the registry is unit-testable without a live DB.
type TopicLister interface {
	ListScanTopics(ctx context.Context) (map[string]storage.ScanRoute, error)
}

// SubscriptionManager applies subscription deltas to the live broker client.
// Implemented by *ingest.Subscriber; nil until a subscriber attaches (when MQTT
// is disabled the registry is map-only and these are never called).
type SubscriptionManager interface {
	Subscribe(topic string)
	Unsubscribe(topic string)
}

// Registry is the process-wide topic->route map and subscription set.
type Registry struct {
	lister TopicLister
	log    zerolog.Logger
	mu     sync.RWMutex
	routes map[string]storage.ScanRoute
	mgr    SubscriptionManager
}

// NewRegistry builds an empty registry. Call Reconcile to populate it.
func NewRegistry(lister TopicLister, log zerolog.Logger) *Registry {
	return &Registry{
		lister: lister,
		log:    log.With().Str("component", "topicroute").Logger(),
		routes: map[string]storage.ScanRoute{},
	}
}

// SetManager attaches the subscription manager (the MQTT subscriber). Until set,
// Reconcile only maintains the in-memory map.
func (r *Registry) SetManager(m SubscriptionManager) {
	r.mu.Lock()
	r.mgr = m
	r.mu.Unlock()
}

// Lookup returns the route for a topic from the in-memory map (message path).
func (r *Registry) Lookup(topic string) (storage.ScanRoute, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.routes[topic]
	return rt, ok
}

// Topics returns a snapshot of all known topics, for OnConnect bulk-subscribe.
func (r *Registry) Topics() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.routes))
	for t := range r.routes {
		out = append(out, t)
	}
	return out
}

// Reconcile re-derives the map from the DB and applies the add/remove deltas to
// the subscription manager (if attached). Safe to call on boot (no manager =>
// map-only), on scan-device CRUD, and on a periodic ticker — it converges the
// live subscription set to the registered topics either way.
func (r *Registry) Reconcile(ctx context.Context) error {
	fresh, err := r.lister.ListScanTopics(ctx)
	if err != nil {
		return err
	}
	var toSub, toUnsub []string
	r.mu.Lock()
	for topic := range r.routes {
		if _, ok := fresh[topic]; !ok {
			delete(r.routes, topic)
			toUnsub = append(toUnsub, topic)
		}
	}
	for topic, route := range fresh {
		if _, ok := r.routes[topic]; !ok {
			toSub = append(toSub, topic)
		}
		r.routes[topic] = route // refresh route even when the topic is unchanged
	}
	mgr := r.mgr
	r.mu.Unlock()

	if mgr != nil {
		for _, t := range toSub {
			mgr.Subscribe(t)
		}
		for _, t := range toUnsub {
			mgr.Unsubscribe(t)
		}
	}
	if len(toSub) > 0 || len(toUnsub) > 0 {
		r.log.Info().Int("added", len(toSub)).Int("removed", len(toUnsub)).Msg("topic registry reconciled")
	}
	return nil
}
