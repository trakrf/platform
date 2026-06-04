package ingest

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// cs463Payload is the CS463 reader JSON shape (verified against live preview
// traffic 2026-06-04). rssi arrives as a quoted string (e.g. "-56").
type cs463Payload struct {
	Tags []cs463Tag `json:"tags"`
}

type cs463Tag struct {
	EPC              string `json:"epc"`
	TimeStampOfRead  int64  `json:"timeStampOfRead"` // microseconds since epoch
	AntennaPort      int    `json:"antennaPort"`
	CapturePointName string `json:"capturePointName"`
	RSSI             string `json:"rssi"`
}

func parseCS463(payload []byte) ([]scanread.Read, error) {
	var p cs463Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("cs463: unmarshal payload: %w", err)
	}
	reads := make([]scanread.Read, 0, len(p.Tags))
	for _, t := range p.Tags {
		// RSSI is informational (a TRA-901 gate input), not load-bearing for
		// asset_scans. Parse leniently — a malformed value (float, signed, blank)
		// must not discard an otherwise-valid read. ParseFloat tolerates "-56",
		// "-56.5", "+40"; on failure we keep the read with rssi 0.
		rssi := 0
		if f, err := strconv.ParseFloat(t.RSSI, 64); err == nil {
			rssi = int(math.Round(f))
		}
		reads = append(reads, scanread.Read{
			EPC:              t.EPC,
			CapturePointName: t.CapturePointName,
			AntennaPort:      t.AntennaPort,
			RSSI:             rssi,
			ReaderTimestamp:  time.UnixMicro(t.TimeStampOfRead).UTC(),
		})
	}
	return reads, nil
}
