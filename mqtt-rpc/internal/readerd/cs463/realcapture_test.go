package cs463

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Real /API list responses captured live from cs463-212 (TRA-1002 bench, secret-free
// — the listServer capture is deliberately excluded as it carried broker creds).
// These pin the entity parsers against actual reader output, CI-safe (no hardware).

func serveCapture(t *testing.T, name string) *entFake {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read capture %s: %v", name, err)
	}
	// entFake serves the body for whatever command is requested.
	return newEntFake(map[string]string{
		"listDataFormat":      string(data),
		"listTriggeringLogic": string(data),
		"listResultantAction": string(data),
		"listEvent":           string(data),
	})
}

func TestRealCapture_ListDataFormat(t *testing.T) {
	f := serveCapture(t, "cs463-212_listDataFormat.xml")
	defer f.close()
	rows, err := f.client().ListDataFormat(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	// The live TRA-994 golden data format must parse with the trimmed numeric fields.
	df, ok := rows["TrakRF-data-format"]
	if !ok {
		t.Fatalf("TrakRF-data-format not parsed from real capture; got %d rows", len(rows))
	}
	if df["tagDataField4"] != "RSSI_Number" || df["tagDataField3"] != "AntennaPort_Number" ||
		df["field1"] != "SequenceNumber" {
		t.Fatalf("real golden data format fields unexpected: %v", df)
	}
}

func TestRealCapture_ListEvent(t *testing.T) {
	f := serveCapture(t, "cs463-212_listEvent.xml")
	defer f.close()
	rows, err := f.client().ListEvent(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := rows["MQTT"]
	if !ok {
		t.Fatalf("live MQTT event not parsed; got %d rows", len(rows))
	}
	// eventDrift reads exactly these attrs — confirm they are present in real output.
	for _, k := range []string{"operProfile_id", "triggering_logic", "resultant_action",
		"exclusivity", "duplicateEliminationWindow", "antennaDifferentiation", "enable"} {
		if _, present := ev[k]; !present {
			t.Errorf("real event row missing attr %q that eventDrift compares", k)
		}
	}
}

func TestRealCapture_ListResultantAction(t *testing.T) {
	f := serveCapture(t, "cs463-212_listResultantAction.xml")
	defer f.close()
	rows, err := f.client().ListResultantAction(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	act, ok := rows["MQTT"]
	if !ok {
		t.Fatalf("live MQTT action not parsed; got %d rows", len(rows))
	}
	// action_mode comes back as the HUMAN form on real hardware (not the enum).
	if act["action_mode"] != "Low Latency Alert to Server" {
		t.Errorf("real action_mode = %q, want human form 'Low Latency Alert to Server'", act["action_mode"])
	}
	if act["transport"] != "MQTT" {
		t.Errorf("real action transport = %q, want MQTT", act["transport"])
	}
}

func TestRealCapture_ListTriggeringLogic(t *testing.T) {
	f := serveCapture(t, "cs463-212_listTriggeringLogic.xml")
	defer f.close()
	rows, err := f.client().ListTriggeringLogic(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := rows["TrakRF"]
	if !ok {
		t.Fatalf("live TrakRF trigger not parsed; got %d rows", len(rows))
	}
	// Confirms the bench finding the golden config now matches: reader-side RSSI gate.
	if tr["mode"] != "Trigger if RSSI larger than or equal to" {
		t.Errorf("real TrakRF trigger mode = %q, want RSSI gate", tr["mode"])
	}
	if tr["capture_point"] == "" {
		t.Error("real trigger missing capture_point (concatenated form triggerDrift normalizes against)")
	}
}
