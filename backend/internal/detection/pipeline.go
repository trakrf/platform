// Package detection runs trigger detectors inside the scan transaction, so the
// rows they write commit or roll back together with the scan.
package detection

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/storage"
)

var metricStageErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "detection_stage_errors_total",
	Help: "Detection stage failures that were contained without failing the scan, by stage.",
}, []string{"stage"})

// Pending holds the work a stage defers until the scan outcome is known.
// Either field may be nil.
type Pending struct {
	OnCommit   func(rowsKept bool) // scan committed; rowsKept is false if this stage failed
	OnRollback func()              // scan rolled back
}

// Stage is one detector. It writes on tx; the pipeline wraps it in a savepoint.
type Stage interface {
	Name() string
	Detect(ctx context.Context, tx pgx.Tx, in storage.DetectInput) (Pending, error)
}

// Pipeline runs its stages in order for each scan.
type Pipeline struct {
	log    *zerolog.Logger
	stages []Stage
}

func NewPipeline(log *zerolog.Logger, stages ...Stage) *Pipeline {
	return &Pipeline{log: log, stages: stages}
}

// Begin starts the per-scan state that carries each stage's Pending from
// Detect to Committed or RolledBack.
func (p *Pipeline) Begin() *Run { return &Run{p: p} }

type stageResult struct {
	pending  Pending
	rowsKept bool
}

// Run is one scan's pass through the pipeline.
type Run struct {
	p       *Pipeline
	results []stageResult
}

// Detect is a storage.DetectFunc. A stage error never fails the scan: that
// stage's savepoint rolls back and the next stage still runs.
func (r *Run) Detect(ctx context.Context, tx pgx.Tx, in storage.DetectInput) error {
	for _, st := range r.p.stages {
		sp, err := tx.Begin(ctx)
		if err != nil {
			r.fail(st, err, "detection savepoint begin failed")
			continue
		}
		pending, err := st.Detect(ctx, sp, in)
		if err == nil {
			if err = sp.Commit(ctx); err != nil {
				r.fail(st, err, "detection savepoint commit failed")
			}
		} else {
			r.fail(st, err, "detection stage failed")
		}
		kept := err == nil
		if !kept {
			if rbErr := sp.Rollback(ctx); rbErr != nil {
				r.p.log.Error().Err(rbErr).Str("stage", st.Name()).Msg("detection savepoint rollback failed")
			}
		}
		r.results = append(r.results, stageResult{pending: pending, rowsKept: kept})
	}
	return nil
}

func (r *Run) fail(st Stage, err error, msg string) {
	metricStageErrors.WithLabelValues(st.Name()).Inc()
	r.p.log.Error().Err(err).Str("stage", st.Name()).Msg(msg)
}

// Committed reports the scan committed to every stage that ran.
func (r *Run) Committed() {
	for _, res := range r.results {
		if res.pending.OnCommit != nil {
			res.pending.OnCommit(res.rowsKept)
		}
	}
}

// RolledBack reports the scan rolled back to every stage that ran.
func (r *Run) RolledBack() {
	for _, res := range r.results {
		if res.pending.OnRollback != nil {
			res.pending.OnRollback()
		}
	}
}
