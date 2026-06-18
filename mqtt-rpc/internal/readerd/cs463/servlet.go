package cs463

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// successMarker is the substring the OperationProfileDetail servlet returns in
// its HTML response on a successful "Modify" submit.
const successMarker = "Successful!"

// defaultPowerDBm is used when a port's power is absent from the caller-supplied
// maps. The adapter is expected to pass a full positional power set (one per port);
// this is a safety net only.
//
// Dwell deliberately has NO 2000 ms fallback: it is a golden param coupled to the
// dedup window (TRA-994), and a hardcoded 2000 silently decoupled it (4x redundant
// reads/visit) whenever the reader omitted dwellTime<port>. Missing dwell falls
// back to GoldenDwellMs (golden.go), never 2000.
const defaultPowerDBm = 30.0

// SetProfilePower writes an operation profile via the reader's web-UI servlet
// POST /OperationProfileDetail. The /API setOperProfile cannot set antenna
// enablement on this firmware; the servlet can (verified on hardware).
//
// This is a read-modify-write: the caller first fetches the active profile via
// GetActiveProfile and passes its attribute map as profileFields so the unchanged
// profile-level settings (link profile, session, target, filters, ...) are
// carried through verbatim.
//
// Parameters:
//   - profileID:     the profile to modify (servlet field "profileid").
//   - antennaCount:  total physical ports on the reader (e.g. 4). transmitpower[]
//     and dwelltime[] are POSITIONAL and emitted once per port 1..antennaCount.
//   - enabledPorts:  the ports to enable; emits antenna[]=enable{N} for each, in
//     order. Disabled ports send NO antenna[] entry.
//   - powers:        full positional power per port (1..antennaCount) in dBm. The
//     adapter is expected to pass a complete set; a missing port falls back to
//     profileFields["transmitPower{N}"] then defaultPowerDBm.
//   - profileFields: the remaining profile attributes from GetActiveProfile (GET
//     attr casing, e.g. linkProfile/sessionNo/...); mapped to the servlet's
//     lowercased field names. Missing attrs use sensible defaults.
//
// Success is HTTP 200 with "Successful!" in the body. Any other status, or a 200
// without the marker, is returned as an error including a body snippet.
func (c *Client) SetProfilePower(ctx context.Context, profileID string, antennaCount int, enabledPorts []int, powers map[int]float64, profileFields map[string]string) error {
	if antennaCount <= 0 {
		// derive from the highest port we were told about
		for p := range powers {
			if p > antennaCount {
				antennaCount = p
			}
		}
		for _, p := range enabledPorts {
			if p > antennaCount {
				antennaCount = p
			}
		}
	}

	form := url.Values{}
	form.Set("profileid", profileID)

	// antenna[]: one entry per ENABLED port only, value enable{N} (1-based).
	enabled := make(map[int]bool, len(enabledPorts))
	for _, p := range enabledPorts {
		enabled[p] = true
	}
	for _, p := range enabledPorts {
		form.Add("antenna[]", "enable"+strconv.Itoa(p))
	}

	// transmitpower[] and dwelltime[]: POSITIONAL, one per port 1..N in order.
	for port := 1; port <= antennaCount; port++ {
		pw, ok := powers[port]
		if !ok {
			if v, fok := profileFields["transmitPower"+strconv.Itoa(port)]; fok {
				if f, perr := strconv.ParseFloat(v, 64); perr == nil {
					pw = f
				} else {
					pw = defaultPowerDBm
				}
			} else {
				pw = defaultPowerDBm
			}
		}
		// One decimal place to match the reader's own wire format (captured live:
		// transmitpower[]=30.0), avoiding any servlet-side numeric-format quirk.
		form.Add("transmitpower[]", strconv.FormatFloat(pw, 'f', 1, 64))

		// Source dwell: carried-through current value, else golden — never 2000.
		dwell := strconv.Itoa(GoldenDwellMs)
		if v, dok := profileFields["dwellTime"+strconv.Itoa(port)]; dok && v != "" {
			dwell = v
		}
		form.Add("dwelltime[]", dwell)
	}

	// Profile-level fields (read-modify-write). Map GET attr names -> servlet
	// lowercased field names, with sensible defaults when absent.
	setField(form, "linkprofile", profileFields, "linkProfile", "1")
	setField(form, "sessionno", profileFields, "sessionNo", "0")
	setField(form, "target", profileFields, "target", "0")
	setField(form, "retry", profileFields, "retry", "0")
	setField(form, "queryalgorithm", profileFields, "queryAlgorithm", "DynamicQ")
	setField(form, "tagpopulation", profileFields, "populationEst", "50")
	setField(form, "tagmodel", profileFields, "tagModel", "ANY")
	setField(form, "firstextrabanktype", profileFields, "firstExtraBankType", "none")
	setField(form, "firstextrabankoffset", profileFields, "firstExtraBankOffset", "0")
	setField(form, "firstextrabanklength", profileFields, "firstExtraBankLength", "0")
	setField(form, "secondextrabanktype", profileFields, "secondExtraBankType", "none")
	setField(form, "secondextrabankoffset", profileFields, "secondExtraBankOffset", "0")
	setField(form, "secondextrabanklength", profileFields, "secondExtraBankLength", "0")
	setField(form, "minonchiprssi", profileFields, "minOnChipRSSI", "0")
	setField(form, "maxonchiprssi", profileFields, "maxOnChipRSSI", "0")
	setField(form, "moistavgwindow", profileFields, "moistAvgWindow", "0")
	setField(form, "tempavgwindow", profileFields, "tempAvgWindow", "0")
	setField(form, "reflectedpowerthreshold", profileFields, "reflectedPowerThreshold", "0.0")
	setField(form, "retryerrorantennaporttime", profileFields, "retryErrorAntennaPortTime", "0")

	// prefilter (x7) + postfilter: "none" when unset.
	for i := 0; i < 7; i++ {
		form.Add("prefilter", "none")
	}
	form.Set("postfilter", "none")

	// "Modify" submit (plain, NOT lock/permalock) + empty password fields.
	form.Set("modifyprofile", "")
	form.Set("unlockpassword", "")
	form.Set("lockpassword", "")

	u := c.baseURL + "/OperationProfileDetail"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("cs463: build servlet request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cs463: servlet request to %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("cs463: read servlet response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cs463: servlet returned status %d: %.200s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), successMarker) {
		return fmt.Errorf("cs463: servlet write not confirmed (no %q marker): %.200s", successMarker, body)
	}
	return nil
}

// setField copies profileFields[getName] into form[servletName], or a default.
func setField(form url.Values, servletName string, profileFields map[string]string, getName, def string) {
	if v, ok := profileFields[getName]; ok && v != "" {
		form.Set(servletName, v)
		return
	}
	form.Set(servletName, def)
}
