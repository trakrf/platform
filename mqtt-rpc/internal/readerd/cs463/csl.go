// Package cs463 is a transport-only driver for CSL Intelligent Fixed Readers
// (CS463) over their localhost HTTP "/API" interface (CSL RFID Programmer's
// Manual, HTTP API V1.4). It covers the read/session operations only: login,
// read the active operation profile, logout, force logout, and re-arm an event.
//
// The profile WRITE path (setOperProfile) is deliberately NOT implemented here:
// the /API write is broken on this firmware, so writing moves to a servlet POST
// in a separate task. XML is confined entirely to this package.
//
// Vendor references, including the GPIO wiring guide whose polarity rule the GPO
// commands depend on: docs/cs463-reader-references.md.
package cs463

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultTimeout = 8 * time.Second

// MinPowerDBm / MaxPowerDBm are the CS463's OPERATIONAL transmit-power range:
// 10.0–31.5 dBm in 0.5 dB steps. The reader is Indy RS2000-based — the module
// tops out at 31.5 dBm conducted, and below ~10 dBm the PA chain is uncalibrated
// and the read zone collapses to a few inches (operationally meaningless). The
// firmware's HTTP API will technically accept 0.0–32.0 (its raw field range), but
// that is not the usable/calibrated envelope, so capabilities and the write guard
// use the operational range. CSL further derates to ~30 dBm at the connector.
const (
	MinPowerDBm = 10.0
	MaxPowerDBm = 31.5
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
	// A cookie jar persists the JSESSIONID set by the web-UI form login (/Login)
	// across requests, so the servlet write (/OperationProfileDetail) is
	// authenticated. The /API session_id login is unaffected by the jar.
	jar, _ := cookiejar.New(nil)
	return &Client{
		http:    &http.Client{Timeout: timeout, Jar: jar},
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

// LoginServlet performs the reader's web-UI form login (POST /Login) so the
// JSESSIONID cookie is established in the client's cookie jar. The servlet write
// path (OperationProfileDetail) authenticates via this cookie, NOT via the /API
// session_id — without it the reader silently returns its login HTML and the
// write fails (verified on hardware). Must be called before SetProfilePower.
//
// The reader replies 302 (redirect) on success and 200 in some firmware; both
// 2xx and 3xx are treated as success. The cookie jar captures Set-Cookie on the
// initial response before any redirect is followed, so the JSESSIONID persists.
func (c *Client) LoginServlet(ctx context.Context) error {
	form := url.Values{"username": {c.user}, "password": {c.pass}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/Login", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("cs463: build form-login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cs463: form login to %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("cs463: form login returned status %d", resp.StatusCode)
	}
	return nil
}

// LogoutServlet ends the web-UI session by issuing GET /Logout, which carries the
// JSESSIONID cookie via the client's cookie jar. The CS463 allows only one root
// login at a time and the web (cookie) session occupies that single slot just as
// the /API session_id does; releasing the web session here frees the slot so a
// subsequent /API login can succeed (verified on hardware). Best-effort: the
// reader replies 200 or a redirect; both 2xx and 3xx are treated as success.
func (c *Client) LogoutServlet(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/Logout", nil)
	if err != nil {
		return fmt.Errorf("cs463: build servlet-logout request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cs463: servlet logout to %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("cs463: servlet logout returned status %d", resp.StatusCode)
	}
	return nil
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

// --- GPIO -----------------------------------------------------------------

// DirectIOOutput drives a general purpose output port high or low.
//
// Unlike the session-bound runIO_output, this command carries username/password
// inline and BYPASSES the reader's single-root-session lock — verified on
// hardware while the web UI held the session. That property matters for an
// alarm: firing must not fail because someone left a browser open on the reader.
//
// WIRING NOTE: the GPO is a POLARIZED optically isolated switch. Current must
// enter GPO(+) and exit GPO(-) (for GPO1: pin 4 in, pin 14 out). Wired backwards,
// an internal body diode is forward-biased and conducts continuously — regardless
// of the commanded state, and even with the reader powered off. A continuity test
// will not reveal it, because the meter's test voltage sits below the diode's
// forward threshold.
func (c *Client) DirectIOOutput(ctx context.Context, port int, on bool) error {
	if port < 1 || port > 4 {
		return fmt.Errorf("cs463: gpo port %d out of range [1, 4]", port)
	}
	logic := "0"
	if on {
		logic = "1"
	}
	params := url.Values{
		"command":    {"directIOOutput"},
		"mode":       {"run"},
		"port":       {strconv.Itoa(port)},
		"oper_logic": {logic},
		"username":   {c.user},
		"password":   {c.pass},
	}
	var ack xmlAck
	if err := c.do(ctx, params, &ack); err != nil {
		return err
	}
	if ack.Error != nil {
		return fmt.Errorf("cs463: directIOOutput port %d: %s", port, ack.Error.Msg)
	}
	if !strings.HasPrefix(ack.Ack, "OK") {
		return fmt.Errorf("cs463: directIOOutput port %d not acked: %q", port, ack.Ack)
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
