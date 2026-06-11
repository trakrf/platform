// Package mustering is the TRA-978 mustering engine. It sits on the same ingest
// fan-out seam as geofence (ingest.ReadEvaluator): after the subscriber derives
// asset_scans for the membership-passing reads of a message, it hands those
// resolved reads here. The engine tracks per-org in-memory presence for
// person-assets and, while a muster event is active, transitions a person's
// entry to "at muster" the moment they are read at a muster-point location.
//
// All persistence is org-scoped via storage (RLS). SSE deltas are pushed through
// a broadcaster (see broadcast.go). Single-replica only — the in-memory presence
// and active-event caches are per-process, same constraint as geofence/readstream
// (TRA-907).
package mustering

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/muster"
	"github.com/trakrf/platform/backend/internal/storage"
)

// cacheTTL bounds how long person-ness and muster-point/active-event caches are
// trusted before a lazy refresh. Short enough that seeding/CRUD changes take
// effect within a demo cycle, long enough to avoid a DB round-trip per read.
const cacheTTL = 30 * time.Second

// presenceCoalesce is the minimum interval between presence broadcasts per org.
// Presence deltas are idempotent (full headcount snapshot), so coalescing keeps
// a noisy reader from flooding subscribers.
const presenceCoalesce = 1 * time.Second

// engineStore is the storage surface the engine needs; *storage.Storage
// satisfies it. Narrowed so engine_test.go can inject a fake.
type engineStore interface {
	ListPersonPresence(ctx context.Context, orgID int, window time.Duration) ([]muster.PersonPresence, error)
	ListPersonAssetIDs(ctx context.Context, orgID int) ([]int, error)
	ListZones(ctx context.Context, orgID int) ([]muster.ZonePresence, error)
	ListMusterPointIDs(ctx context.Context, orgID int) ([]int, error)
	CreateMusterEvent(ctx context.Context, orgID, startedBy, windowMinutes int) (*muster.Event, error)
	GetActiveMusterEvent(ctx context.Context, orgID int) (*muster.Event, error)
	GetMusterEvent(ctx context.Context, orgID int, id int) (*muster.Event, error)
	ListMusterEvents(ctx context.Context, orgID int) ([]muster.Event, error)
	MarkEntryAtMuster(ctx context.Context, orgID int, eventID int, assetID int, musterLocationID int, seenAt time.Time) (*muster.Entry, error)
	UpdateEntryStatus(ctx context.Context, orgID int, eventID, entryID int, action string, userID int, note string) (*muster.Entry, error)
	CompleteMusterEvent(ctx context.Context, orgID int, eventID int, status string, report json.RawMessage) (*muster.Event, error)
	AppendMusterUnlock(ctx context.Context, orgID int, eventID int, unlock map[string]any) error
}

// broadcaster receives engine deltas for fan-out over SSE. *Broadcaster
// satisfies it; engine_test.go injects a fake to assert on emitted events.
type broadcaster interface {
	BroadcastSnapshot(orgID int, payload SnapshotPayload)
	BroadcastPresence(orgID int, payload PresencePayload)
	BroadcastEntry(orgID int, entry muster.Entry, counts muster.Counts)
	BroadcastEvent(orgID int, ev muster.Event)
}

// personState is one person-asset's last-known presence.
type personState struct {
	locationID *int
	lastSeen   time.Time
}

// orgState is the per-org in-memory cache.
type orgState struct {
	mu sync.Mutex

	// presence: person asset_id -> last-known location + time. Hydrated lazily
	// from ListPersonPresence on first touch.
	presence       map[int]personState
	presenceLoaded bool

	// personSet caches which asset ids are persons; refreshed with the presence
	// hydration (a person appears in ListPersonPresence) plus on cache miss.
	personSet     map[int]struct{}
	personSetAt   time.Time
	musterPoints  map[int]struct{}
	musterPointAt time.Time

	// active event id (0 = none), and the expected-asset set for that event.
	activeEventID int
	expectedSet   map[int]struct{}
	activeLoaded  bool

	lastPresenceBroadcast time.Time
}

