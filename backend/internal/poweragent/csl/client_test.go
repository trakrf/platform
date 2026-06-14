package csl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// twoProfileList mimics a real getOperProfile response: an inactive "Default"
// profile and the active "TrakRF" profile (ports 1,2 @ 30 dBm).
const twoProfileList = `<?xml version="1.0" encoding="UTF-8" standalone="no"?><CSL><Command>getOperProfile</Command><ProfileList>` +
	`<profile active="false" antennaPort="14" antenna_port="1,4" linkProfile="1" populationEst="50" sessionNo="0" target="2" queryAlgorithm="DynamicQ" reflectedPowerThreshold="24.0" tagModel="ANY" profile_id="Default Profile" dwellTime1="2000" transmitPower="30.0" transmitPower1="30.0" transmitPower2="30.0" transmitPower3="30.0" transmitPower4="30.0"/>` +
	`<profile active="true" antennaPort="12" antenna_port="1,2" linkProfile="1" populationEst="50" sessionNo="0" target="2" queryAlgorithm="DynamicQ" reflectedPowerThreshold="24.0" tagModel="ANY" profile_id="TrakRF" dwellTime1="600" dwellTime2="600" transmitPower="30.0" transmitPower1="30.0" transmitPower2="30.0" transmitPower3="0.0" transmitPower4="0.0"/>` +
	`</ProfileList></CSL>`

const loginOK = `<?xml version="1.0"?><CSL><Command>login</Command><Ack>OK: session_id=abc123</Ack></CSL>`
const loginBusy = `<?xml version="1.0"?><CSL><Command>login</Command><Error alreadyLoginIP="192.168.50.203" alreadyLoginUser="root" code="-10" msg="Error: Multiple login not allowed!"/></CSL>`
const ackOK = `<?xml version="1.0"?><CSL><Command>x</Command><Ack>OK:</Ack></CSL>`

// wipedProfileList is what a firmware that drops antenna enablement returns
// after setOperProfile: the active profile with antenna_port emptied.
const wipedProfileList = `<?xml version="1.0"?><CSL><Command>getOperProfile</Command><ProfileList>` +
	`<profile active="true" antennaPort="" antenna_port="" profile_id="TrakRF" transmitPower1="18.0" transmitPower2="30.0"/>` +
	`</ProfileList></CSL>`

// fakeReader records setOperProfile params and serves canned responses.
type fakeReader struct {
	srv          *httptest.Server
	busy         bool
	forced       bool
	setParams    url.Values
	loggedOut    bool
	wipeAfterSet bool // simulate firmware that drops antenna_port on setOperProfile
	didSet       bool
}

func newFakeReader() *fakeReader {
	f := &fakeReader{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
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
			if f.wipeAfterSet && f.didSet {
				_, _ = w.Write([]byte(wipedProfileList))
				return
			}
			_, _ = w.Write([]byte(twoProfileList))
		case "setOperProfile":
			f.setParams = q
			f.didSet = true
			_, _ = w.Write([]byte(ackOK))
		case "logout":
			f.loggedOut = true
			_, _ = w.Write([]byte(ackOK))
		default:
			_, _ = w.Write([]byte(ackOK))
		}
	}))
	return f
}

func (f *fakeReader) client() *Client { return New(f.srv.URL, "root", "pw", 0) }
func (f *fakeReader) close()          { f.srv.Close() }

