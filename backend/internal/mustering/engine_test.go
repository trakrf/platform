package mustering

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/muster"
	"github.com/trakrf/platform/backend/internal/storage"
)

func ptr(i int) *int { return &i }

// ── fake store ─────────────────────────────────────────────────────────────────

type fakeStore struct {
	mu sync.Mutex

	persons      []muster.PersonPresence
	zones        []muster.ZonePresence
	musterPoints []int

	active    *muster.Event
	events    map[int]*muster.Event
	nextID    int
	markCalls []markCall
	updated   []muster.Entry
	unlocks   []map[string]any
	completed []completeCall
	createErr error
}

type markCall struct {
	eventID, assetID, locationID int
}
type completeCall struct {
	eventID int
	status  string
	report  json.RawMessage
}

func newFakeStore() *fakeStore {
	return &fakeStore{events: map[int]*muster.Event{}, nextID: 100}
}

func (f *fakeStore) ListPersonPresence(_ context.Context, _ int, _ time.Duration) ([]muster.PersonPresence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]muster.PersonPresence(nil), f.persons...), nil
}
func (f *fakeStore) ListPersonAssetIDs(_ context.Context, _ int) ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]int, 0, len(f.persons))
	for _, p := range f.persons {
		ids = append(ids, p.AssetID)
	}
	return ids, nil
}
func (f *fakeStore) ListZones(_ context.Context, _ int) ([]muster.ZonePresence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]muster.ZonePresence(nil), f.zones...), nil
}
func (f *fakeStore) ListMusterPointIDs(_ context.Context, _ int) ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.musterPoints...), nil
}
func (f *fakeStore) CreateMusterEvent(_ context.Context, orgID, startedBy, windowMinutes int) (*muster.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.nextID++
	ev := &muster.Event{ID: f.nextID, OrgID: orgID, Status: "active", WindowMinutes: windowMinutes, StartedBy: &startedBy, StartedAt: time.Now()}
	for _, p := range f.persons {
		ev.Entries = append(ev.Entries, muster.Entry{
			ID: f.nextID*1000 + p.AssetID, MusterEventID: ev.ID, AssetID: p.AssetID,
			Label: p.Label, ExpectedLocationID: p.LocationID, Status: "missing",
		})
	}
	ev.Counts = muster.Counts{Expected: len(ev.Entries), Missing: len(ev.Entries)}
	f.active = ev
	f.events[ev.ID] = ev
	return ev, nil
}
func (f *fakeStore) GetActiveMusterEvent(_ context.Context, _ int) (*muster.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.active == nil {
		return nil, nil
	}
	cp := *f.active
	cp.Counts = countEntries(f.active.Entries)
	return &cp, nil
}
func (f *fakeStore) GetMusterEvent(_ context.Context, _ int, id int) (*muster.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ev := f.events[id]
	if ev == nil {
		return nil, nil
	}
	cp := *ev
	cp.Counts = countEntries(ev.Entries)
	return &cp, nil
}
func (f *fakeStore) ListMusterEvents(_ context.Context, _ int) ([]muster.Event, error) {
	return nil, nil
}
func (f *fakeStore) MarkEntryAtMuster(_ context.Context, _ int, eventID, assetID, locationID int, seenAt time.Time) (*muster.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markCalls = append(f.markCalls, markCall{eventID, assetID, locationID})
	ev := f.events[eventID]
	if ev == nil {
		return nil, nil
	}
	for i := range ev.Entries {
		if ev.Entries[i].AssetID == assetID {
			if ev.Entries[i].Status != "missing" {
				return nil, nil // sticky no-op
			}
			ev.Entries[i].Status = "at_muster"
			ev.Entries[i].MusterLocationID = &locationID
			ev.Entries[i].FirstMusterSeenAt = &seenAt
			cp := ev.Entries[i]
			return &cp, nil
		}
	}
	return nil, nil
}
func (f *fakeStore) UpdateEntryStatus(_ context.Context, _ int, eventID, entryID int, action string, userID int, note string) (*muster.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ev := f.events[eventID]
	if ev == nil {
		return nil, nil
	}
	for i := range ev.Entries {
		if ev.Entries[i].ID == entryID {
			cur := ev.Entries[i].Status
			now := time.Now()
			switch action {
			case "verify":
				if cur != "at_muster" {
					return nil, muster.ErrInvalidTransition{Current: cur, Action: action}
				}
				ev.Entries[i].Status = "verified"
				ev.Entries[i].VerifiedBy = &userID
				ev.Entries[i].VerifiedAt = &now
			case "mark_safe":
				if cur != "missing" && cur != "at_muster" {
					return nil, muster.ErrInvalidTransition{Current: cur, Action: action}
				}
				ev.Entries[i].Status = "safe_manual"
				ev.Entries[i].MarkedSafeBy = &userID
				ev.Entries[i].MarkedSafeAt = &now
				ev.Entries[i].MarkedSafeNote = note
			}
			cp := ev.Entries[i]
			f.updated = append(f.updated, cp)
			return &cp, nil
		}
	}
	return nil, nil
}
func (f *fakeStore) CompleteMusterEvent(_ context.Context, _ int, eventID int, status string, report json.RawMessage) (*muster.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completed = append(f.completed, completeCall{eventID, status, report})
	ev := f.events[eventID]
	if ev == nil {
		return nil, nil
	}
	ev.Status = status
	ev.Report = report
	if f.active != nil && f.active.ID == eventID {
		f.active = nil
	}
	cp := *ev
	return &cp, nil
}
func (f *fakeStore) AppendMusterUnlock(_ context.Context, _ int, _ int, unlock map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unlocks = append(f.unlocks, unlock)
	return nil
}

