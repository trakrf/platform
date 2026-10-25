package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/storage"
)

var occurredAt = time.Date(2026, 10, 20, 14, 5, 0, 0, time.UTC)

type fakeStore struct {
	entitled    bool
	entitledErr error

	occ      notificationevent.Occurrence
	occFound bool
	alarmN   int
	moveN    int

	assets    map[int]storage.AssetIdentity
	locations map[int]string
	subs      []notificationrecipient.Subscriber

	eventInserted bool
	events        []notificationevent.NotificationEvent
	deliveries    []notificationdelivery.NotificationDelivery
	withTxN       int
	storeCalls    int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		entitled: true, occFound: true, eventInserted: true,
		occ: notificationevent.Occurrence{
			Source: notificationevent.SourceAlarm, SourceID: 42, AssetID: 7, OccurredAt: occurredAt,
		},
		assets: map[int]storage.AssetIdentity{7: {ID: 7, ExternalKey: "ASSET-0007", Name: "Forklift 7"}},
	}
}

func (f *fakeStore) OrgIsEntitled(context.Context, int) (bool, error) {
	f.storeCalls++
	return f.entitled, f.entitledErr
}

func (f *fakeStore) GetAlarmOccurrence(context.Context, int, int) (notificationevent.Occurrence, bool, error) {
	f.storeCalls++
	f.alarmN++
	return f.occ, f.occFound, nil
}

func (f *fakeStore) GetLocationChangeOccurrence(context.Context, int, int) (notificationevent.Occurrence, bool, error) {
	f.storeCalls++
	f.moveN++
	return f.occ, f.occFound, nil
}

func (f *fakeStore) LookupMoveNames(context.Context, int, []int, []int) (map[int]storage.AssetIdentity, map[int]string, error) {
	f.storeCalls++
	return f.assets, f.locations, nil
}

func (f *fakeStore) ListSubscribersForAsset(context.Context, int, int) ([]notificationrecipient.Subscriber, error) {
	f.storeCalls++
	return f.subs, nil
}

func (f *fakeStore) IsSuppressed(context.Context, int, notificationdelivery.Channel, string) (bool, error) {
	f.storeCalls++
	return false, nil
}

func (f *fakeStore) WithOrgTx(_ context.Context, _ int, fn func(pgx.Tx) error) error {
	f.storeCalls++
	f.withTxN++
	return fn(nil)
}

func (f *fakeStore) InsertNotificationEventIfAbsentTx(_ context.Context, _ pgx.Tx, _ int, e notificationevent.NotificationEvent) (int, bool, error) {
	f.events = append(f.events, e)
	if !f.eventInserted {
		return 0, false, nil
	}
	return 500, true, nil
}

func (f *fakeStore) InsertNotificationDeliveryIfAbsentTx(_ context.Context, _ pgx.Tx, _ int, d notificationdelivery.NotificationDelivery) (int64, bool, error) {
	f.deliveries = append(f.deliveries, d)
	return int64(len(f.deliveries)), true, nil
}

func (f *fakeStore) SetNotificationDeliveryRiverJobIDTx(context.Context, pgx.Tx, int, int64, int64) error {
	return nil
}

type fakeJobs struct {
	calls  int
	failOn int
}

func (j *fakeJobs) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	j.calls++
	if j.calls == j.failOn {
		return nil, errors.New("river insert failed")
	}
	return &rivertype.JobInsertResult{Job: &rivertype.JobRow{ID: int64(9000 + j.calls)}}, nil
}

type fakeRenderer struct{ err error }

func (r fakeRenderer) Render(_ notificationevent.Source, ch notificationdelivery.Channel, _ TemplateData) (Rendered, error) {
	if r.err != nil {
		return Rendered{}, r.err
	}
	if ch == notificationdelivery.ChannelEmail {
		return Rendered{Subject: "SUBJECT-TEXT", Body: "BODY-TEXT"}, nil
	}
	return Rendered{Body: "BODY-TEXT"}, nil
}

type harness struct {
	store    *fakeStore
	jobs     *fakeJobs
	planner  *DeliveryPlanner
	registry *prometheus.Registry
	logs     *bytes.Buffer
}

func newHarness(t *testing.T, renderer Renderer) *harness {
	t.Helper()
	h := &harness{store: newFakeStore(), jobs: &fakeJobs{}, registry: prometheus.NewRegistry(), logs: &bytes.Buffer{}}
	metrics, err := NewMetrics(h.registry)
	require.NoError(t, err)
	outboxMetrics, err := outbox.NewMetrics(h.registry)
	require.NoError(t, err)
	if renderer == nil {
		renderer = fakeRenderer{}
	}
	h.planner, err = NewDeliveryPlanner(Deps{
		Store: h.store, Jobs: h.jobs, Renderer: renderer, Metrics: metrics, OutboxMetrics: outboxMetrics,
		PhoneRegion: "US", Log: zerolog.New(h.logs).Level(zerolog.DebugLevel),
	})
	require.NoError(t, err)
	return h
}

