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
	Close()
}

// Storage handles all database operations using a connection pool.
type Storage struct {
	pool PgxPool
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

// Close gracefully closes the database connection pool and releases all resources.
func (s *Storage) Close() {
	if s.pool != nil {
		s.pool.Close()
		slog.Info("Database connection pool closed")
	}
}

// Pool returns the underlying connection pool for advanced use cases
// that require direct pool access.
func (s *Storage) Pool() PgxPool {
	return s.pool
}
