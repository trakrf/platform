// Package csl is a transport-only driver for CSL Intelligent Fixed Readers
// (CS463) over their HTTP "/API" interface (CSL RFID Programmer's Manual,
// HTTP API V1.4). It reads and writes per-antenna transmit power by editing the
// reader's *active* operation profile: login -> getOperProfile -> mutate
// transmitPower{N} -> setOperProfile -> logout.
//
// The single-session lock is handled by callers via a fast login/act/logout
// cycle; ForceLogout (root-only) clears a stale session when the operator
// confirms. XML is confined entirely to this package.
package csl

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultTimeout = 8 * time.Second

// MinPowerDBm / MaxPowerDBm are the reader's hardware limits (doc: 0.0–32.0 dBm,
// step 0.1). The UI clamps to a narrower practical range; the client enforces the
// hardware bound so a bad request can never push the radio out of spec.
const (
	MinPowerDBm = 0.0
	MaxPowerDBm = 32.0
)

// settableParams is the allowlist of operation-profile attributes that
// setOperProfile accepts (doc #17). On a read-modify-write we forward only these
// (sourced from the getOperProfile response) so we never echo a read-only or
// unknown attribute back — while still preserving the reader's existing config.
var settableParams = []string{
	"profile_id", "linkProfile", "populationEst", "sessionNo", "target",
	"queryAlgorithm", "reflectedPowerThreshold", "tagModel", "antenna_port",
	"transmitPower",
	"transmitPower1", "transmitPower2", "transmitPower3", "transmitPower4",
	"transmitPower5", "transmitPower6", "transmitPower7", "transmitPower8",
	"transmitPower9", "transmitPower10", "transmitPower11", "transmitPower12",
	"transmitPower13", "transmitPower14", "transmitPower15", "transmitPower16",
	"dwellTime1", "dwellTime2", "dwellTime3", "dwellTime4", "dwellTime5",
	"dwellTime6", "dwellTime7", "dwellTime8", "dwellTime9", "dwellTime10",
	"dwellTime11", "dwellTime12", "dwellTime13", "dwellTime14", "dwellTime15",
	"dwellTime16",
	"retry", "tagFocus", "fastId", "minOnChipRSSI", "maxOnChipRSSI",
	"moistAvgWindow", "tempAvgWindow", "reconfigAntennaPortError",
	"retryErrorAntennaPortTime",
	"memoryBank1", "memoryBank1Offset", "memoryBank1Length",
	"memoryBank2", "memoryBank2Offset", "memoryBank2Length",
}

// Client talks to one reader's HTTP API.
type Client struct {
	http    *http.Client
	baseURL string // e.g. http://192.168.50.212
	user    string
	pass    string
}

// New builds a Client. timeout <= 0 uses the package default (8s); the CS463
// profile round-trip is slower than a Shelly RPC, so the default is generous.
func New(baseURL, user, pass string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		http:    &http.Client{Timeout: timeout},
		baseURL: strings.TrimRight(baseURL, "/"),
		user:    user,
		pass:    pass,
	}
}

// Profile is the active operation profile as a flat attribute map plus the
// per-port powers extracted for convenience.
type Profile struct {
	ID     string
	Attrs  map[string]string
	Powers map[int]float64 // antenna port (1..16) -> dBm
}

// Result is the outcome of an Apply call.
type Result struct {
	Busy          bool           // reader had another session and force was not requested
	HolderIP      string         // who held the session (when Busy)
	ActiveProfile string         // profile_id that was edited / read
	Powers        map[int]float64 // resulting per-port powers
}

// --- XML wire types -------------------------------------------------------

type xmlError struct {
	Code           string `xml:"code,attr"`
	Msg            string `xml:"msg,attr"`
	AlreadyLoginIP string `xml:"alreadyLoginIP,attr"`
}

type xmlAck struct {
	XMLName xml.Name  `xml:"CSL"`
	Command string    `xml:"Command"`
	Ack     string    `xml:"Ack"`
	Error   *xmlError `xml:"Error"`
}

type xmlProfile struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

type xmlProfileList struct {
	XMLName  xml.Name     `xml:"CSL"`
	Command  string       `xml:"Command"`
	Profiles []xmlProfile `xml:"ProfileList>profile"`
	Error    *xmlError    `xml:"Error"`
}

// --- low-level request ----------------------------------------------------

func (c *Client) do(ctx context.Context, params url.Values, out any) error {
	u := c.baseURL + "/API?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("csl: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("csl: request to %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("csl: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("csl: %s returned status %d", c.baseURL, resp.StatusCode)
	}
	if err := xml.Unmarshal(body, out); err != nil {
		return fmt.Errorf("csl: parse xml: %w (body: %.200s)", err, body)
	}
	return nil
}

// --- session ops ----------------------------------------------------------

// Login authenticates and returns a session id. On the single-session lock
// (code -10) it returns session=="" and holderIP set (not an error) so callers
// can surface a "busy" state and optionally force.
func (c *Client) Login(ctx context.Context) (session, holderIP string, err error) {
	params := url.Values{"command": {"login"}, "username": {c.user}, "password": {c.pass}}
	var ack xmlAck
	if err := c.do(ctx, params, &ack); err != nil {
		return "", "", err
	}
	if ack.Error != nil {
		if ack.Error.Code == "-10" {
			return "", ack.Error.AlreadyLoginIP, nil // busy, not an error
		}
		return "", "", fmt.Errorf("csl: login failed: %s", ack.Error.Msg)
	}
	session = parseSessionID(ack.Ack)
	if session == "" {
		return "", "", fmt.Errorf("csl: login ok but no session_id in %q", ack.Ack)
	}
	return session, "", nil
}

