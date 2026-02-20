package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/benjaminbenny/internal-transfers-system/internal/service"
)

// AccountsHandler handles account-related endpoints.
type AccountsHandler struct {
	svc    *service.Service
	logger *slog.Logger
}

// NewAccountsHandler creates a new accounts handler.
func NewAccountsHandler(svc *service.Service, logger *slog.Logger) *AccountsHandler {
	return &AccountsHandler{
		svc:    svc,
		logger: logger,
	}
}

type createAccountRequest struct {
	AccountID      int64  `json:"account_id"`
	InitialBalance string `json:"initial_balance"`
}

// HandleCreateAccount creates a new account.
func (h *AccountsHandler) HandleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInvalidJSON(w)
		return
	}

	if err := h.svc.CreateAccount(r.Context(), req.AccountID, req.InitialBalance); err != nil {
		status, code := mapError(err)
		writeError(w, status, code)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

type getAccountResponse struct {
	AccountID int64  `json:"account_id"`
	Balance   string `json:"balance"`
}

// HandleGetAccount retrieves an account by ID.
func (h *AccountsHandler) HandleGetAccount(w http.ResponseWriter, r *http.Request) {
	accountIDStr := chi.URLParam(r, "account_id")
	if accountIDStr == "" {
		writeError(w, http.StatusBadRequest, "invalid_account_id")
		return
	}

	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil || accountID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_account_id")
		return
	}

	account, err := h.svc.GetAccount(r.Context(), accountID)
	if err != nil {
		status, code := mapError(err)
		writeError(w, status, code)
		return
	}

	response := getAccountResponse{
		AccountID: account.AccountID,
		Balance:   account.Balance,
	}

	writeJSON(w, http.StatusOK, response)
}
