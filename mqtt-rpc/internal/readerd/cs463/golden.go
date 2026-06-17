package cs463

// Golden reader-entity definitions, owned as code. These are the canonical
// "TrakRF mqtt-rpc"-named entities the daemon reconciles onto every CS463 so the
// ingest chain (profile -> trigger -> action -> server -> format) is provably the
// config TRA-994 validated. All names are TrakRF mqtt-rpc-prefixed so the name
// itself is the ownership claim and never collides with factory Default/Example.

import (
	"net/url"
	"strconv"
	"strings"
)

// Entity names (spaces, exact — referenced verbatim across entities and by config).
// All TrakRF mqtt-rpc-prefixed so the name is the ownership claim. The daemon CREATES
// the Operation Profile too (Adapter.ensureProfile: setOperProfile + servlet to enable
// antenna 1 — /API alone can't enable antennas), with an antenna-1 default, but only if
// absent — an existing one is left untouched so operator Reader.SetConfig tuning
// survives. Only the MQTT Server stays hand-crafted (broker secret + TLS cert).
const (
	NameMQTTServer = "TrakRF mqtt-rpc MQTT Server"
	NameProfile    = "TrakRF mqtt-rpc Profile"
	NameDataFormat = "TrakRF mqtt-rpc Data Format"
	NameTrigger    = "TrakRF mqtt-rpc Trigger"
	NameAction     = "TrakRF mqtt-rpc Action"
	NameEvent      = "TrakRF mqtt-rpc Event"
)

// Golden tunables (validated TRA-994 bench, cs463-212). Dwell couples to dedup:
// dwell <= dedup <= dwell*antennas.
const (
	GoldenDedupMs = 500
	GoldenDwellMs = 500
	// GoldenTriggerRSSIDBm is the reader-side RSSI gate on the golden trigger: the
	// event only fires (publishes) for reads at or above this RSSI, trimming truly
	// weak reads BEFORE the publish path (the TRA-994 bottleneck). It is a documented
	// knob, well below the platform geofence gate (~-65) so it never starves presence.
	// Matches the bench-validated cs463-212 trigger.
	GoldenTriggerRSSIDBm = -80

	triggerModeRSSIGate = "Trigger if RSSI larger than or equal to"
	actionModeLowLat    = "Low Latency Alert to Server"
	managedDesc         = "Managed by TrakRF mqtt-rpc daemon — do not edit"
)

// goldenDataFields are the top-level (field/label) pairs of the trimmed format:
// only what the ingest parser reads — reader identity comes from the MQTT topic, so
// RFIDReaderName/MAC are dropped.
var goldenDataFields = []struct{ field, label string }{
	{"SequenceNumber", "sequenceNumber"},
	{"NumberOfTags", "numberOfTags"},
	{"TagDataList", "tags"},
}

// goldenTagDataFields are the per-tag (tagDataField/tagDataLabel) pairs: numeric
// antenna + numeric rssi (RSSI_Number, enabled by parser PR #502), trimmed of
// timeZone + capturePointName (~30% per-tag payload relief).
var goldenTagDataFields = []struct{ field, label string }{
	{"EPC", "epc"},
	{"TimeStampOfRead", "timeStampOfRead"},
	{"AntennaPort_Number", "antennaPort"},
	{"RSSI_Number", "rssi"},
}

func goldenDataFormatParams() url.Values {
	p := url.Values{}
	p.Set("data_format_id", NameDataFormat)
	p.Set("desc", managedDesc)
	p.Set("format", "JSON")
	for i, f := range goldenDataFields {
		n := strconv.Itoa(i + 1)
		p.Set("field"+n, f.field)
		p.Set("label"+n, f.label)
	}
	for i, f := range goldenTagDataFields {
		n := strconv.Itoa(i + 1)
		p.Set("tagDataField"+n, f.field)
		p.Set("tagDataLabel"+n, f.label)
	}
	return p
}

// capturePointCSV builds "1,2,..,N" for addTriggeringLogic's capturePoint param.
func capturePointCSV(antennaCount int) string {
	parts := make([]string, 0, antennaCount)
	for i := 1; i <= antennaCount; i++ {
		parts = append(parts, strconv.Itoa(i))
	}
	return strings.Join(parts, ",")
}