func newOrgState() *orgState {
	return &orgState{
		presence:     map[int]personState{},
		personSet:    map[int]struct{}{},
		musterPoints: map[int]struct{}{},
		expectedSet:  map[int]struct{}{},
	}
}

// Engine implements ingest.ReadEvaluator and owns the muster lifecycle.
type Engine struct {
	store engineStore
	bc    broadcaster
	log   zerolog.Logger
	now   func() time.Time

	mu     sync.Mutex
	states map[int]*orgState
}

// NewEngine builds an engine over real storage + broadcaster.
func NewEngine(store *storage.Storage, bc *Broadcaster, log *zerolog.Logger) *Engine {
	return newEngine(store, bc, log)
}

// newEngine is the test-friendly constructor over the narrow interfaces.
func newEngine(store engineStore, bc broadcaster, log *zerolog.Logger) *Engine {
	return &Engine{
		store:  store,
		bc:     bc,
		log:    log.With().Str("component", "mustering").Logger(),
		now:    time.Now,
		states: map[int]*orgState{},
	}
}

// state returns (creating if needed) the per-org state.
func (e *Engine) state(orgID int) *orgState {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.states[orgID]
	if st == nil {
		st = newOrgState()
		e.states[orgID] = st
	}
	return st
}

// ── ingest.ReadEvaluator ──────────────────────────────────────────────────────

// Evaluate folds one message's membership-passing reads into per-org presence
// and, while an event is active, transitions expected persons to at_muster when
// read at a muster point. Never returns an error: side effects are best-effort.
func (e *Engine) Evaluate(ctx context.Context, orgID int, _ int64, receivedAt time.Time, reads []storage.ResolvedRead) {
	st := e.state(orgID)
	st.mu.Lock()
	e.ensurePresenceLoaded(ctx, orgID, st)
	e.ensureMusterPointsLoaded(ctx, orgID, st)
	e.ensureActiveLoaded(ctx, orgID, st)

	presenceChanged := false

	// candidate is a muster-point transition collected under st.mu and applied via
	// MarkEntryAtMuster *after* the lock is released — that DB call is synchronous,
	// so holding st.mu across it would serialize every Evaluate/Status/Activate for
	// the org behind DB latency. Doing the write outside the lock is race-safe
	// because the SQL UPDATE is guarded by `AND status='missing'` (sticky semantics
	// enforced DB-side): a concurrent duplicate candidate just no-ops (returns nil).
	// Mirrors the geofence engine's best-effort-outside-lock precedent.
	type candidate struct {
		assetID    int
		locationID int
	}
	var candidates []candidate
	activeEventID := 0

	for _, rd := range reads {
		// Only person-assets participate in muster presence.
		if !e.isPerson(ctx, orgID, st, rd.AssetID) {
			continue
		}

		// Update presence. A nil location keeps the previous zone.
		prev := st.presence[rd.AssetID]
		loc := rd.LocationID
		if loc == nil {
			loc = prev.locationID
		}
		st.presence[rd.AssetID] = personState{locationID: loc, lastSeen: receivedAt}
		presenceChanged = true

		// Muster-point transition only while an event is active and the asset is
		// in the expected set. Collect the candidate; the DB write happens below,
		// outside the lock.
		if st.activeEventID == 0 || rd.LocationID == nil {
			continue
		}
		if _, ok := st.musterPoints[*rd.LocationID]; !ok {
			continue
		}
		if _, ok := st.expectedSet[rd.AssetID]; !ok {
			continue
		}
		activeEventID = st.activeEventID
		candidates = append(candidates, candidate{assetID: rd.AssetID, locationID: *rd.LocationID})
	}

	st.mu.Unlock()

	// Apply muster-point transitions via the synchronous DB write, outside st.mu.
	var transitioned []*muster.Entry
	for _, c := range candidates {
		entry, err := e.store.MarkEntryAtMuster(ctx, orgID, activeEventID, c.assetID, c.locationID, receivedAt)
		if err != nil {
			e.log.Error().Err(err).Int("org_id", orgID).Int("asset_id", c.assetID).Msg("MarkEntryAtMuster failed")
			continue
		}
		if entry != nil { // real transition (nil == already non-missing, no-op)
			transitioned = append(transitioned, entry)
		}
	}

	// Broadcast entry transitions + refreshed counts (outside the lock). Counts are
	// identical across a batch, so fetch the active event once and reuse it.
	if len(transitioned) > 0 {
		var counts muster.Counts
		if ev, err := e.store.GetActiveMusterEvent(ctx, orgID); err == nil && ev != nil {
			counts = ev.Counts
		}
		for _, entry := range transitioned {
			e.bc.BroadcastEntry(orgID, *entry, counts)
		}
	}

	if presenceChanged {
		e.maybeBroadcastPresence(ctx, orgID, st, activeEventID != 0)
	}
}

