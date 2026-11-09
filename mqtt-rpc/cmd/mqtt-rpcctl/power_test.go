package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

func TestPowerRequest(t *testing.T) {
	cases := []struct {
		name, action string
		yes          bool
		wantMethod   string
		wantErr      string // substring; empty means no error
	}{
		{"reboot needs -yes", "reboot", false, "", "-yes"},
		{"shutdown needs -yes and says what it costs", "shutdown", false, "", "power-cycle"},
		{"reboot", "reboot", true, readerrpc.MethodReboot, ""},
		{"shutdown", "shutdown", true, readerrpc.MethodShutdown, ""},
		{"unknown action", "halt", true, "", "unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			method, params, err := powerRequest(c.action, c.yes, "mike@bench")
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if method != c.wantMethod {
				t.Errorf("method = %q, want %q", method, c.wantMethod)
			}
			var p readerrpc.PowerParams
			if err := json.Unmarshal(params, &p); err != nil {
				t.Fatal(err)
			}
			if p.RequestedBy != "mike@bench" {
				t.Errorf("requested_by = %q, want mike@bench", p.RequestedBy)
			}
		})
	}
}

func TestDefaultRequestedBy(t *testing.T) {
	host, _ := os.Hostname() // "" in a container without one
	want := "mike@" + host
	if host == "" {
		want = "mike"
	}
	t.Setenv("USER", "mike")
	if got := defaultRequestedBy(); got != want {
		t.Errorf("defaultRequestedBy = %q, want %q", got, want)
	}

	t.Setenv("USER", "")
	if got := defaultRequestedBy(); got != host {
		t.Errorf("defaultRequestedBy with no USER = %q, want the bare hostname %q", got, host)
	}
}