var alarmTrigger = Trigger{OrgID: 1, Source: notificationevent.SourceAlarm, SourceID: 42}

func emailSub(id int, addr string) notificationrecipient.Subscriber {
	return notificationrecipient.Subscriber{
		SubscriptionID: id, RecipientID: id, Channel: "email", Email: sp(addr), RecipientActive: true,
	}
}

func smsSub(id int, number string) notificationrecipient.Subscriber {
	return notificationrecipient.Subscriber{
		SubscriptionID: id, RecipientID: id, Channel: "sms", Phone: sp(number), RecipientActive: true,
	}
}

// counterValue sums a counter family's series matching the labels; 0 if none.
func counterValue(t *testing.T, reg *prometheus.Registry, family string, want map[string]string) float64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	var total float64
	for _, f := range families {
		if f.GetName() != family {
			continue
		}
		for _, m := range f.GetMetric() {
			got := map[string]string{}
			for _, l := range m.GetLabel() {
				got[l.GetName()] = l.GetValue()
			}
			match := true
			for k, v := range want {
				if got[k] != v {
					match = false
				}
			}
			if match {
				total += m.GetCounter().GetValue()
			}
		}
	}
	return total
}

func TestPlan_AlarmTriggerUsesOnlyAlarmSource(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.NoError(t, err)
	assert.Equal(t, 1, h.store.alarmN)
	assert.Equal(t, 0, h.store.moveN)
	require.Len(t, h.store.events, 1)
	e := h.store.events[0]
	assert.Equal(t, notificationevent.SourceAlarm, e.Source)
	require.NotNil(t, e.AlarmEventID)
	assert.Equal(t, 42, *e.AlarmEventID)
	assert.Nil(t, e.LocationChangeID)
	assert.Equal(t, 7, e.AssetID)
	assert.Equal(t, occurredAt, e.OccurredAt)
}

func TestPlan_LocationChangeTriggerUsesOnlyLocationChangeSource(t *testing.T) {
	h := newHarness(t, nil)
	h.store.occ.Source = notificationevent.SourceLocationChange
	_, err := h.planner.Plan(context.Background(), Trigger{OrgID: 1, Source: notificationevent.SourceLocationChange, SourceID: 9})
	require.NoError(t, err)
	assert.Equal(t, 0, h.store.alarmN)
	assert.Equal(t, 1, h.store.moveN)
	require.Len(t, h.store.events, 1)
	e := h.store.events[0]
	assert.Equal(t, notificationevent.SourceLocationChange, e.Source)
	require.NotNil(t, e.LocationChangeID)
	assert.Equal(t, 9, *e.LocationChangeID)
	assert.Nil(t, e.AlarmEventID)
}

func TestPlan_InvalidTriggerTouchesNothing(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.planner.Plan(context.Background(), Trigger{OrgID: 0, Source: notificationevent.SourceAlarm, SourceID: 1})
	require.ErrorIs(t, err, ErrInvalidTrigger)
	assert.Equal(t, 0, h.store.storeCalls)
}

func TestPlan_SourceNotFoundIsInvalid(t *testing.T) {
	h := newHarness(t, nil)
	h.store.occFound = false
	_, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.ErrorIs(t, err, ErrInvalidTrigger)
	assert.Equal(t, 0, h.store.withTxN)
}

func TestPlan_UnentitledWritesNothing(t *testing.T) {
	h := newHarness(t, nil)
	h.store.entitled = false
	res, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.NoError(t, err)
	assert.Equal(t, OutcomeSkippedUnentitled, res.Outcome)
	assert.Equal(t, 0, h.store.withTxN)
}

func TestPlan_EntitlementErrorRetries(t *testing.T) {
	h := newHarness(t, nil)
	h.store.entitledErr = errors.New("db down")
	_, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrInvalidTrigger))
}

func TestPlan_RenderErrorWritesNothing(t *testing.T) {
	h := newHarness(t, fakeRenderer{err: errors.New("bad template")})
	h.store.subs = []notificationrecipient.Subscriber{emailSub(1, "a@acme.test")}
	_, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.Error(t, err)
	assert.Equal(t, 0, h.store.withTxN)
}

func TestPlan_ExistingEventStops(t *testing.T) {
	h := newHarness(t, nil)
	h.store.eventInserted = false
	h.store.subs = []notificationrecipient.Subscriber{emailSub(1, "a@acme.test")}
	res, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.NoError(t, err)
	assert.Equal(t, OutcomeDuplicate, res.Outcome)
	assert.Empty(t, h.store.deliveries)
	assert.Equal(t, 0, h.jobs.calls)
}

