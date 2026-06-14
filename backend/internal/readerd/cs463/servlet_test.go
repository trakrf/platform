package cs463

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeServlet records the POST body sent to /OperationProfileDetail and serves a
// canned HTML response.
type fakeServlet struct {
	srv        *httptest.Server
	status     int
	body       string
	gotPath    string
	gotCT      string
	gotForm    url.Values
	gotRawBody string
	gotMethod  string
}

func newFakeServlet(status int, body string) *fakeServlet {
	f := &fakeServlet{status: status, body: body}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.gotPath = r.URL.Path
		f.gotCT = r.Header.Get("Content-Type")
		f.gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		f.gotRawBody = string(raw)
		f.gotForm, _ = url.ParseQuery(f.gotRawBody)
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	}))
	return f
}

func (f *fakeServlet) client() *Client { return New(f.srv.URL, "root", "pw", 0) }
func (f *fakeServlet) close()          { f.srv.Close() }

// countOccurrences counts how many times key=val appears in the raw urlencoded body.
func countAntennaValues(raw string) []string {
	var out []string
	for _, kv := range strings.Split(raw, "&") {
		k, v, found := strings.Cut(kv, "=")
		if !found {
			continue
		}
		if k == "antenna%5B%5D" || k == "antenna[]" {
			dv, _ := url.QueryUnescape(v)
			out = append(out, dv)
		}
	}
	return out
}

func positionalValues(raw, name string) []string {
	enc := url.QueryEscape(name)
	var out []string
	for _, kv := range strings.Split(raw, "&") {
		k, v, found := strings.Cut(kv, "=")
		if !found {
			continue
		}
		if k == enc || k == name {
			dv, _ := url.QueryUnescape(v)
			out = append(out, dv)
		}
	}
	return out
}

func sampleProfileFields() map[string]string {
	// names as returned by GetActiveProfile (CS463 GET attr casing)
	return map[string]string{
		"linkProfile":               "1",
		"sessionNo":                 "0",
		"target":                    "2",
		"retry":                     "0",
		"queryAlgorithm":            "DynamicQ",
		"populationEst":             "50",
		"tagModel":                  "ANY",
		"reflectedPowerThreshold":   "24.0",
		"minOnChipRSSI":             "0",
		"maxOnChipRSSI":             "0",
		"moistAvgWindow":            "0",
		"tempAvgWindow":             "0",
		"retryErrorAntennaPortTime": "0",
		"dwellTime1":                "600",
		"dwellTime2":                "600",
	}
}

