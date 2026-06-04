package ingest

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
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

func parseCS463(payload []byte) ([]Read, error) {
	var p cs463Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("cs463: unmarshal payload: %w", err)
	}
	reads := make([]Read, 0, len(p.Tags))
	for _, t := range p.Tags {
		rssi := 0
		if t.RSSI != "" {
			v, err := strconv.Atoi(t.RSSI)
			if err != nil {
				return nil, fmt.Errorf("cs463: parse rssi %q: %w", t.RSSI, err)
			}
			rssi = v
		}
		reads = append(reads, Read{
			EPC:              t.EPC,
			CapturePointName: t.CapturePointName,
			AntennaPort:      t.AntennaPort,
			RSSI:             rssi,
			ReaderTimestamp:  time.UnixMicro(t.TimeStampOfRead).UTC(),
		})
	}
	return reads, nil
}