func TestPlan_AssetDeletedRegistersEventOnly(t *testing.T) {
	h := newHarness(t, nil)
	h.store.assets = map[int]storage.AssetIdentity{}
	h.store.subs = []notificationrecipient.Subscriber{emailSub(1, "a@acme.test")}
	res, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.NoError(t, err)
	assert.Equal(t, OutcomeSkippedAssetDeleted, res.Outcome)
	assert.Len(t, h.store.events, 1)
	assert.Empty(t, h.store.deliveries)
}

func TestPlan_NoSubscribersRegistersEventOnly(t *testing.T) {
	h := newHarness(t, nil)
	res, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.NoError(t, err)
	assert.Equal(t, OutcomeSkippedNoSubscribers, res.Outcome)
	assert.Len(t, h.store.events, 1)
	assert.Empty(t, h.store.deliveries)
}

func TestPlan_EnqueuesEligibleWithPayload(t *testing.T) {
	h := newHarness(t, nil)
	inactive := emailSub(3, "gone@acme.test")
	inactive.RecipientActive = false
	h.store.subs = []notificationrecipient.Subscriber{emailSub(1, "Ops@Acme.test"), smsSub(2, "(555) 123-4567"), inactive}

	res, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.NoError(t, err)
	assert.Equal(t, Outcome(""), res.Outcome)
	assert.Equal(t, 500, res.EventID)

	outcomes := make([]Outcome, 0, len(res.Recipients))
	for _, r := range res.Recipients {
		outcomes = append(outcomes, r.Outcome)
	}
	assert.Equal(t, []Outcome{OutcomeEnqueued, OutcomeEnqueued, OutcomeSkippedInactive}, outcomes)

	require.Len(t, h.store.deliveries, 2)
	var msg DeliveryMessage
	require.NoError(t, json.Unmarshal(h.store.deliveries[1].Payload, &msg))
	assert.Equal(t, DeliveryMessage{
		EventID: 500, Source: notificationevent.SourceAlarm, Channel: notificationdelivery.ChannelSMS,
		RecipientID: 2, To: "+15551234567", Body: "BODY-TEXT", AssetID: 7, OccurredAt: occurredAt,
	}, msg)
	assert.Equal(t, deliveryID(500, 2, notificationdelivery.ChannelSMS), h.store.deliveries[1].DeliveryID)
	assert.Equal(t, res.Recipients[1].DeliveryID, h.store.deliveries[1].DeliveryID)

	assert.Equal(t, float64(2), counterValue(t, h.registry, "trakrf_notification_delivery_planning_outcomes_total",
		map[string]string{"source": "alarm", "outcome": "enqueued"}))
	assert.Equal(t, float64(2), counterValue(t, h.registry, "trakrf_outbox_enqueues_total", map[string]string{"result": "ok"}))
}

func TestPlan_NoMetricsWhenTxFails(t *testing.T) {
	h := newHarness(t, nil)
	h.jobs.failOn = 2
	h.store.subs = []notificationrecipient.Subscriber{emailSub(1, "a@acme.test"), emailSub(2, "b@acme.test")}
	_, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.Error(t, err)
	assert.Equal(t, float64(0), counterValue(t, h.registry, "trakrf_notification_delivery_planning_outcomes_total",
		map[string]string{"outcome": "enqueued"}))
	assert.Equal(t, float64(0), counterValue(t, h.registry, "trakrf_outbox_enqueues_total", map[string]string{"result": "ok"}))
	assert.Equal(t, float64(1), counterValue(t, h.registry, "trakrf_notification_delivery_planning_outcomes_total",
		map[string]string{"source": "alarm", "outcome": "failed"}))
}

func TestPlan_NoPIIInLogsOrErrors(t *testing.T) {
	h := newHarness(t, nil)
	h.jobs.failOn = 2
	h.store.subs = []notificationrecipient.Subscriber{emailSub(1, "pii-person@acme.test"), smsSub(2, "(555) 123-4567")}
	_, failErr := h.planner.Plan(context.Background(), alarmTrigger)
	require.Error(t, failErr)

	h.jobs.failOn = 0
	h.store.eventInserted = true
	_, err := h.planner.Plan(context.Background(), alarmTrigger)
	require.NoError(t, err)

	text := h.logs.String() + failErr.Error()
	for _, secret := range []string{"pii-person@acme.test", "(555) 123-4567", "+15551234567", "SUBJECT-TEXT", "BODY-TEXT"} {
		assert.NotContains(t, text, secret)
	}
	assert.Contains(t, h.logs.String(), `"source":"alarm"`)
}