func countEntries(entries []muster.Entry) muster.Counts {
	c := muster.Counts{Expected: len(entries)}
	for _, e := range entries {
		switch e.Status {
		case "missing":
			c.Missing++
		case "at_muster":
			c.AtMuster++
		case "verified":
			c.Verified++
		case "safe_manual":
			c.SafeManual++
		}
	}
	return c
}

// ── fake broadcaster ───────────────────────────────────────────────────────────

type fakeBC struct {
	mu        sync.Mutex
	snapshots []SnapshotPayload
	presence  []PresencePayload
	entries   []entryPayload
	events    []muster.Event
}

func (b *fakeBC) BroadcastSnapshot(_ int, p SnapshotPayload) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snapshots = append(b.snapshots, p)
}
func (b *fakeBC) BroadcastPresence(_ int, p PresencePayload) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.presence = append(b.presence, p)
}
func (b *fakeBC) BroadcastEntry(_ int, entry muster.Entry, counts muster.Counts) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, entryPayload{Entry: entry, Counts: counts})
}
func (b *fakeBC) BroadcastEvent(_ int, ev muster.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}

func newTestEngine(store engineStore, bc broadcaster) *Engine {
	log := zerolog.Nop()
	return newEngine(store, bc, &log)
}

func resolved(assetID, locationID int) storage.ResolvedRead {
	return storage.ResolvedRead{AssetID: assetID, ScanPointID: 1, LocationID: ptr(locationID), EPC: "EPC", RSSI: -50}
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestActivate_SnapshotsExpectedSet(t *testing.T) {
	store := newFakeStore()
	store.persons = []muster.PersonPresence{
		{AssetID: 1, Label: "Operator 001", LocationID: ptr(10)},
		{AssetID: 2, Label: "Operator 002", LocationID: ptr(11)},
	}
	bc := &fakeBC{}
	e := newTestEngine(store, bc)

	ev, err := e.Activate(context.Background(), 1, 99, 15)
	require.NoError(t, err)
	require.Equal(t, "active", ev.Status)
	require.Len(t, ev.Entries, 2)
	require.Equal(t, 2, ev.Counts.Missing)
	require.Len(t, bc.events, 1) // activation broadcast
}

func TestActivate_ConflictPropagates(t *testing.T) {
	store := newFakeStore()
	store.createErr = muster.ErrActiveEventExists{}
	e := newTestEngine(store, &fakeBC{})
	_, err := e.Activate(context.Background(), 1, 99, 15)
	require.ErrorAs(t, err, &muster.ErrActiveEventExists{})
}

func TestEvaluate_MusterPointReadTransitionsExpected(t *testing.T) {
	store := newFakeStore()
	store.persons = []muster.PersonPresence{{AssetID: 1, Label: "Op1", LocationID: ptr(10)}}
	store.musterPoints = []int{20}
	bc := &fakeBC{}
	e := newTestEngine(store, bc)

	_, err := e.Activate(context.Background(), 1, 99, 15)
	require.NoError(t, err)

	// Read person 1 at muster point 20.
	e.Evaluate(context.Background(), 1, 1, time.Now(), []storage.ResolvedRead{resolved(1, 20)})

	require.Len(t, store.markCalls, 1)
	require.Equal(t, 20, store.markCalls[0].locationID)
	require.Len(t, bc.entries, 1)
	require.Equal(t, "at_muster", bc.entries[0].Entry.Status)
}

func TestEvaluate_NonMusterReadOnlyUpdatesPresence(t *testing.T) {
	store := newFakeStore()
	store.persons = []muster.PersonPresence{{AssetID: 1, Label: "Op1", LocationID: ptr(10)}}
	store.zones = []muster.ZonePresence{{LocationID: 10, Name: "Floor"}, {LocationID: 20, Name: "Muster", MusterPoint: true}}
	store.musterPoints = []int{20}
	bc := &fakeBC{}
	e := newTestEngine(store, bc)
	_, _ = e.Activate(context.Background(), 1, 99, 15)

	// Read at zone 10 (not a muster point).
	e.Evaluate(context.Background(), 1, 1, time.Now(), []storage.ResolvedRead{resolved(1, 10)})

	require.Empty(t, store.markCalls)
	require.NotEmpty(t, bc.presence) // presence updated + broadcast
}

func TestEvaluate_StickyVerifiedNotDowngraded(t *testing.T) {
	store := newFakeStore()
	store.persons = []muster.PersonPresence{{AssetID: 1, Label: "Op1", LocationID: ptr(10)}}
	store.musterPoints = []int{20}
	e := newTestEngine(store, &fakeBC{})
	ev, _ := e.Activate(context.Background(), 1, 99, 15)

	// Transition to at_muster, then verify.
	e.Evaluate(context.Background(), 1, 1, time.Now(), []storage.ResolvedRead{resolved(1, 20)})
	entryID := ev.Entries[0].ID
	_, _, err := e.Verify(context.Background(), 1, ev.ID, entryID, 99)
	require.NoError(t, err)

	// Another muster-point read must be a no-op (sticky).
	before := len(store.markCalls)
	e.Evaluate(context.Background(), 1, 1, time.Now(), []storage.ResolvedRead{resolved(1, 20)})
	// MarkEntryAtMuster is still called but returns nil (no-op); status stays verified.
	got, _ := store.GetMusterEvent(context.Background(), 1, ev.ID)
	require.Equal(t, "verified", got.Entries[0].Status)
	require.GreaterOrEqual(t, len(store.markCalls), before)
}

func TestEvaluate_UnknownNonPersonIgnored(t *testing.T) {
	store := newFakeStore()
	store.persons = []muster.PersonPresence{{AssetID: 1, Label: "Op1", LocationID: ptr(10)}}
	store.musterPoints = []int{20}
	e := newTestEngine(store, &fakeBC{})
	_, _ = e.Activate(context.Background(), 1, 99, 15)

	// Asset 999 is not a person — must be ignored.
	e.Evaluate(context.Background(), 1, 1, time.Now(), []storage.ResolvedRead{resolved(999, 20)})
	require.Empty(t, store.markCalls)
}

func TestVerify_InvalidTransition409(t *testing.T) {
	store := newFakeStore()
	store.persons = []muster.PersonPresence{{AssetID: 1, Label: "Op1", LocationID: ptr(10)}}
	e := newTestEngine(store, &fakeBC{})
	ev, _ := e.Activate(context.Background(), 1, 99, 15)

	// Verify from 'missing' is invalid.
	_, _, err := e.Verify(context.Background(), 1, ev.ID, ev.Entries[0].ID, 99)
	require.ErrorAs(t, err, &muster.ErrInvalidTransition{})
}

func TestMarkSafe_FromMissing(t *testing.T) {
	store := newFakeStore()
	store.persons = []muster.PersonPresence{{AssetID: 1, Label: "Op1", LocationID: ptr(10)}}
	bc := &fakeBC{}
	e := newTestEngine(store, bc)
	ev, _ := e.Activate(context.Background(), 1, 99, 15)

	entry, _, err := e.MarkSafe(context.Background(), 1, ev.ID, ev.Entries[0].ID, 99, "called in")
	require.NoError(t, err)
	require.Equal(t, "safe_manual", entry.Status)
	require.Equal(t, "called in", entry.MarkedSafeNote)
}

func TestAllClear_ComputesReport(t *testing.T) {
	store := newFakeStore()
	store.persons = []muster.PersonPresence{
		{AssetID: 1, Label: "Op1", LocationID: ptr(10)},
		{AssetID: 2, Label: "Op2", LocationID: ptr(10)},
	}
	store.zones = []muster.ZonePresence{
		{LocationID: 10, Name: "Floor"},
		{LocationID: 20, Name: "Muster A", MusterPoint: true},
	}
	store.musterPoints = []int{20}
	bc := &fakeBC{}
	e := newTestEngine(store, bc)
	ev, _ := e.Activate(context.Background(), 1, 99, 15)

	// Both persons reach the muster point.
	e.Evaluate(context.Background(), 1, 1, time.Now(), []storage.ResolvedRead{resolved(1, 20), resolved(2, 20)})

	done, err := e.AllClear(context.Background(), 1, ev.ID, 99)
	require.NoError(t, err)
	require.Equal(t, "completed", done.Status)
	require.NotNil(t, done.Report)

	var rep struct {
		TotalSeconds int `json:"total_seconds"`
		Counts       muster.Counts
		Zones        []struct {
			LocationID int        `json:"location_id"`
			Expected   int        `json:"expected"`
			Accounted  int        `json:"accounted"`
			ClearedAt  *time.Time `json:"cleared_at"`
		} `json:"zones"`
		MusterPoints []struct {
			LocationID int `json:"location_id"`
			Arrivals   int `json:"arrivals"`
		} `json:"muster_points"`
	}
	require.NoError(t, json.Unmarshal(done.Report, &rep))
	require.Len(t, rep.Zones, 1)
	require.Equal(t, 10, rep.Zones[0].LocationID)
	require.Equal(t, 2, rep.Zones[0].Expected)
	require.Equal(t, 2, rep.Zones[0].Accounted)
	require.NotNil(t, rep.Zones[0].ClearedAt) // fully accounted → cleared_at set
	require.Len(t, rep.MusterPoints, 1)
	require.Equal(t, 2, rep.MusterPoints[0].Arrivals)
}

func TestAllClear_PartialZoneNotCleared(t *testing.T) {
	store := newFakeStore()
	store.persons = []muster.PersonPresence{
		{AssetID: 1, Label: "Op1", LocationID: ptr(10)},
		{AssetID: 2, Label: "Op2", LocationID: ptr(10)},
	}
	store.zones = []muster.ZonePresence{{LocationID: 10, Name: "Floor"}, {LocationID: 20, Name: "Muster", MusterPoint: true}}
	store.musterPoints = []int{20}
	e := newTestEngine(store, &fakeBC{})
	ev, _ := e.Activate(context.Background(), 1, 99, 15)

	// Only person 1 reaches muster; person 2 stays missing.
	e.Evaluate(context.Background(), 1, 1, time.Now(), []storage.ResolvedRead{resolved(1, 20)})

	done, err := e.AllClear(context.Background(), 1, ev.ID, 99)
	require.NoError(t, err)
	var rep struct {
		Zones []struct {
			Accounted int        `json:"accounted"`
			Expected  int        `json:"expected"`
			ClearedAt *time.Time `json:"cleared_at"`
		} `json:"zones"`
	}
	require.NoError(t, json.Unmarshal(done.Report, &rep))
	require.Len(t, rep.Zones, 1)
	require.Equal(t, 1, rep.Zones[0].Accounted)
	require.Equal(t, 2, rep.Zones[0].Expected)
	require.Nil(t, rep.Zones[0].ClearedAt) // not fully accounted → nil
}

func TestStatus_PresenceAndActiveEvent(t *testing.T) {
	store := newFakeStore()
	store.persons = []muster.PersonPresence{{AssetID: 1, Label: "Op1", LocationID: ptr(10), LastSeenAt: time.Now()}}
	store.zones = []muster.ZonePresence{{LocationID: 10, Name: "Floor"}}
	e := newTestEngine(store, &fakeBC{})

	snap, err := e.Status(context.Background(), 1)
	require.NoError(t, err)
	require.Nil(t, snap.Event)
	require.Len(t, snap.Zones, 1)
	require.Equal(t, 1, snap.Zones[0].Count) // hydrated person counted
	require.Equal(t, 1, snap.PersonsOnSite)

	_, _ = e.Activate(context.Background(), 1, 99, 15)
	snap, err = e.Status(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, snap.Event)
}

func TestUnlock_RecordsReveal(t *testing.T) {
	store := newFakeStore()
	e := newTestEngine(store, &fakeBC{})
	err := e.Unlock(context.Background(), 1, 100, 99, "op@example.com")
	require.NoError(t, err)
	require.Len(t, store.unlocks, 1)
	require.Equal(t, "op@example.com", store.unlocks[0]["email"])
}
