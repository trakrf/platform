package cs463

// Live hardware integration test for the golden-config reconcile. Skipped unless
// CS463_LIVE is set (so it never runs in CI). It drives a REAL CS463 over its /API:
//
//	CS463_LIVE=1 CS463_IP=192.168.50.212 CS463_WEB_PASS=… \
//	  go test ./internal/readerd/cs463/ -run TestLiveReconcile -v
//
// It ONLY ever creates/deletes TrakRF mqtt-rpc-named entities (+ a dummy localhost
// golden server so the active golden event never publishes to a real broker) — never
// stock/Default/Example entities. NOTE: enabling the golden event activates its
// referenced profile (TrakRF mqtt-rpc Profile), so this test DOES switch the active
// profile — run it on a reader you are commissioning, not one you need left on another
// profile. Teardown deletes every entity it made. Each /API session is exclusive (the
// CS463 single-session lock), so the test never holds a session across
// Adapter.Reconcile (which opens its own).

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"
)

func newLiveClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("CS463_LIVE") == "" {
		t.Skip("set CS463_LIVE=1 (and CS463_IP/CS463_WEB_PASS) to run the live hardware test")
	}
	ip, pass := os.Getenv("CS463_IP"), os.Getenv("CS463_WEB_PASS")
	if ip == "" || pass == "" {
		t.Skip("CS463_IP and CS463_WEB_PASS required for the live test")
	}
	return New("http://"+ip, "root", pass, 0)
}

// delGolden removes every entity the test creates (idempotent — missing is fine).
// The event must be disabled before it can be deleted ("Event is enabled. Disable
// before delete.").
func delGolden(ctx context.Context, c *Client, session string) {
	_ = c.EnableEvent(ctx, session, NameEvent, false)
	dels := []struct{ cmd, key, id string }{
		{"delEvent", "event_id", NameEvent},
		{"delResultantAction", "action_id", NameAction},
		{"delTriggeringLogic", "logic_id", NameTrigger},
		{"delDataFormat", "data_format_id", NameDataFormat},
		{"delServerID", "server_id", NameMQTTServer},  // note: delServerID, not delServer
		{"delOperProfile", "profile_id", NameProfile}, // our own profile (not stock Default)
	}
	for _, d := range dels {
		_ = c.writeEntity(ctx, session, d.cmd, url.Values{d.key: {d.id}})
	}
}

