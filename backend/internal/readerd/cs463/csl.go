// Package cs463 is a transport-only driver for CSL Intelligent Fixed Readers
// (CS463) over their localhost HTTP "/API" interface (CSL RFID Programmer's
// Manual, HTTP API V1.4). It covers the read/session operations only: login,
// read the active operation profile, logout, force logout, and re-arm an event.
//
// The profile WRITE path (setOperProfile) is deliberately NOT implemented here:
// the /API write is broken on this firmware, so writing moves to a servlet POST
// in a separate task. XML is confined entirely to this package.
package cs463

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
// step 0.1). The client exposes the hardware bound so the write path (separate
// task) and callers can clamp against the radio spec.
const (
	MinPowerDBm = 0.0
	MaxPowerDBm = 32.0
)

// Client talks to one reader's HTTP API.
type Client struct {
	http    *http.Client
	baseURL string // e.g. http://127.0.0.1
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

// Profile is an operation profile as a flat attribute map plus the per-port
// powers extracted for convenience.
type Profile struct {
	ID     string
	Attrs  map[string]string
	Powers map[int]float64 // antenna port (1..16) -> dBm
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
		return fmt.Errorf("cs463: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cs463: request to %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("cs463: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cs463: %s returned status %d", c.baseURL, resp.StatusCode)
	}
	if err := xml.Unmarshal(body, out); err != nil {
		return fmt.Errorf("cs463: parse xml: %w (body: %.200s)", err, body)
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
		return "", "", fmt.Errorf("cs463: login failed: %s", ack.Error.Msg)
	}
	session = parseSessionID(ack.Ack)
	if session == "" {
		return "", "", fmt.Errorf("cs463: login ok but no session_id in %q", ack.Ack)
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
		return fmt.Errorf("cs463: forceLogout failed: %s", ack.Error.Msg)
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
		return fmt.Errorf("cs463: logout failed: %s", ack.Error.Msg)
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
		return Profile{}, fmt.Errorf("cs463: getOperProfile failed: %s", list.Error.Msg)
	}
	for _, p := range list.Profiles {
		attrs := attrMap(p.Attrs)
		if attrs["active"] == "true" {
			return Profile{ID: attrs["profile_id"], Attrs: attrs, Powers: extractPowers(attrs)}, nil
		}
	}
	return Profile{}, fmt.Errorf("cs463: no active operation profile found")
}

// EnableEvent enables or disables a reader event. It is used to re-arm the event
// after a profile write (the write path lives in a separate task).
func (c *Client) EnableEvent(ctx context.Context, session, eventID string, enable bool) error {
	params := url.Values{
		"session_id": {session},
		"command":    {"enableEvent"},
		"event_id":   {eventID},
		"enable":     {strconv.FormatBool(enable)},
	}
	var ack xmlAck
	if err := c.do(ctx, params, &ack); err != nil {
		return err
	}
	if ack.Error != nil {
		return fmt.Errorf("cs463: enableEvent failed: %s", ack.Error.Msg)
	}
	return nil
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
