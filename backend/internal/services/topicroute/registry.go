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

// EntitlementChecker reports whether an org is entitled (satisfied by
// *storage.Storage). Optional: only used to explain why a topic was dropped.
type EntitlementChecker interface {
	OrgIsEntitled(ctx context.Context, orgID int) (bool, error)
}

// Registry is the process-wide topic->route map and subscription set.
type Registry struct {
	lister      TopicLister
	log         zerolog.Logger
	mu          sync.RWMutex
	routes      map[string]storage.ScanRoute
	mgr         SubscriptionManager
	entitlement EntitlementChecker
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

// SetEntitlementChecker lets Reconcile tell a subscription cutoff apart from an
// ordinary device removal when a topic drops out of the active list.
func (r *Registry) SetEntitlementChecker(c EntitlementChecker) {
	r.mu.Lock()
	r.entitlement = c
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
	removed := map[string]storage.ScanRoute{}
	r.mu.Lock()
	for topic, route := range r.routes {
		if _, ok := fresh[topic]; !ok {
			delete(r.routes, topic)
			toUnsub = append(toUnsub, topic)
			removed[topic] = route
		}
	}
	for topic, route := range fresh {
		if _, ok := r.routes[topic]; !ok {
			toSub = append(toSub, topic)
		}
		r.routes[topic] = route // refresh route even when the topic is unchanged
	}
	mgr := r.mgr
	entitlement := r.entitlement
	r.mu.Unlock()

	if mgr != nil {
		for _, t := range toSub {
			mgr.Subscribe(t)
		}
		for _, t := range toUnsub {
			mgr.Unsubscribe(t)
		}
	}
	if entitlement != nil {
		r.warnCutoffs(ctx, entitlement, removed)
	}
	if len(toSub) > 0 || len(toUnsub) > 0 {
		r.log.Info().Int("added", len(toSub)).Int("removed", len(toUnsub)).Msg("topic registry reconciled")
	}
	return nil
}

// warnCutoffs logs, at WARN, each dropped topic whose org is no longer entitled.
// The reader behind it keeps publishing and sees no error while the broker
// discards its reads, so this is the operator's signal that it is not a fault.
func (r *Registry) warnCutoffs(ctx context.Context, c EntitlementChecker, removed map[string]storage.ScanRoute) {
	checked := map[int]bool{}
	for topic, route := range removed {
		entitled, seen := checked[route.OrgID]
		if !seen {
			var err error
			entitled, err = c.OrgIsEntitled(ctx, route.OrgID)
			if err != nil {
				r.log.Warn().Err(err).Int("org_id", route.OrgID).Msg("entitlement check failed for dropped topic")
				continue
			}
			checked[route.OrgID] = entitled
		}
		if !entitled {
			r.log.Warn().Str("topic", topic).Int("org_id", route.OrgID).Int("scan_device_id", route.ScanDeviceID).
				Msg("fixed reader unsubscribed: org not entitled; its reads are discarded until reactivation")
		}
	}
}
