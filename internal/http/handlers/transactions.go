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

type createTransactionResponse struct {
	ID int64 `json:"id"`
}

// HandleCreateTransaction creates a new transaction.
//
// The optional Idempotency-Key header makes a retry safe: without it, behavior is unchanged
// (204 No Content); with it, the transfer is deduplicated by key and the resulting transaction
// id is returned (200) so a retry can be verified against the original.
func (h *TransactionsHandler) HandleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	var req createTransactionRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInvalidJSON(w)
		return
	}

	// No key => original contract, unchanged.
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		id, err := h.svc.CreateTransactionIdempotent(r.Context(), req.SourceAccountID, req.DestinationAccountID, req.Amount, key)
		if err != nil {
			status, code := mapError(err)
			writeError(w, status, code)
			return
		}
		writeJSON(w, http.StatusOK, createTransactionResponse{ID: id})
		return
	}

	if err := h.svc.CreateTransaction(r.Context(), req.SourceAccountID, req.DestinationAccountID, req.Amount); err != nil {
		status, code := mapError(err)
		writeError(w, status, code)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}