func TestSetProfilePower_BuildsServletBody(t *testing.T) {
	f := newFakeServlet(200, "<html>Successful!</html>")
	defer f.close()

	err := f.client().SetProfilePower(
		context.Background(),
		"TrakRF",
		4,
		[]int{1, 2},
		map[int]float64{1: 22.5, 2: 30, 3: 30, 4: 30},
		sampleProfileFields(),
	)
	if err != nil {
		t.Fatalf("SetProfilePower: %v", err)
	}

	if f.gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", f.gotMethod)
	}
	if f.gotPath != "/OperationProfileDetail" {
		t.Fatalf("path = %q, want /OperationProfileDetail", f.gotPath)
	}
	if !strings.HasPrefix(f.gotCT, "application/x-www-form-urlencoded") {
		t.Fatalf("content-type = %q, want application/x-www-form-urlencoded", f.gotCT)
	}

	// antenna[]: exactly enable1 and enable2, NOT enable3/enable4
	ant := countAntennaValues(f.gotRawBody)
	if len(ant) != 2 || ant[0] != "enable1" || ant[1] != "enable2" {
		t.Fatalf("antenna[] = %v, want [enable1 enable2]", ant)
	}
	for _, a := range ant {
		if a == "enable3" || a == "enable4" {
			t.Fatalf("antenna[] must not contain %q (disabled port)", a)
		}
	}

	// transmitpower[]: four positional values in order
	tp := positionalValues(f.gotRawBody, "transmitpower[]")
	if len(tp) != 4 {
		t.Fatalf("transmitpower[] count = %d, want 4 (%v)", len(tp), tp)
	}
	wantPow := []string{"22.5", "30", "30", "30"}
	for i, w := range wantPow {
		if tp[i] != w {
			t.Fatalf("transmitpower[%d] = %q, want %q (full: %v)", i, tp[i], w, tp)
		}
	}

	// dwelltime[]: four positional values
	dw := positionalValues(f.gotRawBody, "dwelltime[]")
	if len(dw) != 4 {
		t.Fatalf("dwelltime[] count = %d, want 4 (%v)", len(dw), dw)
	}
	if dw[0] != "600" || dw[1] != "600" {
		t.Fatalf("dwelltime[] = %v, want first two 600", dw)
	}

	// modify submit + empty password fields present; no lock variants
	if _, ok := f.gotForm["modifyprofile"]; !ok {
		t.Fatalf("modifyprofile must be present")
	}
	if _, ok := f.gotForm["modifylockprofile"]; ok {
		t.Fatalf("modifylockprofile must NOT be present")
	}
	if _, ok := f.gotForm["modifypermalockprofile"]; ok {
		t.Fatalf("modifypermalockprofile must NOT be present")
	}
	if _, ok := f.gotForm["unlockpassword"]; !ok {
		t.Fatalf("unlockpassword must be present")
	}
	if _, ok := f.gotForm["lockpassword"]; !ok {
		t.Fatalf("lockpassword must be present")
	}

	// profileid + carried profile-level fields
	if got := f.gotForm.Get("profileid"); got != "TrakRF" {
		t.Fatalf("profileid = %q, want TrakRF", got)
	}
	if got := f.gotForm.Get("linkprofile"); got != "1" {
		t.Fatalf("linkprofile = %q, want 1", got)
	}
	if got := f.gotForm.Get("sessionno"); got != "0" {
		t.Fatalf("sessionno = %q, want 0", got)
	}
	if got := f.gotForm.Get("target"); got != "2" {
		t.Fatalf("target = %q, want 2", got)
	}
	if got := f.gotForm.Get("queryalgorithm"); got != "DynamicQ" {
		t.Fatalf("queryalgorithm = %q, want DynamicQ", got)
	}
	if got := f.gotForm.Get("tagpopulation"); got != "50" {
		t.Fatalf("tagpopulation = %q, want 50 (from populationEst)", got)
	}
	if got := f.gotForm.Get("tagmodel"); got != "ANY" {
		t.Fatalf("tagmodel = %q, want ANY", got)
	}
	if got := f.gotForm.Get("reflectedpowerthreshold"); got != "24.0" {
		t.Fatalf("reflectedpowerthreshold = %q, want 24.0", got)
	}

	// prefilter (x7) + postfilter default to "none"
	pf := positionalValues(f.gotRawBody, "prefilter[]")
	if len(pf) == 0 {
		pf = f.gotForm["prefilter"]
	}
	if got := f.gotForm.Get("prefilter"); got != "none" && len(pf) == 0 {
		t.Fatalf("prefilter must default to none, got form=%v", f.gotForm["prefilter"])
	}
	if got := f.gotForm.Get("postfilter"); got != "none" {
		t.Fatalf("postfilter = %q, want none", got)
	}
}

func TestSetProfilePower_SuccessRequiresMarker(t *testing.T) {
	f := newFakeServlet(200, "<html>Operation failed: invalid power</html>")
	defer f.close()

	err := f.client().SetProfilePower(context.Background(), "TrakRF", 4,
		[]int{1}, map[int]float64{1: 30, 2: 30, 3: 30, 4: 30}, sampleProfileFields())
	if err == nil {
		t.Fatalf("expected error when body lacks Successful! marker")
	}
	if !strings.Contains(err.Error(), "failed") && !strings.Contains(err.Error(), "Successful") {
		t.Fatalf("error should include a body snippet, got: %v", err)
	}
}

func TestSetProfilePower_HTTP500(t *testing.T) {
	f := newFakeServlet(500, "internal error")
	defer f.close()

	err := f.client().SetProfilePower(context.Background(), "TrakRF", 4,
		[]int{1}, map[int]float64{1: 30, 2: 30, 3: 30, 4: 30}, sampleProfileFields())
	if err == nil {
		t.Fatalf("expected error on HTTP 500")
	}
}

func TestSetProfilePower_MissingPowerFallsBack(t *testing.T) {
	f := newFakeServlet(200, "Successful!")
	defer f.close()

	// powers omits port 3 and 4; expect fallback default for those positions.
	err := f.client().SetProfilePower(context.Background(), "TrakRF", 4,
		[]int{1, 2}, map[int]float64{1: 22.5, 2: 30}, sampleProfileFields())
	if err != nil {
		t.Fatalf("SetProfilePower: %v", err)
	}
	tp := positionalValues(f.gotRawBody, "transmitpower[]")
	if len(tp) != 4 {
		t.Fatalf("transmitpower[] count = %d, want 4 (%v)", len(tp), tp)
	}
	if tp[0] != "22.5" || tp[1] != "30" {
		t.Fatalf("transmitpower[] first two = %v, want [22.5 30]", tp[:2])
	}
	// ports 3,4 default to 30.0
	if tp[2] != "30" || tp[3] != "30" {
		t.Fatalf("transmitpower[] fallback = %v, want [30 30] for ports 3,4", tp[2:])
	}
}