// ForceLogout clears any held session (root-only command). It is invoked only on
// an operator-confirmed force.
func (c *Client) ForceLogout(ctx context.Context) error {
	params := url.Values{"command": {"forceLogout"}, "username": {c.user}, "password": {c.pass}}
	var ack xmlAck
	if err := c.do(ctx, params, &ack); err != nil {
		return err
	}
	if ack.Error != nil {
		return fmt.Errorf("csl: forceLogout failed: %s", ack.Error.Msg)
	}
	return nil
}

// Logout ends a session. Best-effort: callers defer it and ignore the error.
func (c *Client) Logout(ctx context.Context, session string) error {
	params := url.Values{"session_id": {session}, "command": {"logout"}}
	var ack xmlAck
	if err := c.do(ctx, params, &ack); err != nil {
		return err
	}
	if ack.Error != nil {
		return fmt.Errorf("csl: logout failed: %s", ack.Error.Msg)
	}
	return nil
}

// GetActiveProfile returns the operation profile flagged active="true".
func (c *Client) GetActiveProfile(ctx context.Context, session string) (Profile, error) {
	// profile_id is required by the API but the response returns ALL profiles;
	// we pass a placeholder and select the active one.
	params := url.Values{"session_id": {session}, "command": {"getOperProfile"}, "profile_id": {"_"}}
	var list xmlProfileList
	if err := c.do(ctx, params, &list); err != nil {
		return Profile{}, err
	}
	if list.Error != nil {
		return Profile{}, fmt.Errorf("csl: getOperProfile failed: %s", list.Error.Msg)
	}
	for _, p := range list.Profiles {
		attrs := attrMap(p.Attrs)
		if attrs["active"] == "true" {
			return Profile{ID: attrs["profile_id"], Attrs: attrs, Powers: extractPowers(attrs)}, nil
		}
	}
	return Profile{}, fmt.Errorf("csl: no active operation profile found")
}

// SetProfile writes the profile back via setOperProfile, forwarding only
// allowlisted (settable) attributes sourced from prof.Attrs. Callers mutate
// prof.Attrs (the transmitPower{N} keys) before calling.
func (c *Client) SetProfile(ctx context.Context, session string, prof Profile) error {
	params := url.Values{"session_id": {session}, "command": {"setOperProfile"}}
	for _, key := range settableParams {
		if v, ok := prof.Attrs[key]; ok {
			params.Set(key, v)
		}
	}
	var ack xmlAck
	if err := c.do(ctx, params, &ack); err != nil {
		return err
	}
	if ack.Error != nil {
		return fmt.Errorf("csl: setOperProfile failed: %s", ack.Error.Msg)
	}
	return nil
}

// Apply runs the full fast login -> read active -> mutate powers -> write ->
// logout cycle. powers maps antenna port (1..4) to desired dBm. When the reader
// is busy and force is false it returns Result{Busy:true} without mutating; when
// force is true it ForceLogouts first. A get (no mutation) is requested with an
// empty powers map.
func (c *Client) Apply(ctx context.Context, powers map[int]float64, force bool) (Result, error) {
	for port, dbm := range powers {
		if port < 1 || port > 16 {
			return Result{}, fmt.Errorf("csl: antenna port %d out of range 1..16", port)
		}
		if dbm < MinPowerDBm || dbm > MaxPowerDBm {
			return Result{}, fmt.Errorf("csl: power %.1f dBm out of range %.1f..%.1f", dbm, MinPowerDBm, MaxPowerDBm)
		}
	}

	if force {
		// Clear any held session first; ignore "no session" style errors.
		_ = c.ForceLogout(ctx)
	}

	session, holderIP, err := c.Login(ctx)
	if err != nil {
		return Result{}, err
	}
	if session == "" {
		return Result{Busy: true, HolderIP: holderIP}, nil
	}
	defer func() { _ = c.Logout(context.WithoutCancel(ctx), session) }()

	prof, err := c.GetActiveProfile(ctx, session)
	if err != nil {
		return Result{}, err
	}

	if len(powers) == 0 {
		// get-only: report current state.
		return Result{ActiveProfile: prof.ID, Powers: prof.Powers}, nil
	}

	for port, dbm := range powers {
		prof.Attrs["transmitPower"+strconv.Itoa(port)] = formatPower(dbm)
	}
	if err := c.SetProfile(ctx, session, prof); err != nil {
		return Result{}, err
	}
	return Result{ActiveProfile: prof.ID, Powers: extractPowers(prof.Attrs)}, nil
}

// --- helpers --------------------------------------------------------------

func parseSessionID(ack string) string {
	// Ack looks like "OK: session_id=42add4cd".
	const marker = "session_id="
	i := strings.Index(ack, marker)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(ack[i+len(marker):])
}

func attrMap(attrs []xml.Attr) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[a.Name.Local] = a.Value
	}
	return m
}

func extractPowers(attrs map[string]string) map[int]float64 {
	out := make(map[int]float64, 4)
	for port := 1; port <= 16; port++ {
		if v, ok := attrs["transmitPower"+strconv.Itoa(port)]; ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				out[port] = f
			}
		}
	}
	return out
}

func formatPower(dbm float64) string {
	return strconv.FormatFloat(dbm, 'f', 1, 64)
}
