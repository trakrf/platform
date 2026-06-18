package cs463

import (
	"strings"
	"testing"
)

func cloneRow(r EntityRow) EntityRow {
	out := make(EntityRow, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

func TestGoldenEventParams(t *testing.T) {
	p := goldenEventParams()
	for k, want := range map[string]string{
		"event_id": NameEvent, "operProfile_id": NameProfile,
		"triggering_logic": NameTrigger, "resultant_action": NameAction,
		"exclusivity": "Non-exclusive", "duplicateEliminationWindow": "500",
		"antennaDifferentiation": "true", "enable": "true",
	} {
		if p.Get(k) != want {
			t.Errorf("event param %s = %q, want %q", k, p.Get(k), want)
		}
	}
}

func TestEventDrift(t *testing.T) {
	matching := EntityRow{
		"operProfile_id": NameProfile, "triggering_logic": NameTrigger,
		"resultant_action": NameAction, "exclusivity": "Non-exclusive",
		"duplicateEliminationWindow": "500", "antennaDifferentiation": "true", "enable": "true",
	}
	if eventDrift(matching) {
		t.Error("no drift expected when row matches golden")
	}
	// dedup/antDiff changes are customer-editable (TRA-1003): must NOT report drift.
	dedupChanged := cloneRow(matching)
	dedupChanged["duplicateEliminationWindow"] = "5000"
	if eventDrift(dedupChanged) {
		t.Error("no drift expected when only dedup window differs (customer-editable knob)")
	}
	antDiffChanged := cloneRow(matching)
	antDiffChanged["antennaDifferentiation"] = "false"
	if eventDrift(antDiffChanged) {
		t.Error("no drift expected when only antennaDifferentiation differs (customer-editable knob)")
	}
	// structural field changes must still report drift.
	wrongProfile := cloneRow(matching)
	wrongProfile["operProfile_id"] = "SomeOtherProfile"
	if !eventDrift(wrongProfile) {
		t.Error("drift expected when operProfile_id differs")
	}
	wrongTrigger := cloneRow(matching)
	wrongTrigger["triggering_logic"] = "SomeOtherTrigger"
	if !eventDrift(wrongTrigger) {
		t.Error("drift expected when triggering_logic differs")
	}
	disabled := cloneRow(matching)
	disabled["enable"] = "false"
	if !eventDrift(disabled) {
		t.Error("drift expected when enable=false")
	}
}

func TestGoldenDataFormatTrimmedNumeric(t *testing.T) {
	q := goldenDataFormatParams().Encode()
	for _, want := range []string{"RSSI_Number", "AntennaPort_Number", "TimeStampOfRead", "tagDataField"} {
		if !strings.Contains(q, want) {
			t.Errorf("golden data format missing %s in %s", want, q)
		}
	}
	if strings.Contains(q, "CapturePointName") || strings.Contains(q, "TimeZone") ||
		strings.Contains(q, "RFIDReaderName") {
		t.Errorf("trimmed format must drop CapturePointName/TimeZone/RFIDReaderName: %s", q)
	}
}

func TestDataFormatDrift(t *testing.T) {
	matching := EntityRow{
		"format": "JSON",
		"field1": "SequenceNumber", "label1": "sequenceNumber",
		"field2": "NumberOfTags", "label2": "numberOfTags",
		"field3": "TagDataList", "label3": "tags",
		"tagDataField1": "EPC", "tagDataLabel1": "epc",
		"tagDataField2": "TimeStampOfRead", "tagDataLabel2": "timeStampOfRead",
		"tagDataField3": "AntennaPort_Number", "tagDataLabel3": "antennaPort",
		"tagDataField4": "RSSI_Number", "tagDataLabel4": "rssi",
	}
	if dataFormatDrift(matching) {
		t.Error("no drift expected for golden-matching data format")
	}
	stringRSSI := cloneRow(matching)
	stringRSSI["tagDataField4"] = "RSSI" // string rssi instead of numeric
	if !dataFormatDrift(stringRSSI) {
		t.Error("drift expected when rssi field is not numeric RSSI_Number")
	}
	fat := cloneRow(matching)
	fat["tagDataField5"] = "CapturePointName"
	fat["tagDataLabel5"] = "capturePointName"
	if !dataFormatDrift(fat) {
		t.Error("drift expected when an extra (un-trimmed) tag field is present")
	}
}

func TestGoldenTriggerParams(t *testing.T) {
	p := goldenTriggerParams(4)
	if p.Get("mode") != "Trigger if RSSI larger than or equal to" {
		t.Errorf("trigger mode = %q, want RSSI gate", p.Get("mode"))
	}
	if p.Get("logic") != "-80" {
		t.Errorf("trigger logic (RSSI threshold) = %q, want -80", p.Get("logic"))
	}
	if p.Get("capturePoint") != "1,2,3,4" {
		t.Errorf("capturePoint = %q, want 1,2,3,4", p.Get("capturePoint"))
	}
}

func TestTriggerDrift(t *testing.T) {
	// list returns capture_point concatenated ("1234"); golden builds "1,2,3,4".
	matching := EntityRow{
		"mode": "Trigger if RSSI larger than or equal to", "logic": "-80", "capture_point": "1234",
	}
	if triggerDrift(matching, 4) {
		t.Error("no drift expected when mode + threshold + capture_point match (normalized)")
	}
	wrongMode := cloneRow(matching)
	wrongMode["mode"] = "Read Any Tags (any ID, 1 trigger per tag)"
	if !triggerDrift(wrongMode, 4) {
		t.Error("drift expected when trigger mode differs")
	}
	wrongThresh := cloneRow(matching)
	wrongThresh["logic"] = "-65"
	if !triggerDrift(wrongThresh, 4) {
		t.Error("drift expected when RSSI threshold differs")
	}
	missingAnt := cloneRow(matching)
	missingAnt["capture_point"] = "123"
	if !triggerDrift(missingAnt, 4) {
		t.Error("drift expected when capture_point misses an antenna")
	}
}

func TestActionDrift(t *testing.T) {
	matching := EntityRow{
		"server_id": NameMQTTServer, "data_format_id": NameDataFormat,
		"transport": "MQTT", "action_mode": "Low Latency Alert to Server",
	}
	if actionDrift(matching) {
		t.Error("no drift expected for golden-matching action")
	}
	// enum form of action_mode should also be accepted
	enumForm := cloneRow(matching)
	enumForm["action_mode"] = "LOW_LATENCY_ALERT_TO_SERVER"
	if actionDrift(enumForm) {
		t.Error("enum action_mode form should not be treated as drift")
	}
	wrongServer := cloneRow(matching)
	wrongServer["server_id"] = "Some Other Server"
	if !actionDrift(wrongServer) {
		t.Error("drift expected when server_id differs")
	}
}
