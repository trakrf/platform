// Package shelly is a transport-only driver for Shelly Gen2+ (NG) devices over
// local HTTP RPC. It drives a single relay channel via the Switch.Set method.
// See https://shelly-api-docs.shelly.cloud/gen2/ (TRA-903).
package shelly

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const defaultTimeout = 3 * time.Second

// Client issues Gen2+ RPC calls to Shelly devices.
type Client struct {
	http *http.Client
}

// New builds a Client with the given per-request timeout (defaults to 3s when
// <= 0). The short timeout keeps a dead device from stalling the fire path.
func New(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{http: &http.Client{Timeout: timeout}}
}

type rpcRequest struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

// Set drives one relay channel on/off via the Switch.Set RPC over local HTTP.
// When on and offAfterSec > 0 it sets toggle_after, the device's one-shot
// flip-back timer (seconds): the relay turns on now and the device flips it off
// after the delay with no further call. offAfterSec is ignored for off commands.
// Any transport error or non-2xx response is returned so callers can fail-quiet
// (log, do not crash). Comms failure leaves the relay in its own default state.
func (c *Client) Set(ctx context.Context, baseURL string, switchID int, on bool, offAfterSec int) error {
	params := map[string]any{"id": switchID, "on": on}
	if on && offAfterSec > 0 {
		params["toggle_after"] = offAfterSec
	}
	body, err := json.Marshal(rpcRequest{
		ID:     1,
		Method: "Switch.Set",
		Params: params,
	})
	if err != nil {
		return fmt.Errorf("shelly: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/rpc", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("shelly: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("shelly: request to %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("shelly: %s returned status %d", baseURL, resp.StatusCode)
	}
	return nil
}