// ── lazy cache hydration (caller holds st.mu) ─────────────────────────────────

func (e *Engine) ensurePresenceLoaded(ctx context.Context, orgID int, st *orgState) {
	// Person-ness is metadata-driven, independent of any sighting window, so a
	// freshly-created person (no scans yet) is recognized immediately. Refreshed
	// on a cacheTTL cadence so newly-seeded persons appear within the demo cycle.
	e.refreshPersonSet(ctx, orgID, st)

	if st.presenceLoaded {
		return
	}
	// Hydrate last-known location/time from the widest reasonable window so
	// headcounts are warm; the engine keeps presence live thereafter.
	persons, err := e.store.ListPersonPresence(ctx, orgID, 15*time.Minute)
	if err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Msg("presence hydration failed")
		return
	}
	for _, p := range persons {
		st.presence[p.AssetID] = personState{locationID: p.LocationID, lastSeen: p.LastSeenAt}
	}
	st.presenceLoaded = true
}

// refreshPersonSet repopulates the person-ness cache from ListPersonAssetIDs at
// most once per cacheTTL. Caller holds st.mu.
func (e *Engine) refreshPersonSet(ctx context.Context, orgID int, st *orgState) {
	if !st.personSetAt.IsZero() && e.now().Sub(st.personSetAt) < cacheTTL {
		return
	}
	ids, err := e.store.ListPersonAssetIDs(ctx, orgID)
	if err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Msg("person-set hydration failed")
		return
	}
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	st.personSet = set
	st.personSetAt = e.now()
}

func (e *Engine) ensureMusterPointsLoaded(ctx context.Context, orgID int, st *orgState) {
	if !st.musterPointAt.IsZero() && e.now().Sub(st.musterPointAt) < cacheTTL {
		return
	}
	ids, err := e.store.ListMusterPointIDs(ctx, orgID)
	if err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Msg("muster-point hydration failed")
		return
	}
	mp := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		mp[id] = struct{}{}
	}
	st.musterPoints = mp
	st.musterPointAt = e.now()
}

func (e *Engine) ensureActiveLoaded(ctx context.Context, orgID int, st *orgState) {
	if st.activeLoaded {
		return
	}
	ev, err := e.store.GetActiveMusterEvent(ctx, orgID)
	if err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Msg("active-event hydration failed")
		return
	}
	st.setActive(ev)
	st.activeLoaded = true
}

// setActive records (or clears) the cached active event + expected set.
func (st *orgState) setActive(ev *muster.Event) {
	if ev == nil || ev.Status != "active" {
		st.activeEventID = 0
		st.expectedSet = map[int]struct{}{}
		return
	}
	st.activeEventID = ev.ID
	exp := make(map[int]struct{}, len(ev.Entries))
	for _, en := range ev.Entries {
		exp[en.AssetID] = struct{}{}
	}
	st.expectedSet = exp
}

// isPerson reports whether assetID is a person-asset, refreshing the cache on a
// miss (a newly-seeded person seen before the next presence hydration). Caller
// holds st.mu.
func (e *Engine) isPerson(ctx context.Context, orgID int, st *orgState, assetID int) bool {
	if _, ok := st.personSet[assetID]; ok {
		return true
	}
	// Miss: refresh from storage at most every cacheTTL (covers a person seeded
	// since the last hydration), then re-check. The throttle avoids a DB round-
	// trip per read for a genuinely-unknown (non-person) asset.
	e.refreshPersonSet(ctx, orgID, st)
	_, ok := st.personSet[assetID]
	return ok
}

