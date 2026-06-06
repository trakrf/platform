package alarm

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/trakrf/platform/backend/internal/geofence"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

type fakeLookup struct {
	devices   []outputdevice.OutputDevice
	err       error
	gotOrg    int
	gotLoc    int
	callCount int
}

func (f *fakeLookup) ListOutputDevicesForLocation(_ context.Context, orgID, locationID int) ([]outputdevice.OutputDevice, error) {
	f.gotOrg, f.gotLoc = orgID, locationID
	f.callCount++
	return f.devices, f.err
}

type setCall struct {
	deviceID    int
	on          bool
	offAfterSec int
}

type fakeDriver struct {
	calls    []setCall
	failOnID int // returns an error when device.ID == failOnID
}

func (d *fakeDriver) Set(_ context.Context, dev outputdevice.OutputDevice, on bool, offAfterSec int) error {
	d.calls = append(d.calls, setCall{dev.ID, on, offAfterSec})
	if dev.ID == d.failOnID {
		return errors.New("boom")
	}
	return nil
}

func testEvent() geofence.AlarmEvent {
	loc := 42
	return geofence.AlarmEvent{OrgID: 7, AssetID: 3, ScanPointID: 9, LocationID: &loc, EPC: "E2", RSSI: -50, FiredAt: time.Unix(0, 0)}
}

func newTestFirer(lookup deviceLookup, drv deviceSetter) Firer {
	log := zerolog.New(io.Discard)
	return NewFirer(lookup, drv, &log)
}

func TestFirer_FiresEachBoundDevice(t *testing.T) {
	lookup := &fakeLookup{devices: []outputdevice.OutputDevice{
		{ID: 1, BaseURL: "http://a", SwitchID: 0},
		{ID: 2, BaseURL: "http://b", SwitchID: 1},
	}}
	drv := &fakeDriver{}
	f := newTestFirer(lookup, drv)

	if err := f.Fire(context.Background(), testEvent()); err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	if lookup.gotOrg != 7 || lookup.gotLoc != 42 {
		t.Errorf("lookup got org=%d location=%d, want 7/42", lookup.gotOrg, lookup.gotLoc)
	}
	if len(drv.calls) != 2 {
		t.Fatalf("driver calls = %d, want 2", len(drv.calls))
	}
	want := []setCall{{1, true, 0}, {2, true, 0}}
	for i, c := range drv.calls {
		if c != want[i] {
			t.Errorf("call[%d] = %+v, want %+v", i, c, want[i])
		}
	}
}

func TestFirer_PassesAutoOffSecondsFromMetadata(t *testing.T) {
	lookup := &fakeLookup{devices: []outputdevice.OutputDevice{
		{ID: 1, Metadata: map[string]any{"auto_off_seconds": float64(30)}},
		{ID: 2, Metadata: map[string]any{}}, // no auto-off -> 0
	}}
	drv := &fakeDriver{}
	f := newTestFirer(lookup, drv)

	if err := f.Fire(context.Background(), testEvent()); err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	want := []setCall{{1, true, 30}, {2, true, 0}}
	for i, c := range drv.calls {
		if c != want[i] {
			t.Errorf("call[%d] = %+v, want %+v", i, c, want[i])
		}
	}
}

func TestFirer_NoDevicesNoCalls(t *testing.T) {
	lookup := &fakeLookup{devices: nil}
	drv := &fakeDriver{}
	f := newTestFirer(lookup, drv)

	if err := f.Fire(context.Background(), testEvent()); err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	if len(drv.calls) != 0 {
		t.Errorf("driver calls = %d, want 0", len(drv.calls))
	}
}

func TestFirer_NilLocationIsNoOp(t *testing.T) {
	lookup := &fakeLookup{devices: []outputdevice.OutputDevice{{ID: 1, BaseURL: "http://a"}}}
	drv := &fakeDriver{}
	f := newTestFirer(lookup, drv)

	ev := testEvent()
	ev.LocationID = nil // tripped scan point not mapped to a location

	if err := f.Fire(context.Background(), ev); err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	if lookup.callCount != 0 {
		t.Errorf("lookup called %d times, want 0 when location is nil", lookup.callCount)
	}
	if len(drv.calls) != 0 {
		t.Errorf("driver calls = %d, want 0 when location is nil", len(drv.calls))
	}
}

func TestFirer_DriverErrorAggregatedNotFatal(t *testing.T) {
	lookup := &fakeLookup{devices: []outputdevice.OutputDevice{
		{ID: 1, BaseURL: "http://ok", SwitchID: 0},
		{ID: 2, BaseURL: "http://bad", SwitchID: 0},
	}}
	drv := &fakeDriver{failOnID: 2}
	f := newTestFirer(lookup, drv)

	err := f.Fire(context.Background(), testEvent())
	if err == nil {
		t.Fatal("expected aggregated error from failing device, got nil")
	}
	// Still attempted both devices (one bad device does not stop the others).
	if len(drv.calls) != 2 {
		t.Errorf("driver calls = %d, want 2 (all attempted)", len(drv.calls))
	}
}

func TestFirer_LookupErrorReturned(t *testing.T) {
	lookup := &fakeLookup{err: errors.New("db down")}
	drv := &fakeDriver{}
	f := newTestFirer(lookup, drv)

	if err := f.Fire(context.Background(), testEvent()); err == nil {
		t.Fatal("expected error when lookup fails, got nil")
	}
	if len(drv.calls) != 0 {
		t.Errorf("driver calls = %d, want 0 when lookup fails", len(drv.calls))
	}
}
