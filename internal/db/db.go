package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a new PostgreSQL connection pool with reasonable defaults.
// It pings the database to ensure connectivity before returning.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	// Parse the connection string and configure the pool
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Set reasonable pool defaults
	config.MaxConns = 25                          // Maximum number of connections
	config.MinConns = 5                           // Minimum number of connections
	config.MaxConnLifetime = time.Hour            // Max lifetime of a connection
	config.MaxConnIdleTime = 30 * time.Minute     // Max idle time before closing
	config.HealthCheckPeriod = time.Minute        // Periodic health check interval
	config.ConnConfig.ConnectTimeout = 5 * time.Second // Connection timeout

	// Create the connection pool
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Ping the database to fail fast if connection is not available
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}
