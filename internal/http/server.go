package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/benjaminbenny/internal-transfers-system/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewServer creates and configures a new HTTP server with appropriate timeouts.
func NewServer(cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) *http.Server {
	router := NewRouter(pool, logger)

	return &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
