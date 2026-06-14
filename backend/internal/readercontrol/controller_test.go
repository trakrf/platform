package readercontrol

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/readerpower"
	"github.com/trakrf/platform/backend/internal/storage"
)

type fakeRoutes struct {
	routes map[string]storage.ScanRoute
}

func (f fakeRoutes) Lookup(topic string) (storage.ScanRoute, bool) {
	r, ok := f.routes[topic]
	return r, ok
}
func (f fakeRoutes) Topics() []string {
	out := make([]string, 0, len(f.routes))
	for t := range f.routes {
		out = append(out, t)
	}
	return out
}

type fakeStore struct {
	gotRoute storage.ScanRoute
	gotState readerpower.State
	calls    int
}

func (f *fakeStore) SetAntennaPowerState(_ context.Context, route storage.ScanRoute, st readerpower.State) error {
	f.gotRoute = route
	f.gotState = st
	f.calls++
	return nil
}

func TestPublishPowerCommand_TopicAndPayload(t *testing.T) {
	var gotTopic string
	var gotPayload []byte
	c := &Controller{
		log:     zerolog.Nop(),
		publish: func(topic string, payload []byte) error { gotTopic = topic; gotPayload = payload; return nil },
	}
	err := c.PublishPowerCommand(context.Background(), "demo/cs463-212", readerpower.Command{
		RequestID: "r1", Powers: map[string]float64{"1": 22.5}, Force: true,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotTopic != "demo/cs463-212/command/power" {
		t.Fatalf("topic = %q", gotTopic)
	}
	var cmd readerpower.Command
	if err := json.Unmarshal(gotPayload, &cmd); err != nil {
		t.Fatalf("payload not valid command json: %v", err)
	}
	if cmd.Powers["1"] != 22.5 || !cmd.Force {
		t.Fatalf("decoded command wrong: %+v", cmd)
	}
}

func TestProcessState_PersistsToResolvedRoute(t *testing.T) {
	store := &fakeStore{}
	routes := fakeRoutes{routes: map[string]storage.ScanRoute{
		"demo/cs463-212": {OrgID: 7, ScanDeviceID: 42, DeviceType: "csl_cs463"},
	}}
	c := &Controller{log: zerolog.Nop(), store: store, routes: routes}

	st := readerpower.State{RequestID: "r1", Status: readerpower.StatusOK, ActiveProfile: "TrakRF", Powers: map[string]float64{"1": 22.5}}
	payload, _ := json.Marshal(st)
	c.processState("demo/cs463-212/state/power", payload)

	if store.calls != 1 {
		t.Fatalf("store calls = %d, want 1", store.calls)
	}
	if store.gotRoute.OrgID != 7 || store.gotRoute.ScanDeviceID != 42 {
		t.Fatalf("route = %+v, want org7 dev42 (topic must strip /state/power and resolve)", store.gotRoute)
	}
	if store.gotState.ActiveProfile != "TrakRF" {
		t.Fatalf("state = %+v", store.gotState)
	}
}

func TestProcessState_UnknownReaderIgnored(t *testing.T) {
	store := &fakeStore{}
	c := &Controller{log: zerolog.Nop(), store: store, routes: fakeRoutes{routes: map[string]storage.ScanRoute{}}}
	c.processState("nope/state/power", []byte(`{"status":"ok"}`))
	if store.calls != 0 {
		t.Fatalf("unknown reader must not persist")
	}
}
