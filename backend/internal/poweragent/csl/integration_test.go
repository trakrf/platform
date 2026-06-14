package csl

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveReadOnly hits a real CS463 and reads the active profile WITHOUT
// mutating it. Skipped unless READER_ADDR is set, e.g.:
//
//	READER_ADDR=http://192.168.50.212 READER_USER=root READER_PASS='…' \
//	  go test ./internal/poweragent/csl/ -run TestLiveReadOnly -v
func TestLiveReadOnly(t *testing.T) {
	addr := os.Getenv("READER_ADDR")
	if addr == "" {
		t.Skip("READER_ADDR not set; skipping live reader test")
	}
	c := New(addr, env("READER_USER", "root"), os.Getenv("READER_PASS"), 10*time.Second)
	res, err := c.Apply(context.Background(), map[int]float64{}, false) // get-only
	if err != nil {
		t.Fatalf("live get: %v", err)
	}
	if res.Busy {
		t.Fatalf("reader busy (held by %s); free it and retry", res.HolderIP)
	}
	t.Logf("active profile = %q, powers = %v", res.ActiveProfile, res.Powers)
	if res.ActiveProfile == "" {
		t.Fatalf("no active profile parsed from live reader")
	}
	if len(res.Powers) == 0 {
		t.Fatalf("no transmitPower attrs parsed from live reader")
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
