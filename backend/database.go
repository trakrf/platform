package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var db *pgxpool.Pool

// initDB initializes the database connection pool
func initDB(ctx context.Context) error {
	pgURL := os.Getenv("PG_URL")
	if pgURL == "" {
		return fmt.Errorf("PG_URL environment variable not set")
	}

	config, err := pgxpool.ParseConfig(pgURL)
	if err != nil {
		return fmt.Errorf("failed to parse PG_URL: %w", err)
	}

	// Configure pool
	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	db = pool
	slog.Info("Database connection pool initialized",
		"max_conns", config.MaxConns,
		"min_conns", config.MinConns)

	return nil
}

// closeDB gracefully closes the database connection pool
func closeDB() {
	if db != nil {
		db.Close()
		slog.Info("Database connection pool closed")
	}
}