func goldenTriggerParams(antennaCount int) url.Values {
	p := url.Values{}
	p.Set("logic_id", NameTrigger)
	p.Set("desc", managedDesc)
	p.Set("mode", triggerModeRSSIGate)
	p.Set("logic", strconv.Itoa(GoldenTriggerRSSIDBm)) // RSSI threshold value
	p.Set("capturePoint", capturePointCSV(antennaCount))
	return p
}

func goldenActionParams() url.Values {
	p := url.Values{}
	p.Set("action_id", NameAction)
	p.Set("desc", managedDesc)
	p.Set("action_mode", actionModeLowLat)
	p.Set("transport", "MQTT")
	p.Set("server_id", NameMQTTServer)
	p.Set("data_format_id", NameDataFormat)
	p.Set("condition", "None")
	return p
}

func goldenEventParams() url.Values {
	p := url.Values{}
	p.Set("event_id", NameEvent)
	p.Set("desc", managedDesc)
	p.Set("operProfile_id", NameProfile)
	p.Set("exclusivity", "Non-exclusive")
	p.Set("duplicateEliminationWindow", strconv.Itoa(GoldenDedupMs))
	p.Set("antennaDifferentiation", "true")
	p.Set("triggering_logic", NameTrigger)
	p.Set("resultant_action", NameAction)
	p.Set("enable", "true")
	return p
}

// --- drift detection: compare a list* row (verbatim attr keys) to golden -------

func eventDrift(cur EntityRow) bool {
	return cur["operProfile_id"] != NameProfile ||
		cur["triggering_logic"] != NameTrigger ||
		cur["resultant_action"] != NameAction ||
		cur["exclusivity"] != "Non-exclusive" ||
		cur["duplicateEliminationWindow"] != strconv.Itoa(GoldenDedupMs) ||
		!strings.EqualFold(cur["antennaDifferentiation"], "true") ||
		!strings.EqualFold(cur["enable"], "true")
}

func triggerDrift(cur EntityRow, antennaCount int) bool {
	// list returns capture_point concatenated ("1234"); add takes "1,2,3,4" — normalize.
	wantCP := strings.ReplaceAll(capturePointCSV(antennaCount), ",", "")
	return cur["mode"] != triggerModeRSSIGate ||
		cur["logic"] != strconv.Itoa(GoldenTriggerRSSIDBm) ||
		cur["capture_point"] != wantCP
}

func actionDrift(cur EntityRow) bool {
	return cur["server_id"] != NameMQTTServer ||
		cur["data_format_id"] != NameDataFormat ||
		!strings.EqualFold(cur["transport"], "MQTT") ||
		!eqAny(cur["action_mode"], actionModeLowLat, "LOW_LATENCY_ALERT_TO_SERVER")
}

// dataFormatDrift compares the full field/label + tagDataField/tagDataLabel pair
// sets so an extra (un-trimmed) field or a string-rssi format is caught. Pairs are
// compared as ordered sets keyed by their numeric suffix.
func dataFormatDrift(cur EntityRow) bool {
	if !strings.EqualFold(cur["format"], "JSON") {
		return true
	}
	return !pairsMatch(cur, "field", "label", goldenDataFields) ||
		!pairsMatch(cur, "tagDataField", "tagDataLabel", goldenTagDataFields)
}

// pairsMatch verifies the reader's numbered field/label pairs equal golden's, with
// no extras. golden is the desired ordered pairs; fieldKey/labelKey are the attr
// prefixes ("field"/"label" or "tagDataField"/"tagDataLabel").
func pairsMatch(cur EntityRow, fieldKey, labelKey string, golden []struct{ field, label string }) bool {
	for i, g := range golden {
		n := strconv.Itoa(i + 1)
		if cur[fieldKey+n] != g.field || cur[labelKey+n] != g.label {
			return false
		}
	}
	// any field beyond len(golden) means the reader has extra (un-trimmed) fields
	if _, extra := cur[fieldKey+strconv.Itoa(len(golden)+1)]; extra {
		return false
	}
	return true
}

func eqAny(got string, wants ...string) bool {
	for _, w := range wants {
		if strings.EqualFold(got, w) {
			return true
		}
	}
	return false
}
