package ingest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/storage"
)

// TRA-974: the per-message "ingest message processed" line is a log firehose at
// real read volume (~1-2 msg/sec/topic). It must be a Debug, not an Info, so it
// is silent in normal operation but recoverable via LOG_LEVEL=debug. Observability
// is unaffected — the counts are exported as Prometheus metrics.
func TestLogMessageProcessed_SilentAtInfoLevel(t *testing.T) {
	var buf bytes.Buffer
	log := zerolog.New(&buf).Level(zerolog.InfoLevel)
	s := &Subscriber{log: log}

	s.logMessageProcessed("orgslug/reader1", 7, storage.PersistResult{Inserted: 2, Dropped: map[string]int{"unknown_tag": 1}}, 3)

	if buf.Len() != 0 {
		t.Fatalf("expected no output at Info level, got: %s", buf.String())
	}
}

func TestLogMessageProcessed_EmitsAtDebugLevel(t *testing.T) {
	var buf bytes.Buffer
	log := zerolog.New(&buf).Level(zerolog.DebugLevel)
	s := &Subscriber{log: log}

	s.logMessageProcessed("orgslug/reader1", 7, storage.PersistResult{Inserted: 2, Dropped: map[string]int{"unknown_tag": 1}}, 3)

	out := buf.String()
	if !strings.Contains(out, "ingest message processed") {
		t.Fatalf("expected per-message line at Debug level, got: %s", out)
	}
	if !strings.Contains(out, `"level":"debug"`) {
		t.Fatalf("expected debug level, got: %s", out)
	}
}
