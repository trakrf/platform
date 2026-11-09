package geofence

import (
	"context"
	"errors"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

// TRA-1314: every drive leaves an output_drives row naming the device and the
// result, and the alarm row is written before the drives so each ON can name
// the fire it delivered.

// undelivered reads the global undelivered-alarm counter; tests assert deltas.
func undelivered(t *testing.T) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := metricUndelivered.Write(m); err != nil {
		t.Fatalf("read undelivered counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestEvaluate_RecordsEachDeviceDriveWithAlarmID(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(1, nil), dev(2, nil)}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{})
	defer e.Stop()
	at := time.Unix(1000, 0)

	e.Evaluate(context.Background(), 42, 9, at, []storage.ResolvedRead{read("EPC1", -50)})

	drives := s.recordedDrives()
	if len(drives) != 2 {
		t.Fatalf("drive rows = %d, want one per device (2)", len(drives))
	}
	for i, d := range drives {
		if d.OutputDeviceID != i+1 || !d.On || d.Cause != storage.DriveCauseAlarm || d.Err != nil {
			t.Errorf("drive %d = %+v, want device %d, ON, cause alarm, ok", i, d, i+1)
		}
		if d.AlarmEventID == nil || *d.AlarmEventID != 1 {
			t.Errorf("drive %d alarm id = %v, want the fire's id 1", i, d.AlarmEventID)
		}
		if !d.DrivenAt.Equal(at) {
			t.Errorf("drive %d driven_at = %v, want the read time %v", i, d.DrivenAt, at)
		}
	}
}

func TestEvaluate_FailedDriveRecordsErrorAndCountsUndelivered(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(1, nil)}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{err: errors.New("device unreachable: dial tcp 192.0.2.10:80: i/o timeout")})
	defer e.Stop()
	before := undelivered(t)

	e.Evaluate(context.Background(), 42, 9, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})

	drives := s.recordedDrives()
	if len(drives) != 1 || drives[0].Err == nil || drives[0].Err.Error() != "device unreachable: dial tcp 192.0.2.10:80: i/o timeout" {
		t.Fatalf("drives = %+v, want one row carrying the timeout", drives)
	}
	if got := undelivered(t) - before; got != 1 {
		t.Errorf("undelivered alarms += %v, want 1", got)
	}
	if len(s.rows) != 1 {
		t.Errorf("alarm rows = %d, want 1: the detection is still recorded", len(s.rows))
	}
}

func TestEvaluate_OneDeliveredDeviceIsNotUndelivered(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(1, nil), dev(2, nil)}}
	d := &flakyDriver{fail: map[int]bool{0: true}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()
	before := undelivered(t)

	e.Evaluate(context.Background(), 42, 9, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})

	if got := undelivered(t) - before; got != 0 {
		t.Errorf("undelivered alarms += %v, want 0 when one of two devices worked", got)
	}
}

func TestEvaluate_AlarmInsertFailureStillDrivesAndRecords(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(1, nil)}, insertErr: errors.New("db down")}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	e.Evaluate(context.Background(), 42, 9, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})

	if d.onCount() != 1 {
		t.Fatalf("ON drives = %d, want 1: a failed alarm write must not stop the horn", d.onCount())
	}
	drives := s.recordedDrives()
	if len(drives) != 1 || drives[0].AlarmEventID != nil || drives[0].Cause != storage.DriveCauseAlarm {
		t.Fatalf("drives = %+v, want one alarm-cause row with no alarm id", drives)
	}
}

func TestEvaluate_DriveRecordFailureDoesNotStopTheNextDevice(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(1, nil), dev(2, nil)}, driveErr: errors.New("db down")}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	e.Evaluate(context.Background(), 42, 9, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})

	if d.onCount() != 2 {
		t.Fatalf("ON drives = %d, want 2", d.onCount())
	}
}

func TestEvaluate_PresenceRefreshRecordsCauseRefresh(t *testing.T) {
	s := presenceStore()
	e := presenceEngine(t, &fakeDriver{}, s)
	t0 := time.Unix(1000, 0)

	evalAt(e, 1, t0)
	evalAt(e, 2, t0.Add(presenceRefreshEvery))

	drives := s.recordedDrives()
	if len(drives) != 2 {
		t.Fatalf("drive rows = %d, want the edge ON and one refresh", len(drives))
	}
	if drives[0].Cause != storage.DriveCauseAlarm || drives[0].AlarmEventID == nil {
		t.Errorf("first drive = %+v, want the alarm ON naming its fire", drives[0])
	}
	if drives[1].Cause != storage.DriveCauseRefresh || drives[1].AlarmEventID != nil || !drives[1].On {
		t.Errorf("second drive = %+v, want a refresh ON with no alarm id", drives[1])
	}
}

func TestEvaluate_PresenceAgeOutRecordsOffRow(t *testing.T) {
	s := presenceStore()
	e := presenceEngine(t, &fakeDriver{}, s)
	var timers []*recordedTimer
	e.presence.afterFunc = func(d time.Duration, f func()) timerHandle {
		rt := &recordedTimer{d: d, f: f}
		timers = append(timers, rt)
		return rt
	}

	evalAt(e, 1, time.Unix(1000, 0))
	timers[len(timers)-1].fire()

	drives := s.recordedDrives()
	if len(drives) != 2 {
		t.Fatalf("drive rows = %d, want ON then the age-out OFF", len(drives))
	}
	off := drives[1]
	if off.On || off.Cause != storage.DriveCauseAgeOut || off.AlarmEventID != nil || off.OutputDeviceID != 11 || off.Err != nil {
		t.Errorf("age-out drive = %+v, want device 11 OFF, cause age_out, no alarm id, ok", off)
	}
	if s.driveOrgs[1] != 42 {
		t.Errorf("age-out drive org = %d, want 42", s.driveOrgs[1])
	}
}

// A drive over the broker is only handed off: a powered-off CS463 still lets
// the publish succeed. Its row must not claim the device was reached.
func TestEvaluate_BrokerDriveIsRecordedUnconfirmed(t *testing.T) {
	gpo := outputdevice.OutputDevice{ID: 1, Transport: outputdevice.TransportMQTT, Type: outputdevice.TypeCSLGPO}
	s := &fakeStore{devices: []outputdevice.OutputDevice{gpo}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{})
	defer e.Stop()

	e.Evaluate(context.Background(), 42, 9, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})

	drives := s.recordedDrives()
	if len(drives) != 1 || !drives[0].Unconfirmed || drives[0].Err != nil {
		t.Fatalf("drives = %+v, want one unconfirmed row with no error", drives)
	}
}

func TestEvaluate_HTTPDriveIsRecordedConfirmed(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(1, nil)}}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, &fakeDriver{})
	defer e.Stop()

	e.Evaluate(context.Background(), 42, 9, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})

	if drives := s.recordedDrives(); len(drives) != 1 || drives[0].Unconfirmed {
		t.Fatalf("drives = %+v, want one confirmed row", drives)
	}
}
