package cs463

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// twoProfileList mimics a real getOperProfile response: an inactive "Default"
// profile and the active "TrakRF" profile (ports 1,2 @ 30 dBm).
const twoProfileList = `<?xml version="1.0" encoding="UTF-8" standalone="no"?><CSL><Command>getOperProfile</Command><ProfileList>` +
	`<profile active="false" antennaPort="14" antenna_port="1,4" linkProfile="1" populationEst="50" sessionNo="0" target="2" queryAlgorithm="DynamicQ" reflectedPowerThreshold="24.0" tagModel="ANY" profile_id="Default Profile" dwellTime1="2000" transmitPower="30.0" transmitPower1="30.0" transmitPower2="30.0" transmitPower3="30.0" transmitPower4="30.0"/>` +
	`<profile active="true" antennaPort="12" antenna_port="1,2" linkProfile="1" populationEst="50" sessionNo="0" target="2" queryAlgorithm="DynamicQ" reflectedPowerThreshold="24.0" tagModel="ANY" profile_id="TrakRF" dwellTime1="600" dwellTime2="600" transmitPower="30.0" transmitPower1="30.0" transmitPower2="22.5" transmitPower3="0.0" transmitPower4="0.0"/>` +
	`</ProfileList></CSL>`

const loginOK = `<?xml version="1.0"?><CSL><Command>login</Command><Ack>OK: session_id=abc123</Ack></CSL>`
const loginBusy = `<?xml version="1.0"?><CSL><Command>login</Command><Error alreadyLoginIP="192.168.50.203" alreadyLoginUser="root" code="-10" msg="Error: Multiple login not allowed!"/></CSL>`
const ackOK = `<?xml version="1.0"?><CSL><Command>x</Command><Ack>OK:</Ack></CSL>`

// fakeReader records request params and serves canned responses.
type fakeReader struct {
	srv       *httptest.Server
	busy      bool
	forced    bool
	loggedOut bool
	lastQuery url.Values
}

func newFakeReader() *fakeReader {
	f := &fakeReader{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.lastQuery = q
		w.Header().Set("Content-Type", "application/xml")
		switch q.Get("command") {
		case "login":
			if f.busy {
				_, _ = w.Write([]byte(loginBusy))
				return
			}
			_, _ = w.Write([]byte(loginOK))
		case "forceLogout":
			f.forced = true
			f.busy = false
			_, _ = w.Write([]byte(ackOK))
		case "getOperProfile":
			_, _ = w.Write([]byte(twoProfileList))
		case "logout":
			f.loggedOut = true
			_, _ = w.Write([]byte(ackOK))
		case "enableEvent":
			_, _ = w.Write([]byte(ackOK))
		default:
			_, _ = w.Write([]byte(ackOK))
		}
	}))
	return f
}

func (f *fakeReader) client() *Client { return New(f.srv.URL, "root", "pw", 0) }
func (f *fakeReader) close()          { f.srv.Close() }

func TestLogin_ParsesSession(t *testing.T) {
	f := newFakeReader()
	defer f.close()

	session, holderIP, err := f.client().Login(context.Background())
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if session != "abc123" {
		t.Fatalf("session = %q, want abc123", session)
	}
	if holderIP != "" {
		t.Fatalf("holderIP = %q, want empty", holderIP)
	}
}

func TestLogin_BusyReturnsHolderIP(t *testing.T) {
	f := newFakeReader()
	f.busy = true
	defer f.close()

	session, holderIP, err := f.client().Login(context.Background())
	if err != nil {
		t.Fatalf("Login (busy) must not error: %v", err)
	}
	if session != "" {
		t.Fatalf("session = %q, want empty on busy", session)
	}
	if holderIP != "192.168.50.203" {
		t.Fatalf("holderIP = %q, want 192.168.50.203", holderIP)
	}
}

func TestGetActiveProfile_SelectsActive(t *testing.T) {
	f := newFakeReader()
	defer f.close()

	prof, err := f.client().GetActiveProfile(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("GetActiveProfile: %v", err)
	}
	if prof.ID != "TrakRF" {
		t.Fatalf("profile ID = %q, want TrakRF (must select active=true, not the first profile)", prof.ID)
	}
	if got := prof.Attrs["antenna_port"]; got != "1,2" {
		t.Fatalf("antenna_port = %q, want 1,2", got)
	}
	if prof.Powers[1] != 30.0 {
		t.Fatalf("Powers[1] = %v, want 30.0", prof.Powers[1])
	}
	if prof.Powers[2] != 22.5 {
		t.Fatalf("Powers[2] = %v, want 22.5", prof.Powers[2])
	}
	if prof.Powers[3] != 0.0 {
		t.Fatalf("Powers[3] = %v, want 0.0", prof.Powers[3])
	}
	// confirm the getOperProfile placeholder profile_id was sent
	if got := f.lastQuery.Get("profile_id"); got != "_" {
		t.Fatalf("profile_id query = %q, want placeholder _", got)
	}
}

func TestEnableEvent_Disable(t *testing.T) {
	f := newFakeReader()
	defer f.close()

	if err := f.client().EnableEvent(context.Background(), "abc123", "evt1", false); err != nil {
		t.Fatalf("EnableEvent: %v", err)
	}
	if got := f.lastQuery.Get("command"); got != "enableEvent" {
		t.Fatalf("command = %q, want enableEvent", got)
	}
	if got := f.lastQuery.Get("event_id"); got != "evt1" {
		t.Fatalf("event_id = %q, want evt1", got)
	}
	if got := f.lastQuery.Get("enable"); got != "false" {
		t.Fatalf("enable = %q, want false", got)
	}
	if got := f.lastQuery.Get("session_id"); got != "abc123" {
		t.Fatalf("session_id = %q, want abc123", got)
	}
}

func TestEnableEvent_Enable(t *testing.T) {
	f := newFakeReader()
	defer f.close()

	if err := f.client().EnableEvent(context.Background(), "abc123", "evt1", true); err != nil {
		t.Fatalf("EnableEvent: %v", err)
	}
	if got := f.lastQuery.Get("enable"); got != "true" {
		t.Fatalf("enable = %q, want true", got)
	}
}

func TestForceLogout_AckParse(t *testing.T) {
	f := newFakeReader()
	defer f.close()

	if err := f.client().ForceLogout(context.Background()); err != nil {
		t.Fatalf("ForceLogout: %v", err)
	}
	if !f.forced {
		t.Fatalf("forceLogout must be issued")
	}
	if got := f.lastQuery.Get("command"); got != "forceLogout" {
		t.Fatalf("command = %q, want forceLogout", got)
	}
}

func TestLogout_AckParse(t *testing.T) {
	f := newFakeReader()
	defer f.close()

	if err := f.client().Logout(context.Background(), "abc123"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if !f.loggedOut {
		t.Fatalf("logout must be issued")
	}
	if got := f.lastQuery.Get("session_id"); got != "abc123" {
		t.Fatalf("session_id = %q, want abc123", got)
	}
}

func TestParseSessionID(t *testing.T) {
	if got := parseSessionID("OK: session_id=42add4cd"); got != "42add4cd" {
		t.Fatalf("parseSessionID = %q", got)
	}
	if got := parseSessionID("OK:"); got != "" {
		t.Fatalf("parseSessionID(no id) = %q, want empty", got)
	}
}
