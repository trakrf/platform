package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/trakrf/platform/backend/internal/detection"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanread"
	"github.com/trakrf/platform/backend/internal/services/topicroute"
	"github.com/trakrf/platform/backend/internal/storage"
)

const oneTagPayload = `{"tags":[{"epc":"E280","antennaPort":1,"rssi":-50,"timeStampOfRead":1}]}`

type fakeMessage struct {
	mqtt.Message
}

func (fakeMessage) Topic() string   { return "reader/reads" }
func (fakeMessage) Payload() []byte { return []byte(oneTagPayload) }

// fakeTx accepts the pipeline's savepoint statements.
type fakeTx struct{ pgx.Tx }

func (fakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

// fakeScanStore stands in for the database. persistErr fails the scan after the
// hook ran, as a failed commit does.
type fakeScanStore struct {
	persistErr error
	plain      int
	withHook   int
}

func (*fakeScanStore) InsertRawTagScan(context.Context, string, []byte) (int64, error) {
	return 1, nil
}

func (*fakeScanStore) ResolveScanTopic(context.Context, string) (storage.ScanRoute, bool, error) {
	return storage.ScanRoute{OrgID: 1, ScanDeviceID: 2, DeviceType: scandevice.DeviceTypeCS463}, true, nil
}

func (f *fakeScanStore) PersistReads(context.Context, int, int, int64, time.Time, []scanread.Read) (storage.PersistResult, error) {
	f.plain++
	return storage.PersistResult{Resolved: []storage.ResolvedRead{{AssetID: 1}}}, nil
}

func (f *fakeScanStore) PersistReadsDetect(ctx context.Context, orgID, scanDeviceID int, tagScanID int64, receivedAt time.Time, _ []scanread.Read, detect storage.DetectFunc) (storage.PersistResult, error) {
	f.withHook++
	resolved := []storage.ResolvedRead{{AssetID: 1}}
	err := detect(ctx, fakeTx{}, storage.DetectInput{
		OrgID: orgID, ScanDeviceID: scanDeviceID, TagScanID: tagScanID, ReceivedAt: receivedAt, Reads: resolved,
	})
	if err == nil {
		err = f.persistErr
	}
	if err != nil {
		return storage.PersistResult{}, err
	}
	return storage.PersistResult{Resolved: resolved}, nil
}

// eventLog records the order the scan's outcome reaches each collaborator.
type eventLog struct{ events []string }

func (l *eventLog) add(e string) { l.events = append(l.events, e) }

func (l *eventLog) Name() string { return "recording" }

func (l *eventLog) Detect(context.Context, pgx.Tx, storage.DetectInput) (detection.Pending, error) {
	l.add("detect")
	return detection.Pending{
		OnCommit:   func(bool) { l.add("committed") },
		OnRollback: func() { l.add("rolled_back") },
	}, nil
}

func (l *eventLog) Evaluate(context.Context, int, int64, time.Time, []storage.ResolvedRead) {
	l.add("evaluate")
}

func newDetectSubscriber(store scanStore, eval ReadEvaluator) *Subscriber {
	log := zerolog.Nop()
	return &Subscriber{store: store, registry: topicroute.NewRegistry(nil, log), eval: eval, log: log}
}

func TestHandleMessage_WithDetector_CommitsThenEvaluates(t *testing.T) {
	store, events := &fakeScanStore{}, &eventLog{}
	log := zerolog.Nop()
	s := newDetectSubscriber(store, events)
	s.SetDetector(detection.NewPipeline(&log, events))

	s.handleMessage(nil, fakeMessage{})

	assert.Equal(t, []string{"detect", "committed", "evaluate"}, events.events)
	assert.Equal(t, 1, store.withHook)
	assert.Zero(t, store.plain)
}

func TestHandleMessage_PersistFailure_RollsBackRun(t *testing.T) {
	store, events := &fakeScanStore{persistErr: errors.New("commit failed")}, &eventLog{}
	log := zerolog.Nop()
	s := newDetectSubscriber(store, events)
	s.SetDetector(detection.NewPipeline(&log, events))

	s.handleMessage(nil, fakeMessage{})

	assert.Equal(t, []string{"detect", "rolled_back"}, events.events)
}

func TestHandleMessage_NoDetector_UsesPersistReads(t *testing.T) {
	store, events := &fakeScanStore{}, &eventLog{}
	s := newDetectSubscriber(store, events)

	s.handleMessage(nil, fakeMessage{})

	assert.Equal(t, []string{"evaluate"}, events.events)
	assert.Equal(t, 1, store.plain)
	assert.Zero(t, store.withHook)
}
