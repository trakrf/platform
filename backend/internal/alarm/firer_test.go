package alarm

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/trakrf/platform/backend/internal/geofence"
	"github.com/trakrf/platform/backend/internal/models/alarmdevice"
)

type fakeLookup struct {
	devices []alarmdevice.AlarmDevice
	err     error
	gotOrg  int
	gotPnt  int
}

func (f *fakeLookup) ListAlarmDevicesForScanPoint(_ context.Context, orgID, scanPointID int) ([]alarmdevice.AlarmDevice, error) {
	f.gotOrg, f.gotPnt = orgID, scanPointID
	return f.devices, f.err
}

type setCall struct {
	baseURL  string
	switchID int
	on       bool
}

type fakeDriver struct {
	calls   []setCall
	failURL string // returns an error when baseURL == failURL
}

func (d *fakeDriver) Set(_ context.Context, baseURL string, switchID int, on bool) error {
	d.calls = append(d.calls, setCall{baseURL, switchID, on})
	if baseURL == d.failURL {
		return errors.New("boom")
	}
	return nil
}

func testEvent() geofence.AlarmEvent {
	return geofence.AlarmEvent{OrgID: 7, AssetID: 3, ScanPointID: 42, EPC: "E2", RSSI: -50, FiredAt: time.Unix(0, 0)}
}

func newTestFirer(lookup deviceLookup, drv driver) Firer {
	log := zerolog.New(io.Discard)
	return NewFirer(lookup, drv, &log)
}

func TestFirer_FiresEachBoundDevice(t *testing.T) {
	lookup := &fakeLookup{devices: []alarmdevice.AlarmDevice{
		{ID: 1, BaseURL: "http://a", SwitchID: 0},
		{ID: 2, BaseURL: "http://b", SwitchID: 1},
	}}
	drv := &fakeDriver{}
	f := newTestFirer(lookup, drv)

	if err := f.Fire(context.Background(), testEvent()); err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	if lookup.gotOrg != 7 || lookup.gotPnt != 42 {
		t.Errorf("lookup got org=%d point=%d, want 7/42", lookup.gotOrg, lookup.gotPnt)
	}
	if len(drv.calls) != 2 {
		t.Fatalf("driver calls = %d, want 2", len(drv.calls))
	}
	want := []setCall{{"http://a", 0, true}, {"http://b", 1, true}}
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

func TestFirer_DriverErrorAggregatedNotFatal(t *testing.T) {
	lookup := &fakeLookup{devices: []alarmdevice.AlarmDevice{
		{ID: 1, BaseURL: "http://ok", SwitchID: 0},
		{ID: 2, BaseURL: "http://bad", SwitchID: 0},
	}}
	drv := &fakeDriver{failURL: "http://bad"}
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
