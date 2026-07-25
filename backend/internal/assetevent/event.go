// Package assetevent detects asset-level domain events from the scan stream and
// fans them out to Sinks (TRA-1043).
//
// It owns detection — the previous-location lookup, the delta comparison, and
// the name enrichment — and knows nothing about how an event is delivered. The
// webhook package supplies the one Phase 1 Sink; email/SMS notification
// (TRA-1044) plugs in as a second Sink without touching anything here.
//
// The split exists because the two things have different shapes: webhooks are
// org-scoped and org-addressed, while notifying "the owner of that boat" needs
// per-asset recipient routing. Extracting detection out of a live webhook
// package later would be far more expensive than an interface and a package
// name now.
//
// Detection hangs off ingest.MultiEvaluator for the MQTT path and off a
// post-commit call for the handheld path. Everything here is best-effort: an
// event is never allowed to block, slow, or fail a scan write.
package assetevent

import (
	"context"
	"time"
)

// EventAssetMoved is the wire name of the only Phase 1 event type.
const EventAssetMoved = "asset.moved"

// Location is a location's logical identity. No scan point, no antenna — the
// physical layer stays internal.
type Location struct {
	ID   int
	Name string
}

// Asset is an asset's logical identity: the obfuscated surrogate id (wire
// canonical) plus the natural key and display name.
type Asset struct {
	ID          int
	ExternalKey string
	Name        string
}

// AssetMoved is the domain event: an asset was scanned at a location different
// from the last one it was seen at. A rescan at the same location produces
// nothing at all — this is a pure delta, not a heartbeat.
//
// Sinks project this onto their own wire format; the struct itself carries no
// transport concerns.
type AssetMoved struct {
	// DeliveryID identifies this delivery attempt set (one uuid per detected
	// move, stable across the retry attempts of a single send).
	DeliveryID string
	// OccurredAt is the scan instant that produced the move — server time, the
	// same value written to asset_scans.timestamp.
	//
	// There is deliberately no sequence number: concurrent MQTT messages mean
	// deliveries can arrive out of order, and shipping a counter would imply an
	// ordering guarantee we do not provide.
	OccurredAt time.Time
	OrgID      int
	Asset      Asset
	// From is the location the asset was last seen at. nil only for a genuine
	// first-ever sighting (or a prior sighting that carried no location).
	From *Location
	To   Location
}

// Sink delivers a domain event somewhere outside the process. Implementations
// must be safe for concurrent use: the dispatcher calls Deliver from several
// workers at once.
//
// A Sink returns nil for "nothing to do" (no webhook registered, org not
// entitled, delivery disabled) as well as for success — a nil error means the
// dispatcher should not retry. Only a genuine transient failure warrants an
// error return.
type Sink interface {
	Deliver(ctx context.Context, ev AssetMoved) error
}
