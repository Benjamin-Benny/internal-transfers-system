package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/benjaminbenny/internal-transfers-system/internal/http/handlers"
	custommw "github.com/benjaminbenny/internal-transfers-system/internal/http/middleware"
	"github.com/benjaminbenny/internal-transfers-system/internal/repo"
	"github.com/benjaminbenny/internal-transfers-system/internal/service"
)

// NewRouter creates and configures the chi router with all routes and middleware.
func NewRouter(pool *pgxpool.Pool, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	// Add middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(custommw.RequestLogger(logger))

	// Initialize repository and service
	repository := repo.New(pool)
	svc := service.New(repository)

	// Initialize handlers
	healthHandler := handlers.NewHealthHandler()
	accountsHandler := handlers.NewAccountsHandler(svc, logger)
	transactionsHandler := handlers.NewTransactionsHandler(svc, logger)

	// Register routes
	r.Get("/health", healthHandler.HandleHealth)
	r.Post("/accounts", accountsHandler.HandleCreateAccount)
	r.Get("/accounts/{account_id}", accountsHandler.HandleGetAccount)
	r.Post("/transactions", transactionsHandler.HandleCreateTransaction)

	return r
}