func TestLiveReconcile(t *testing.T) {
	c := newLiveClient(t)
	ctx := context.Background()
	const antennas = 4

	// withSession runs fn inside one exclusive /API session, released on return.
	withSession := func(fn func(session string)) {
		sctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		s, holder, err := c.Login(sctx)
		if err != nil {
			t.Fatalf("live login: %v", err)
		}
		if s == "" {
			t.Fatalf("reader busy (held by %s)", holder)
		}
		defer func() { _ = c.Logout(sctx, s) }()
		fn(s)
	}

	withSession(func(s string) {
		p, err := c.GetActiveProfile(ctx, s)
		if err != nil {
			t.Fatalf("read active profile: %v", err)
		}
		t.Logf("active profile before = %q (golden event will activate %q)", p.ID, NameProfile)
		delGolden(ctx, c, s) // clean start
	})
	defer func() { withSession(func(s string) { delGolden(ctx, c, s) }) }() // restore

	// --- prereqs + the three reconcileGolden cases, in one exclusive session ---
	withSession(func(s string) {
		// golden server: dummy localhost broker so the active golden event never
		// publishes to a real broker.
		if err := c.writeEntity(ctx, s, "setServerID", url.Values{
			"server_id": {NameMQTTServer}, "desc": {"TRA-1002 live test"}, "type": {"MQTT"},
			"server_ip": {"127.0.0.1"}, "server_port": {"1883"}, "enable_ssl": {"false"},
			"clean_session": {"true"}, "qos": {"0"},
			"topic": {"trakrf.id/cs463-212-recon-test/reads"}, "client_id": {"cs463-212-recon-test"},
		}); err != nil {
			t.Fatalf("create golden server: %v", err)
		}
		// Pre-create our own profile (verify-exists prereq). setOperProfile can't
		// enable antennas (footgun) but the profile only needs to EXIST for the
		// reconcile-logic test; a real commission clones a stock profile's antennas.
		if err := c.writeEntity(ctx, s, "setOperProfile", url.Values{
			"profile_id": {NameProfile}, "linkProfile": {"1"}, "populationEst": {"50"},
			"sessionNo": {"0"}, "target": {"2"}, "queryAlgorithm": {"DynamicQ"},
			"reflectedPowerThreshold": {"24"}, "tagModel": {"ANY"}, "antenna_port": {"1,2"},
			"transmitPower": {"16.0"}, "dwellTime1": {"500"}, "dwellTime2": {"500"},
		}); err != nil {
			t.Fatalf("create golden profile: %v", err)
		}

		if err := verifyServerAndProfile(ctx, s, c); err != nil {
			t.Fatalf("verifyServerAndProfile should pass with prereqs present: %v", err)
		}

		// 1. create path
		changed, err := reconcileGolden(ctx, s, c, antennas)
		if err != nil {
			t.Fatalf("reconcile (create): %v", err)
		}
		if !changed {
			t.Fatal("first reconcile must report changed=true (created the four entities)")
		}
		assertGoldenPresentNoDrift(t, ctx, c, s, antennas)
		// the golden event references NameProfile and is enabled -> it must now be active
		if active, _ := c.GetActiveProfile(ctx, s); active.ID != NameProfile {
			t.Fatalf("golden event should have activated %q; active=%q", NameProfile, active.ID)
		}

		// 2. idempotent no-op
		changed, err = reconcileGolden(ctx, s, c, antennas)
		if err != nil {
			t.Fatalf("reconcile (noop): %v", err)
		}
		if changed {
			t.Fatal("second reconcile must be a no-op (changed=false) — converged reader")
		}

		// 3. drift correction
		drift := goldenEventParams()
		drift.Set("duplicateEliminationWindow", "5000")
		if err := c.ModEvent(ctx, s, drift); err != nil {
			t.Fatalf("inject drift: %v", err)
		}
		changed, err = reconcileGolden(ctx, s, c, antennas)
		if err != nil {
			t.Fatalf("reconcile (drift): %v", err)
		}
		if !changed {
			t.Fatal("reconcile after drift must report changed=true")
		}
		evs, _ := c.ListEvent(ctx, s)
		if got := evs[NameEvent]["duplicateEliminationWindow"]; got != "500" {
			t.Fatalf("event not reconciled back to golden dedup: got %q want 500", got)
		}
	})

	// --- 4. full Adapter.Reconcile path on its OWN session (verify + reconcile +
	// unconditional re-arm). Converged now, so no entity writes, but it still re-arms. ---
	a := NewAdapter(c, AdapterConfig{AntennaCount: antennas, EventID: NameEvent})
	if err := a.Reconcile(ctx); err != nil {
		t.Fatalf("Adapter.Reconcile (converged): %v", err)
	}
	t.Log("live reconcile validated: create, idempotent no-op, drift correction, adapter path")
}

func assertGoldenPresentNoDrift(t *testing.T, ctx context.Context, c *Client, session string, antennas int) {
	t.Helper()
	dfs, _ := c.ListDataFormat(ctx, session)
	if r, ok := dfs[NameDataFormat]; !ok || dataFormatDrift(r) {
		t.Errorf("data format missing or drifted after create: ok=%v row=%v", ok, r)
	}
	trs, _ := c.ListTriggeringLogic(ctx, session)
	if r, ok := trs[NameTrigger]; !ok || triggerDrift(r, antennas) {
		t.Errorf("trigger missing or drifted after create: ok=%v row=%v", ok, r)
	}
	acts, _ := c.ListResultantAction(ctx, session)
	if r, ok := acts[NameAction]; !ok || actionDrift(r) {
		t.Errorf("action missing or drifted after create: ok=%v row=%v", ok, r)
	}
	evs, _ := c.ListEvent(ctx, session)
	if r, ok := evs[NameEvent]; !ok || eventDrift(r) {
		t.Errorf("event missing or drifted after create: ok=%v row=%v", ok, r)
	}
}
