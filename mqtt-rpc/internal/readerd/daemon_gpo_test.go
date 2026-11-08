package readerd

import (
	"errors"
	"reflect"
	"testing"

	"github.com/rs/zerolog"
)

// Stop must drive the outputs off even when the broker is already gone: a
// shutdown that cannot reach the cloud is exactly when no OFF will arrive.
func TestStop_DrivesOutputsOffWithoutABroker(t *testing.T) {
	a := &fakeAdapter{}
	d := newTestDaemon(a)

	d.Stop()

	if !reflect.DeepEqual(a.outputsOff, []bool{true}) {
		t.Errorf("OutputsOff calls = %v, want one final sweep", a.outputsOff)
	}
}

// Start drives every output off before it connects, clearing a port left on
// by a crash, a kill -9 or the previous version. That sweep is not final:
// the daemon goes on to serve Gpo.Set.
func TestStart_DrivesOutputsOffFirst(t *testing.T) {
	a := &fakeAdapter{}
	log := zerolog.Nop()
	d := New(Config{Broker: BrokerConfig{
		URL: "tcp://127.0.0.1:1", BaseTopic: "trakrf.id/cs463-212", RPCClientID: "t",
	}}, a, &log)

	if err := d.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer d.Stop()

	if !reflect.DeepEqual(a.outputsOff, []bool{false}) {
		t.Errorf("OutputsOff calls after Start = %v, want one non-final sweep", a.outputsOff)
	}
}

// A port that cannot be driven off is logged by the adapter, but it never
// stops the daemon starting or stopping.
func TestStartStop_SurviveASafeStateFailure(t *testing.T) {
	a := &fakeAdapter{outputsOffErr: errors.New("port 2 not driven off")}
	log := zerolog.Nop()
	d := New(Config{Broker: BrokerConfig{URL: "tcp://127.0.0.1:1", BaseTopic: "b", RPCClientID: "t"}}, a, &log)

	if err := d.Start(); err != nil {
		t.Fatalf("Start = %v, want nil despite the safe-state failure", err)
	}
	d.Stop()
	if len(a.outputsOff) != 2 {
		t.Errorf("OutputsOff calls = %v, want start and stop", a.outputsOff)
	}
}
