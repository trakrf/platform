package outbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
)

// fakeBatchStore is an in-memory double for the batch store surface. Its
// WithOrgTx hands fn a nil tx (nothing here touches it) and records whether
// the transaction would have committed or rolled back.
type fakeBatchStore struct {
	existing map[string]bool // delivery IDs already in the outbox
	nextID   int64

	txOpened   int
	committed  bool
	rolledBack bool

	inserted []notificationdelivery.NotificationDelivery
	links    map[int64]int64 // notification delivery id -> river job id
}

func newFakeBatchStore(existing ...string) *fakeBatchStore {
	s := &fakeBatchStore{existing: map[string]bool{}, nextID: 100, links: map[int64]int64{}}
	for _, id := range existing {
		s.existing[id] = true
	}
	return s
}

func (s *fakeBatchStore) WithOrgTx(ctx context.Context, orgID int, fn func(tx pgx.Tx) error) error {
	s.txOpened++
	if err := fn(nil); err != nil {
		s.rolledBack = true
		return err
	}
	s.committed = true
	return nil
}

func (s *fakeBatchStore) InsertNotificationDeliveryIfAbsentTx(ctx context.Context, tx pgx.Tx, orgID int, d notificationdelivery.NotificationDelivery) (int64, bool, error) {
	if s.existing[d.DeliveryID] {
		return 0, false, nil
	}
	s.existing[d.DeliveryID] = true
	s.nextID++
	s.inserted = append(s.inserted, d)
	return s.nextID, true, nil
}

func (s *fakeBatchStore) SetNotificationDeliveryRiverJobIDTx(ctx context.Context, tx pgx.Tx, orgID int, id, riverJobID int64) error {
	s.links[id] = riverJobID
	return nil
}

// fakeInserter stands in for *river.Client; failOn makes the Nth call
// (1-based) fail.
type fakeInserter struct {
	failOn int
	calls  int
	args   []outbox.DeliveryJobArgs
	opts   []*river.InsertOpts
}

func (f *fakeInserter) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	f.calls++
	if f.calls == f.failOn {
		return nil, errors.New("river insert failed")
	}
	f.args = append(f.args, args.(outbox.DeliveryJobArgs))
	f.opts = append(f.opts, opts)
	return &rivertype.JobInsertResult{Job: &rivertype.JobRow{ID: int64(9000 + f.calls)}}, nil
}

func newTestMetrics(t *testing.T) (*outbox.Metrics, *prometheus.Registry) {
	t.Helper()
	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)
	return metrics, registry
}

func batchCmd(deliveryID string) outbox.Command {
	return outbox.Command{
		DeliveryID: deliveryID,
		OrgID:      42,
		Channel:    notificationdelivery.ChannelEmail,
		Payload:    []byte(`{"to":"someone@example.test","body":"secret message"}`),
	}
}

func TestEnqueueBatch_EmptyBatchOpensNoTransaction(t *testing.T) {
	store := newFakeBatchStore()
	jobs := &fakeInserter{}
	metrics, _ := newTestMetrics(t)

	results, err := outbox.NewBatchEnqueuer(store, jobs, metrics).EnqueueBatch(context.Background(), 42, nil)

	require.NoError(t, err)
	require.Nil(t, results)
	require.Zero(t, store.txOpened)
	require.Zero(t, jobs.calls)
}

func TestEnqueueBatch_OrgMismatchRejectedBeforeTransaction(t *testing.T) {
	store := newFakeBatchStore()
	jobs := &fakeInserter{}
	metrics, registry := newTestMetrics(t)
	other := batchCmd("evt-2:contact-1:email")
	other.OrgID = 7

	results, err := outbox.NewBatchEnqueuer(store, jobs, metrics).EnqueueBatch(context.Background(), 42,
		[]outbox.Command{batchCmd("evt-1:contact-1:email"), other})

	require.Error(t, err)
	require.Nil(t, results)
	require.Zero(t, store.txOpened, "a cross-org command must be refused before any transaction opens")
	require.Zero(t, jobs.calls)
	require.Zero(t, enqueueCount(t, registry, "ok"))
}

