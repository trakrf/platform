package ingest

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// cs463Payload is the CS463 reader JSON shape. rssi arrives either as a quoted
// string (legacy "sensor-os-data-format", e.g. "-56") or as a JSON number (the
// TRA-994 golden "TrakRF-data-format" via the RSSI_Number field, e.g. -56) —
// rssiValue accepts both.
type cs463Payload struct {
	Tags []cs463Tag `json:"tags"`
}

type cs463Tag struct {
	EPC             string    `json:"epc"`
	TimeStampOfRead int64     `json:"timeStampOfRead"` // microseconds since epoch
	AntennaPort     int       `json:"antennaPort"`
	RSSI            rssiValue `json:"rssi"`
}

// rssiValue tolerates rssi as a JSON number or a quoted string, rounding to the
// nearest int. RSSI is informational (a TRA-901 gate input), never load-bearing
// for asset_scans, so a blank/garbage value yields 0 and must not fail the read
// or the batch.
type rssiValue int

func (r *rssiValue) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*r = 0
		return nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		*r = rssiValue(math.Round(f))
	}
	return nil
}

func parseCS463(payload []byte) ([]scanread.Read, error) {
	var p cs463Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("cs463: unmarshal payload: %w", err)
	}
	reads := make([]scanread.Read, 0, len(p.Tags))
	for _, t := range p.Tags {
		// AntennaPort routes the read to its per-antenna scan_point downstream
		// (TRA-956). A payload that omits antennaPort (0) is a single-antenna
		// read and resolves to antenna 1.
		antennaPort := t.AntennaPort
		if antennaPort < 1 {
			antennaPort = 1
		}
		reads = append(reads, scanread.Read{
			EPC:             t.EPC,
			AntennaPort:     antennaPort,
			RSSI:            int(t.RSSI), // rssiValue already tolerated string/number
			ReaderTimestamp: time.UnixMicro(t.TimeStampOfRead).UTC(),
		})
	}
	return reads, nil
}
