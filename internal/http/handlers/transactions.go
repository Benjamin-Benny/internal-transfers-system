package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/benjaminbenny/internal-transfers-system/internal/service"
)

// TransactionsHandler handles transaction-related endpoints.
type TransactionsHandler struct {
	svc    *service.Service
	logger *slog.Logger
}

// NewTransactionsHandler creates a new transactions handler.
func NewTransactionsHandler(svc *service.Service, logger *slog.Logger) *TransactionsHandler {
	return &TransactionsHandler{
		svc:    svc,
		logger: logger,
	}
}

type createTransactionRequest struct {
	SourceAccountID      int64  `json:"source_account_id"`
	DestinationAccountID int64  `json:"destination_account_id"`
	Amount               string `json:"amount"`
}

// HandleCreateTransaction creates a new transaction.
func (h *TransactionsHandler) HandleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	var req createTransactionRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInvalidJSON(w)
		return
	}

	if err := h.svc.CreateTransaction(r.Context(), req.SourceAccountID, req.DestinationAccountID, req.Amount); err != nil {
		status, code := mapError(err)
		writeError(w, status, code)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}