func TestApply_SetsPowerOnActiveProfile(t *testing.T) {
	f := newFakeReader()
	defer f.close()

	res, err := f.client().Apply(context.Background(), map[int]float64{1: 22.5, 2: 18}, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Busy {
		t.Fatalf("unexpected busy")
	}
	if res.ActiveProfile != "TrakRF" {
		t.Fatalf("active profile = %q, want TrakRF (must select active=true, not the first profile)", res.ActiveProfile)
	}
	// setOperProfile must carry the mutated powers on the ACTIVE profile.
	if got := f.setParams.Get("profile_id"); got != "TrakRF" {
		t.Fatalf("set profile_id = %q, want TrakRF", got)
	}
	if got := f.setParams.Get("transmitPower1"); got != "22.5" {
		t.Fatalf("transmitPower1 = %q, want 22.5", got)
	}
	if got := f.setParams.Get("transmitPower2"); got != "18.0" {
		t.Fatalf("transmitPower2 = %q, want 18.0", got)
	}
	// Untouched ports preserved from the read.
	if got := f.setParams.Get("transmitPower3"); got != "0.0" {
		t.Fatalf("transmitPower3 = %q, want preserved 0.0", got)
	}
	// Core attrs preserved (read-modify-write), comma antenna_port forwarded.
	if got := f.setParams.Get("antenna_port"); got != "1,2" {
		t.Fatalf("antenna_port = %q, want 1,2", got)
	}
	if got := f.setParams.Get("queryAlgorithm"); got != "DynamicQ" {
		t.Fatalf("queryAlgorithm = %q, want preserved DynamicQ", got)
	}
	if !f.loggedOut {
		t.Fatalf("expected logout after apply (fast login/act/logout)")
	}
}

func TestApply_ForwardsAntennaEnablement(t *testing.T) {
	f := newFakeReader()
	defer f.close()
	if _, err := f.client().Apply(context.Background(), map[int]float64{1: 22}, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Both enablement forms must be forwarded so a power-only change can't drop
	// enabled ports.
	if got := f.setParams.Get("antenna_port"); got != "1,2" {
		t.Fatalf("antenna_port = %q, want 1,2", got)
	}
	if got := f.setParams.Get("antennaPort"); got != "12" {
		t.Fatalf("antennaPort = %q, want 12 (concatenated enablement form must be preserved)", got)
	}
}

func TestApply_DetectsAntennaWipe(t *testing.T) {
	f := newFakeReader()
	f.wipeAfterSet = true // firmware drops antenna_port on setOperProfile
	defer f.close()
	_, err := f.client().Apply(context.Background(), map[int]float64{1: 18}, false)
	if err == nil {
		t.Fatalf("want error when antenna enablement is wiped after set")
	}
	if !strings.Contains(err.Error(), "antenna enablement changed") {
		t.Fatalf("error %q should report antenna enablement change", err.Error())
	}
}

func TestApply_BusyWithoutForce(t *testing.T) {
	f := newFakeReader()
	f.busy = true
	defer f.close()

	res, err := f.client().Apply(context.Background(), map[int]float64{1: 20}, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Busy {
		t.Fatalf("want Busy=true on locked session")
	}
	if res.HolderIP != "192.168.50.203" {
		t.Fatalf("holderIP = %q, want 192.168.50.203", res.HolderIP)
	}
	if f.setParams != nil {
		t.Fatalf("must NOT setOperProfile while busy")
	}
}

func TestApply_ForceClearsSessionThenSets(t *testing.T) {
	f := newFakeReader()
	f.busy = true
	defer f.close()

	res, err := f.client().Apply(context.Background(), map[int]float64{1: 25}, true)
	if err != nil {
		t.Fatalf("Apply(force): %v", err)
	}
	if !f.forced {
		t.Fatalf("force must call forceLogout")
	}
	if res.Busy {
		t.Fatalf("force should proceed, not report busy")
	}
	if got := f.setParams.Get("transmitPower1"); got != "25.0" {
		t.Fatalf("transmitPower1 = %q, want 25.0 after force", got)
	}
}

func TestApply_GetOnlyDoesNotMutate(t *testing.T) {
	f := newFakeReader()
	defer f.close()

	res, err := f.client().Apply(context.Background(), map[int]float64{}, false)
	if err != nil {
		t.Fatalf("Apply(get): %v", err)
	}
	if f.setParams != nil {
		t.Fatalf("get-only must not setOperProfile")
	}
	if res.Powers[1] != 30.0 || res.Powers[2] != 30.0 {
		t.Fatalf("get powers = %v, want ports 1,2 @ 30.0", res.Powers)
	}
}

func TestApply_RejectsOutOfRange(t *testing.T) {
	f := newFakeReader()
	defer f.close()
	if _, err := f.client().Apply(context.Background(), map[int]float64{1: 99}, false); err == nil {
		t.Fatalf("want error for power 99 dBm (>32)")
	}
	if _, err := f.client().Apply(context.Background(), map[int]float64{5: -1}, false); err == nil {
		t.Fatalf("want error for negative power")
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

func TestSetProfile_OmitsUnknownAttrs(t *testing.T) {
	f := newFakeReader()
	defer f.close()
	c := f.client()
	prof := Profile{
		ID: "P",
		Attrs: map[string]string{
			"profile_id":     "P",
			"transmitPower1": "20.0",
			"antennaPort":    "12",   // settable enablement form — MUST be forwarded
			"active":         "true", // read-only, must NOT be forwarded
			"bogusAttr":      "x",    // unknown, must NOT be forwarded
		},
	}
	if err := c.SetProfile(context.Background(), "s", prof); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}
	if f.setParams.Has("active") || f.setParams.Has("bogusAttr") {
		t.Fatalf("forwarded a non-settable attr: %v", f.setParams)
	}
	if !f.setParams.Has("antennaPort") {
		t.Fatalf("antennaPort (enablement) must be forwarded")
	}
	if !strings.Contains(f.setParams.Get("transmitPower1"), "20") {
		t.Fatalf("expected transmitPower1 forwarded")
	}
}
