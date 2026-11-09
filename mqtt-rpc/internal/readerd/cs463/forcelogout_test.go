package cs463

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// forceLogout logs out EVERY reader session, a person in the vendor UI included,
// so it must never happen silently.
func TestForceLogoutIsLoggedAtWarn(t *testing.T) {
	var buf bytes.Buffer
	log := zerolog.New(&buf)
	f := &fakeOps{holderIP: "192.168.50.203", profiles: []Profile{profile("TrakRF", "1,2", map[int]float64{1: 30})}}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4, EventID: "E", Logger: &log})

	if _, err := a.GetOperProfile(context.Background(), true); err != nil {
		t.Fatalf("GetOperProfile(force): %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"level":"warn"`) || !strings.Contains(out, "192.168.50.203") {
		t.Fatalf("want a warn line naming the holder, got %q", out)
	}
	if !strings.Contains(out, "all reader sessions") {
		t.Errorf("the warning must say every session is logged out, got %q", out)
	}
}

func TestNoWarnWithoutForce(t *testing.T) {
	var buf bytes.Buffer
	log := zerolog.New(&buf)
	f := &fakeOps{profiles: []Profile{profile("TrakRF", "1,2", map[int]float64{1: 30})}}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4, EventID: "E", Logger: &log})
	if _, err := a.GetOperProfile(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("a free session needs no force logout, got %q", buf.String())
	}
}
