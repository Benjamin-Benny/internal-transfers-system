package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/benjaminbenny/internal-transfers-system/internal/config"
	"github.com/benjaminbenny/internal-transfers-system/internal/db"
	httpserver "github.com/benjaminbenny/internal-transfers-system/internal/http"
)

func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load configuration from environment variables
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("configuration loaded",
		slog.String("port", cfg.Port),
		slog.String("env", cfg.Env),
	)

	// Create context that cancels on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize database connection pool
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	logger.Info("database connection established")

	// Create HTTP server
	server := httpserver.NewServer(cfg, pool, logger)

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting HTTP server", slog.String("port", cfg.Port))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for interrupt signal or server error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		logger.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("shutting down server...")
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("error during server shutdown", slog.String("error", err.Error()))
	}

	logger.Info("server stopped gracefully")
}