func TestEnqueueBatch_MixedInsertedAndDuplicateKeepsInputOrder(t *testing.T) {
	store := newFakeBatchStore("evt-1:contact-2:email")
	jobs := &fakeInserter{}
	metrics, registry := newTestMetrics(t)
	cmds := []outbox.Command{
		batchCmd("evt-1:contact-1:email"),
		batchCmd("evt-1:contact-2:email"), // already in the outbox
		batchCmd("evt-1:contact-3:email"),
		batchCmd("evt-1:contact-1:email"), // repeated within the batch
	}

	results, err := outbox.NewBatchEnqueuer(store, jobs, metrics).EnqueueBatch(context.Background(), 42, cmds)
	require.NoError(t, err)
	require.True(t, store.committed)

	require.Equal(t, []outbox.BatchResult{
		{DeliveryID: "evt-1:contact-1:email", NotificationDeliveryID: 101, RiverJobID: 9001},
		{DeliveryID: "evt-1:contact-2:email", Duplicate: true},
		{DeliveryID: "evt-1:contact-3:email", NotificationDeliveryID: 102, RiverJobID: 9002},
		{DeliveryID: "evt-1:contact-1:email", Duplicate: true},
	}, results)

	require.Equal(t, 2, jobs.calls, "duplicates must not get a River job")
	require.Equal(t, []outbox.DeliveryJobArgs{
		{OrgID: 42, NotificationDeliveryID: 101},
		{OrgID: 42, NotificationDeliveryID: 102},
	}, jobs.args)
	for _, opts := range jobs.opts {
		require.Equal(t, 6, opts.MaxAttempts)
	}
	require.Equal(t, map[int64]int64{101: 9001, 102: 9002}, store.links)

	require.Equal(t, float64(2), enqueueCount(t, registry, "ok"))
	require.Equal(t, float64(2), enqueueCount(t, registry, "duplicate"))
}

func TestEnqueueBatch_FailedBatchRecordsNoMetrics(t *testing.T) {
	store := newFakeBatchStore("evt-1:contact-2:email")
	jobs := &fakeInserter{failOn: 2}
	metrics, registry := newTestMetrics(t)
	cmds := []outbox.Command{
		batchCmd("evt-1:contact-1:email"),
		batchCmd("evt-1:contact-2:email"),
		batchCmd("evt-1:contact-3:email"),
	}

	results, err := outbox.NewBatchEnqueuer(store, jobs, metrics).EnqueueBatch(context.Background(), 42, cmds)

	require.Error(t, err)
	require.Nil(t, results)
	require.True(t, store.rolledBack)
	require.False(t, store.committed)
	require.Contains(t, err.Error(), "evt-1:contact-3:email", "the error names the failing delivery")
	require.NotContains(t, err.Error(), "secret message")
	require.NotContains(t, err.Error(), "someone@example.test")
	require.Zero(t, enqueueCount(t, registry, "ok"))
	require.Zero(t, enqueueCount(t, registry, "duplicate"))
}

func TestEnqueueBatchTx_MapsRoutingContextAndLeavesTxToCaller(t *testing.T) {
	store := newFakeBatchStore()
	jobs := &fakeInserter{}
	withContext := batchCmd("evt-9:contact-1:sms")
	withContext.Channel = notificationdelivery.ChannelSMS
	withContext.EventID = "evt-9"
	withContext.RecipientID = 5
	withContext.AssetID = 77
	bare := outbox.Command{DeliveryID: "evt-9:contact-2:email", OrgID: 42, Channel: notificationdelivery.ChannelEmail}

	results, err := outbox.EnqueueBatchTx(context.Background(), nil, store, jobs, []outbox.Command{withContext, bare})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Zero(t, store.txOpened, "EnqueueBatchTx runs in the caller's transaction")

	require.Len(t, store.inserted, 2)
	got := store.inserted[0]
	require.Equal(t, "evt-9:contact-1:sms", got.DeliveryID)
	require.Equal(t, notificationdelivery.ChannelSMS, got.Channel)
	require.Equal(t, notificationdelivery.StatePending, got.State)
	require.Equal(t, "evt-9", *got.EventID)
	require.Equal(t, 5, *got.RecipientID)
	require.Equal(t, 77, *got.AssetID)
	require.Equal(t, withContext.Payload, got.Payload)

	zero := store.inserted[1]
	require.Equal(t, notificationdelivery.StatePending, zero.State)
	require.Nil(t, zero.EventID, "a zero routing value is stored as NULL")
	require.Nil(t, zero.RecipientID)
	require.Nil(t, zero.AssetID)
	require.Empty(t, zero.Payload)
}
