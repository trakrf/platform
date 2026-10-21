package outbox

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// Runtime owns the configured River client and its worker registration. A
// notification.Runtime-style lifecycle: Start begins polling/claiming,
// Stop drains in-flight work. Construction never claims or sends anything.
//
// riverpgxv5.Driver implements riverdriver.Driver[pgx.Tx] directly (confirmed
// against vendored source, riverpgxv5/river_pgx_v5_driver_test.go:25) — no
// wrapper transaction type, so this is plain pgx.Tx throughout.
type Runtime struct {
	Client *river.Client[pgx.Tx]
}

// NewRuntime builds the River client against pool (must be the concrete
// *pgxpool.Pool backing Storage — Storage's own PgxPool interface exists for
// pgxmock compatibility and is not sufficient here, since riverpgxv5.New
// requires the concrete pgxpool type for its own maintenance queries).
func NewRuntime(pool *pgxpool.Pool, adapter ChannelAdapter, store deliveryWorkerStore, metrics *Metrics, register ...func(*river.Workers)) (*Runtime, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, NewDeliveryWorker(store, adapter, metrics))
	for _, add := range register {
		add(workers)
	}

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers:     workers,
		RetryPolicy: NewRetryLadder(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to construct river client: %w", err)
	}

	return &Runtime{Client: client}, nil
}

// NewInserter builds an insert-only River client: no queues and no workers,
// so it never claims a job. With no worker bundle River does not check that an
// inserted kind has a worker, so it can queue any job kind.
func NewInserter(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to construct river inserter: %w", err)
	}
	return client, nil
}

func (r *Runtime) Start(ctx context.Context) error {
	if r == nil || r.Client == nil {
		return nil
	}
	return r.Client.Start(ctx)
}

func (r *Runtime) Stop(ctx context.Context) error {
	if r == nil || r.Client == nil {
		return nil
	}
	return r.Client.Stop(ctx)
}
