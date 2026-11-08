package cs463

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// probeCommand is the read the health probe sends. directIOInput authenticates
// inline, so it takes no session slot. It mutates nothing, and it goes through
// the same servlet path (HttpAPIHandler) that fails when the vendor app wedges.
const probeCommand = "directIOInput"

type xmlProbe struct {
	XMLName xml.Name `xml:"CSL"`
	Command string   `xml:"Command"`
	Input   *struct {
		PortList string `xml:"port_list,attr"`
	} `xml:"Input"`
	Error *xmlError `xml:"Error"`
}

// apiProber is the content-checking probe; the real *Client implements it.
type apiProber interface {
	ProbeAPI(ctx context.Context) (state, detail string)
}

var _ apiProber = (*Client)(nil)

// ProbeHealth reports the reader API's health (readerrpc.API*) for the daemon's
// health monitor. Ops that cannot probe report APIUnknown.
func (a *Adapter) ProbeHealth(ctx context.Context) (state, detail string) {
	p, ok := a.ops.(apiProber)
	if !ok {
		return readerrpc.APIUnknown, "reader ops cannot probe"
	}
	return p.ProbeAPI(ctx)
}

// ProbeAPI reports whether the reader's /API is actually processing requests,
// as one of the readerrpc.API* states, plus a short reason.
//
// It is judged on the parsed body, never on the status code: nginx in front of
// a wedged vendor app answers 200 with an empty body for every request. A
// well-formed <Error> reply (a wrong password, say) is healthy. The servlet is
// serving, and no restart can fix a credential.
func (c *Client) ProbeAPI(ctx context.Context) (state, detail string) {
	params := url.Values{"command": {probeCommand}, "username": {c.user}, "password": {c.pass}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/API?"+params.Encode(), nil)
	if err != nil {
		return readerrpc.APIUnreachable, "build request: " + withoutURL(err).Error()
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return readerrpc.APIUnreachable, withoutURL(err).Error()
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return readerrpc.APIDegraded, "read body: " + err.Error()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readerrpc.APIUnreachable, fmt.Sprintf("http status %d", resp.StatusCode)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return readerrpc.APIDegraded, fmt.Sprintf("http %d with an empty body", resp.StatusCode)
	}
	var p xmlProbe
	if err := xml.Unmarshal(body, &p); err != nil {
		return readerrpc.APIDegraded, fmt.Sprintf("unparseable reply (%d bytes)", len(body))
	}
	if p.Command != probeCommand {
		return readerrpc.APIDegraded, fmt.Sprintf("reply names command %q", p.Command)
	}
	switch {
	case p.Error != nil:
		return readerrpc.APIHealthy, fmt.Sprintf("/API answered error %s: %s", p.Error.Code, p.Error.Msg)
	case p.Input != nil:
		return readerrpc.APIHealthy, ""
	default:
		return readerrpc.APIDegraded, "reply has neither Input nor Error"
	}
}
