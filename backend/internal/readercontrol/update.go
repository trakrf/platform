package readercontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/trakrf/platform/backend/internal/readerrpc"
)

// UpdateDaemon asks the on-reader daemon to install a signed release. The reply
// arrives once the daemon has verified and accepted the request; the download,
// swap and restart follow it, so callers watch GetCapabilities().Daemon for the
// outcome. A refusal (bad signature, update in flight, no rollback guard, ...)
// is a *readerrpc.UpdateRefusedError.
func (c *Client) UpdateDaemon(ctx context.Context, base string, p readerrpc.DaemonUpdateParams) (readerrpc.DaemonUpdateResult, error) {
	resp, err := c.call(ctx, base, readerrpc.MethodDaemonUpdate, p)
	if err != nil {
		return readerrpc.DaemonUpdateResult{}, err
	}
	if resp.Error != nil {
		if resp.Error.Code == readerrpc.CodeUpdateRefused {
			return readerrpc.DaemonUpdateResult{}, &readerrpc.UpdateRefusedError{
				Reason: strings.TrimPrefix(resp.Error.Message, "update refused: "),
			}
		}
		return readerrpc.DaemonUpdateResult{}, rpcErr(readerrpc.MethodDaemonUpdate, resp.Error)
	}
	var res readerrpc.DaemonUpdateResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return readerrpc.DaemonUpdateResult{}, fmt.Errorf("readercontrol: decode update result: %w", err)
	}
	return res, nil
}