// ── presence broadcast ────────────────────────────────────────────────────────

// maybeBroadcastPresence coalesces presence headcount broadcasts to at most one
// per presenceCoalesce window per org.
func (e *Engine) maybeBroadcastPresence(ctx context.Context, orgID int, st *orgState, eventActive bool) {
	st.mu.Lock()
	if e.now().Sub(st.lastPresenceBroadcast) < presenceCoalesce {
		st.mu.Unlock()
		return
	}
	st.lastPresenceBroadcast = e.now()
	st.mu.Unlock()

	zones, persons := e.computePresence(ctx, orgID, st, eventActive)
	e.bc.BroadcastPresence(orgID, PresencePayload{Zones: zones, Persons: persons})
}

// computePresence builds zone headcounts (from live locations + in-memory
// presence) and, only while an event is active, the per-person location list
// (break-glass).
func (e *Engine) computePresence(ctx context.Context, orgID int, st *orgState, eventActive bool) ([]muster.ZonePresence, []muster.PersonPresence) {
	zones, err := e.store.ListZones(ctx, orgID)
	if err != nil {
		e.log.Error().Err(err).Int("org_id", orgID).Msg("ListZones failed")
		zones = nil
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	counts := map[int]int{}
	cutoff := e.now().Add(-15 * time.Minute)
	var persons []muster.PersonPresence
	for assetID, ps := range st.presence {
		if ps.lastSeen.Before(cutoff) {
			continue
		}
		if ps.locationID != nil {
			counts[*ps.locationID]++
		}
		if eventActive {
			persons = append(persons, muster.PersonPresence{
				AssetID:    assetID,
				LocationID: ps.locationID,
				LastSeenAt: ps.lastSeen,
			})
		}
	}
	for i := range zones {
		zones[i].Count = counts[zones[i].LocationID]
	}
	return zones, persons
}

// ── lifecycle (called by handlers) ────────────────────────────────────────────

// Activate creates a new muster event (snapshotting the expected set) and
// broadcasts it. Returns ErrActiveEventExists (mapped to 409 by the handler)
// when one is already active.
func (e *Engine) Activate(ctx context.Context, orgID, userID, windowMinutes int) (*muster.Event, error) {
	if windowMinutes <= 0 {
		windowMinutes = 15
	}
	ev, err := e.store.CreateMusterEvent(ctx, orgID, userID, windowMinutes)
	if err != nil {
		return nil, err
	}
	st := e.state(orgID)
	st.mu.Lock()
	st.setActive(ev)
	st.activeLoaded = true
	st.mu.Unlock()

	e.bc.BroadcastEvent(orgID, *ev)
	return ev, nil
}

// AllClear computes the report, completes the event, and broadcasts the
// terminal event. Returns (nil, nil) when the event is absent / wrong org.
func (e *Engine) AllClear(ctx context.Context, orgID, eventID, _ int) (*muster.Event, error) {
	ev, err := e.store.GetMusterEvent(ctx, orgID, eventID)
	if err != nil {
		return nil, err
	}
	if ev == nil {
		return nil, nil
	}
	report := e.computeReport(ctx, orgID, ev)
	updated, err := e.store.CompleteMusterEvent(ctx, orgID, eventID, "completed", report)
	if err != nil {
		return nil, err
	}
	// Carry entries/counts onto the returned event for the response + broadcast.
	updated.Entries = ev.Entries
	updated.Counts = ev.Counts
	e.clearActive(orgID, eventID)
	e.bc.BroadcastEvent(orgID, *updated)
	return updated, nil
}

// Cancel ends the event without a report. Returns (nil, nil) when absent.
func (e *Engine) Cancel(ctx context.Context, orgID, eventID, _ int) (*muster.Event, error) {
	ev, err := e.store.GetMusterEvent(ctx, orgID, eventID)
	if err != nil {
		return nil, err
	}
	if ev == nil {
		return nil, nil
	}
	updated, err := e.store.CompleteMusterEvent(ctx, orgID, eventID, "cancelled", nil)
	if err != nil {
		return nil, err
	}
	updated.Entries = ev.Entries
	updated.Counts = ev.Counts
	e.clearActive(orgID, eventID)
	e.bc.BroadcastEvent(orgID, *updated)
	return updated, nil
}

// clearActive drops the cached active event if it matches eventID.
func (e *Engine) clearActive(orgID, eventID int) {
	st := e.state(orgID)
	st.mu.Lock()
	if st.activeEventID == eventID {
		st.setActive(nil)
	}
	st.mu.Unlock()
}

// Verify / MarkSafe apply an explicit entry transition, persist, and broadcast
// the updated entry + refreshed counts. ErrInvalidTransition → 409; (nil,nil)
// → 404 (entry absent / wrong org).
func (e *Engine) Verify(ctx context.Context, orgID, eventID, entryID, userID int) (*muster.Entry, *muster.Counts, error) {
	return e.applyEntry(ctx, orgID, eventID, entryID, "verify", userID, "")
}

func (e *Engine) MarkSafe(ctx context.Context, orgID, eventID, entryID, userID int, note string) (*muster.Entry, *muster.Counts, error) {
	return e.applyEntry(ctx, orgID, eventID, entryID, "mark_safe", userID, note)
}

func (e *Engine) applyEntry(ctx context.Context, orgID, eventID, entryID int, action string, userID int, note string) (*muster.Entry, *muster.Counts, error) {
	entry, err := e.store.UpdateEntryStatus(ctx, orgID, eventID, entryID, action, userID, note)
	if err != nil {
		return nil, nil, err
	}
	if entry == nil {
		return nil, nil, nil
	}
	var counts muster.Counts
	if ev, err := e.store.GetMusterEvent(ctx, orgID, eventID); err == nil && ev != nil {
		counts = ev.Counts
	}
	e.bc.BroadcastEntry(orgID, *entry, counts)
	return entry, &counts, nil
}

// Unlock appends a break-glass reveal record to the event metadata. Idempotent
// from the caller's perspective (each reveal is logged).
func (e *Engine) Unlock(ctx context.Context, orgID, eventID, userID int, email string) error {
	return e.store.AppendMusterUnlock(ctx, orgID, eventID, map[string]any{
		"user_id": userID,
		"email":   email,
		"at":      e.now().UTC().Format(time.RFC3339),
	})
}

// Status returns the full snapshot payload: zone headcounts + the active event
// (with entries/counts) or nil.
func (e *Engine) Status(ctx context.Context, orgID int) (SnapshotPayload, error) {
	st := e.state(orgID)
	st.mu.Lock()
	e.ensurePresenceLoaded(ctx, orgID, st)
	e.ensureActiveLoaded(ctx, orgID, st)
	eventActive := st.activeEventID != 0
	st.mu.Unlock()

	zones, _ := e.computePresence(ctx, orgID, st, eventActive)
	onsite := 0
	for _, z := range zones {
		if !z.MusterPoint {
			onsite += z.Count
		}
	}

	ev, err := e.store.GetActiveMusterEvent(ctx, orgID)
	if err != nil {
		return SnapshotPayload{}, err
	}
	// Break-glass: while an event is active, enrich each entry with its person's
	// last-known location from in-memory presence (last_seen_location_id /
	// last_seen_at). The API exposes this only while an event is active per spec;
	// the UI additionally gates the display behind an explicit unlock.
	if ev != nil {
		e.enrichEntriesWithPresence(orgID, ev)
	}
	return SnapshotPayload{Zones: zones, PersonsOnSite: onsite, Event: ev}, nil
}

// enrichEntriesWithPresence fills last_seen_location_id / last_seen_at on each
// entry from the engine's in-memory per-person presence. Used only for an active
// event (break-glass per-person location).
func (e *Engine) enrichEntriesWithPresence(orgID int, ev *muster.Event) {
	st := e.state(orgID)
	st.mu.Lock()
	defer st.mu.Unlock()
	for i := range ev.Entries {
		ps, ok := st.presence[ev.Entries[i].AssetID]
		if !ok {
			continue
		}
		ev.Entries[i].LastSeenLocationID = ps.locationID
		if !ps.lastSeen.IsZero() {
			t := ps.lastSeen
			ev.Entries[i].LastSeenAt = &t
		}
	}
}

// ── report ────────────────────────────────────────────────────────────────────

// computeReport builds the all-clear report JSON per the plan's exact shape.
func (e *Engine) computeReport(ctx context.Context, orgID int, ev *muster.Event) json.RawMessage {
	type zoneReport struct {
		LocationID int        `json:"location_id"`
		Name       string     `json:"name"`
		Expected   int        `json:"expected"`
		Accounted  int        `json:"accounted"`
		ClearedAt  *time.Time `json:"cleared_at"`
	}
	type mpReport struct {
		LocationID int    `json:"location_id"`
		Name       string `json:"name"`
		Arrivals   int    `json:"arrivals"`
	}
	type report struct {
		TotalSeconds int           `json:"total_seconds"`
		Counts       muster.Counts `json:"counts"`
		Zones        []zoneReport  `json:"zones"`
		MusterPoints []mpReport    `json:"muster_points"`
	}

	// Location names for zones + muster points.
	names := map[int]string{}
	musterPoint := map[int]bool{}
	if zones, err := e.store.ListZones(ctx, orgID); err == nil {
		for _, z := range zones {
			names[z.LocationID] = z.Name
			musterPoint[z.LocationID] = z.MusterPoint
		}
	}

	// Group entries by expected_location_id; track accounted + max accounted ts.
	type agg struct {
		expected  int
		accounted int
		maxTS     *time.Time
	}
	zoneAgg := map[int]*agg{}
	mpArrivals := map[int]int{}

	for i := range ev.Entries {
		en := ev.Entries[i]
		zoneKey := 0
		if en.ExpectedLocationID != nil {
			zoneKey = *en.ExpectedLocationID
		}
		a := zoneAgg[zoneKey]
		if a == nil {
			a = &agg{}
			zoneAgg[zoneKey] = a
		}
		a.expected++

		accounted := en.Status != "missing"
		if accounted {
			a.accounted++
			if ts := accountedTimestamp(en); ts != nil {
				if a.maxTS == nil || ts.After(*a.maxTS) {
					a.maxTS = ts
				}
			}
		}
		// Muster-point arrivals: any entry that reached a muster point.
		if en.MusterLocationID != nil {
			mpArrivals[*en.MusterLocationID]++
		}
	}

	var zones []zoneReport
	for locID, a := range zoneAgg {
		zr := zoneReport{
			LocationID: locID,
			Name:       names[locID],
			Expected:   a.expected,
			Accounted:  a.accounted,
		}
		// cleared_at = max accounted timestamp only when fully accounted.
		if a.accounted == a.expected {
			zr.ClearedAt = a.maxTS
		}
		zones = append(zones, zr)
	}

	var mps []mpReport
	for locID, arrivals := range mpArrivals {
		mps = append(mps, mpReport{LocationID: locID, Name: names[locID], Arrivals: arrivals})
	}

	total := 0
	if ev.StartedAt.Before(e.now()) {
		total = int(e.now().Sub(ev.StartedAt).Seconds())
	}

	rep := report{
		TotalSeconds: total,
		Counts:       ev.Counts,
		Zones:        zones,
		MusterPoints: mps,
	}
	data, err := json.Marshal(rep)
	if err != nil {
		return nil
	}
	return data
}

// accountedTimestamp returns the timestamp at which an entry became accounted:
// verified_at, marked_safe_at, or first_muster_seen_at, whichever applies.
func accountedTimestamp(en muster.Entry) *time.Time {
	switch en.Status {
	case "verified":
		if en.VerifiedAt != nil {
			return en.VerifiedAt
		}
	case "safe_manual":
		if en.MarkedSafeAt != nil {
			return en.MarkedSafeAt
		}
	}
	return en.FirstMusterSeenAt
}
