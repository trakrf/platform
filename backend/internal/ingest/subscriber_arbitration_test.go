package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/trakrf/platform/backend/internal/arbitration"
	"github.com/trakrf/platform/backend/internal/detection"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanread"
	"github.com/trakrf/platform/backend/internal/storage"
)

type glMessage struct{ mqtt.Message }

func (glMessage) Topic() string { return "gw/reads" }
func (glMessage) Payload() []byte {
	return []byte(`{"dev_ble_mac":"C4DEE2000000","dev_list":[{"mac":"AA0000000001","rssi":-50,"ts":1}]}`)
}

func (f *fakeScanStore) PersistReadsArbitrated(ctx context.Context, orgID, scanDeviceID int, tagScanID int64, receivedAt time.Time, _ []scanread.Read, arbitrate storage.ArbitrateFunc, detect storage.DetectFunc) (storage.PersistResult, error) {
	f.arbitrated++
	loc := 10
	resolved := []storage.ResolvedRead{{AssetID: 1, ScanPointID: 3, LocationID: &loc, RSSI: -50}}
	if _, err := arbitrate(ctx, fakeTx{}, storage.ArbitrateInput{OrgID: orgID, ReceivedAt: receivedAt, Reads: resolved}); err != nil {
		return storage.PersistResult{}, err
	}
	if detect != nil {
		if err := detect(ctx, fakeTx{}, storage.DetectInput{OrgID: orgID, ScanDeviceID: scanDeviceID, TagScanID: tagScanID, ReceivedAt: receivedAt, Reads: resolved}); err != nil {
			return storage.PersistResult{}, err
		}
	}
	if f.persistErr != nil {
		return storage.PersistResult{}, f.persistErr
	}
	return storage.PersistResult{Resolved: resolved}, nil
}

// countingSeeds counts seed lookups: a committed arbitration needs none next time.
type countingSeeds struct{ calls int }

func (c *countingSeeds) PreviousAssetLocationsTx(context.Context, pgx.Tx, int, []int, time.Time) (map[int]storage.PreviousLocation, error) {
	c.calls++
	return nil, nil
}

func newArbitratedSubscriber(store *fakeScanStore, events *eventLog) (*Subscriber, *countingSeeds) {
	s := newDetectSubscriber(store, events)
	seeds := &countingSeeds{}
	s.SetArbiter(arbitration.NewStage(arbitration.New(arbitration.DefaultConfig()), seeds), arbitration.DefaultConfig())
	return s, seeds
}

func TestHandleMessage_BLEGatewayIsArbitrated(t *testing.T) {
	store, events := &fakeScanStore{deviceType: scandevice.DeviceTypeGLS10}, &eventLog{}
	log := zerolog.Nop()
	s, seeds := newArbitratedSubscriber(store, events)
	s.SetDetector(detection.NewPipeline(&log, events))

	s.handleMessage(nil, glMessage{})
	s.handleMessage(nil, glMessage{})

	assert.Equal(t, 2, store.arbitrated)
	assert.Zero(t, store.withHook+store.plain)
	assert.Equal(t, []string{"detect", "committed", "evaluate", "detect", "committed", "evaluate"}, events.events)
	assert.Equal(t, 1, seeds.calls, "the first scan's arbitration was committed")
}

func TestHandleMessage_CS463IsNeverArbitrated(t *testing.T) {
	store, events := &fakeScanStore{}, &eventLog{}
	log := zerolog.Nop()
	s, seeds := newArbitratedSubscriber(store, events)
	s.SetDetector(detection.NewPipeline(&log, events))

	s.handleMessage(nil, fakeMessage{})

	assert.Zero(t, store.arbitrated)
	assert.Equal(t, 1, store.withHook)
	assert.Zero(t, seeds.calls)
}

func TestHandleMessage_ArbitratedRollbackDoesNotCommitArbiter(t *testing.T) {
	store, events := &fakeScanStore{deviceType: scandevice.DeviceTypeGLS10, persistErr: errors.New("commit failed")}, &eventLog{}
	log := zerolog.Nop()
	s, seeds := newArbitratedSubscriber(store, events)
	s.SetDetector(detection.NewPipeline(&log, events))

	s.handleMessage(nil, glMessage{})
	assert.Equal(t, []string{"detect", "rolled_back"}, events.events)

	store.persistErr = nil
	s.handleMessage(nil, glMessage{})
	assert.Equal(t, 2, seeds.calls, "the rolled-back decision was dropped, so the asset seeds again")
}

func TestHandleMessage_ArbitratedWithoutDetector(t *testing.T) {
	store, events := &fakeScanStore{deviceType: scandevice.DeviceTypeGLS10}, &eventLog{}
	s, _ := newArbitratedSubscriber(store, events)

	s.handleMessage(nil, glMessage{})

	assert.Equal(t, 1, store.arbitrated)
	assert.Equal(t, []string{"evaluate"}, events.events)
}
