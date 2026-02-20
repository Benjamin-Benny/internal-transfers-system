package db

import (
	"context"
	"testing"
	"time"
)

func TestNewPool_InvalidURL(t *testing.T) {
	ctx := context.Background()
	
	// Test with invalid database URL
	_, err := NewPool(ctx, "not-a-valid-url")
	if err == nil {
		t.Fatal("NewPool should return error for invalid URL")
	}
}

func TestNewPool_WithTimeout(t *testing.T) {
	// Skip this test in CI/CD or when database is not available
	t.Skip("Skipping integration test - requires running PostgreSQL")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// This would connect to a real database if available
	databaseURL := "postgres://app:app@localhost:5432/transfers?sslmode=disable"
	pool, err := NewPool(ctx, databaseURL)
	if err != nil {
		t.Logf("Could not connect to database (this is expected if DB is not running): %v", err)
		return
	}
	defer pool.Close()
	
	// Verify the pool is working
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}
	
	stats := pool.Stat()
	t.Logf("Pool stats - Total connections: %d, Idle connections: %d", 
		stats.TotalConns(), stats.IdleConns())
}
