// Package readerd hosts the on-reader daemon's reader-agnostic adapter layer.
//
// An Adapter translates the vendor-neutral readerrpc contract into one reader
// family's native operations. The daemon's RPC dispatch depends only on this
// interface, so future readers (e.g. Impinj R420/R700) plug in by implementing
// it without touching the dispatch or contract packages.
package readerd

import (
	"context"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// Adapter is the reader-agnostic control surface the daemon drives. Each reader
// family provides one implementation (cs463.Adapter is the first).
type Adapter interface {
	GetCapabilities(ctx context.Context) (readerrpc.Capabilities, error)
	GetOperProfile(ctx context.Context, force bool) (readerrpc.ReaderConfig, error)
	SetOperProfile(ctx context.Context, cfg readerrpc.ReaderConfig, force bool) (readerrpc.SetConfigResult, error)
	GetStatus(ctx context.Context) (readerrpc.Status, error)

	// GpoSet drives one general purpose output. Every ON is a one-shot: drive
	// on now, release after pulseMs (the daemon default when 0, never more than
	// readerrpc.GpoMaxOn), without a second request. Implementations own that
	// timer so the OFF edge does not depend on a follow-up message arriving.
	GpoSet(ctx context.Context, port int, on bool, pulseMs int) error

	// OutputsOff is the outputs' safe state: cancel every pending release and
	// drive every output off, logging each one's result. The daemon calls it
	// when it starts and when it stops (final), because a release timer dies
	// with the process and a halted reader holds its last output state.
	// After a final call every ON is refused.
	OutputsOff(ctx context.Context, final bool) error
}
