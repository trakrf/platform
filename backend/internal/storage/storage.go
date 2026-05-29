package storage

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxPool is an interface that both *pgxpool.Pool and pgxmock implement
type PgxPool interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	Close()
}

// Storage handles all database operations using a connection pool.
type Storage struct {
	pool PgxPool
	// accessorPool, when non-nil, is what Pool() returns instead of pool.
	// It exists only for the integration test harness (TRA-874): storage
	// methods run on pool (a non-superuser, RLS-enforced role) while Pool()
	// hands tests a superuser pool for fixture setup and cleanup. It is
	// always nil in production, so Pool() returns the real pool there.
	accessorPool PgxPool
}

// New creates a new Storage instance with an initialized connection pool.
// It reads the PG_URL environment variable and configures the pool with
// production-ready settings for max connections, lifetime, and health checks.
func New(ctx context.Context) (*Storage, error) {
	pgURL := os.Getenv("PG_URL")
	if pgURL == "" {
		return nil, fmt.Errorf("PG_URL environment variable not set")
	}

	config, err := pgxpool.ParseConfig(pgURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PG_URL: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	slog.Info("Database connection pool initialized",
		"max_conns", config.MaxConns,
		"min_conns", config.MinConns)

	return &Storage{pool: pool}, nil
}

// NewWithPool creates a Storage instance with an existing pool.
// This is primarily used for testing with mock or test database pools.
func NewWithPool(pool PgxPool) *Storage {
	return &Storage{pool: pool}
}

// NewForTest creates a Storage instance for the integration test harness
// (TRA-874). queryPool is the non-superuser, RLS-enforced pool that every
// storage method and WithOrgTx executes against — so a method that forgets to
// set org context fails the test the way it would fail in production.
// accessorPool is the superuser pool returned by Pool(), used by tests as the
// privileged escape hatch for cross-org fixture setup and TRUNCATE cleanup.
func NewForTest(queryPool, accessorPool PgxPool) *Storage {
	return &Storage{pool: queryPool, accessorPool: accessorPool}
}

// Close gracefully closes the database connection pool and releases all resources.
func (s *Storage) Close() {
	if s.pool != nil {
		s.pool.Close()
		slog.Info("Database connection pool closed")
	}
}

// Pool returns the underlying connection pool for advanced use cases
// that require direct pool access. When a separate accessor pool has been
// configured (integration test harness, TRA-874), it is returned instead so
// tests get the superuser pool for fixture setup while storage methods keep
// running on the RLS-enforced pool.
func (s *Storage) Pool() PgxPool {
	if s.accessorPool != nil {
		return s.accessorPool
	}
	return s.pool
}
