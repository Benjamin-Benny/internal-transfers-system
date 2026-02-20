package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/benjaminbenny/internal-transfers-system/internal/service"
)

// serviceInterface defines the interface that handlers depend on
type serviceInterface interface {
	CreateAccount(ctx context.Context, accountID int64, initialBalance string) error
	GetAccount(ctx context.Context, accountID int64) (service.Account, error)
	CreateTransaction(ctx context.Context, sourceID, destID int64, amount string) error
}

// mockService is a mock implementation of the service interface for testing
type mockService struct {
	createAccountFunc     func(ctx context.Context, accountID int64, initialBalance string) error
	getAccountFunc        func(ctx context.Context, accountID int64) (service.Account, error)
	createTransactionFunc func(ctx context.Context, sourceID, destID int64, amount string) error
}

func (m *mockService) CreateAccount(ctx context.Context, accountID int64, initialBalance string) error {
	if m.createAccountFunc != nil {
		return m.createAccountFunc(ctx, accountID, initialBalance)
	}
	return nil
}

func (m *mockService) GetAccount(ctx context.Context, accountID int64) (service.Account, error) {
	if m.getAccountFunc != nil {
		return m.getAccountFunc(ctx, accountID)
	}
	return service.Account{AccountID: accountID, Balance: "100.00000"}, nil
}

func (m *mockService) CreateTransaction(ctx context.Context, sourceID, destID int64, amount string) error {
	if m.createTransactionFunc != nil {
		return m.createTransactionFunc(ctx, sourceID, destID, amount)
	}
	return nil
}

// testAccountsHandler is a version of AccountsHandler that accepts an interface for testing
type testAccountsHandler struct {
	svc    serviceInterface
	logger *slog.Logger
}

func (h *testAccountsHandler) HandleCreateAccount(w http.ResponseWriter, r *http.Request) {
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

func (h *testAccountsHandler) HandleGetAccount(w http.ResponseWriter, r *http.Request) {
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

// testTransactionsHandler is a version of TransactionsHandler that accepts an interface for testing
type testTransactionsHandler struct {
	svc    serviceInterface
	logger *slog.Logger
}

func (h *testTransactionsHandler) HandleCreateTransaction(w http.ResponseWriter, r *http.Request) {
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

func TestHealthHandler_HandleHealth(t *testing.T) {
	handler := NewHealthHandler()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", response["status"])
	}
}

func TestAccountsHandler_HandleCreateAccount_Success(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mock := &mockService{}
	handler := &testAccountsHandler{svc: mock, logger: logger}

	body := `{"account_id": 123, "initial_balance": "100.50000"}`
	req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.HandleCreateAccount(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rec.Code)
	}

	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for 204, got %s", rec.Body.String())
	}
}

func TestAccountsHandler_HandleCreateAccount_InvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mock := &mockService{}
	handler := &testAccountsHandler{svc: mock, logger: logger}

	body := `{"account_id": "invalid"`
	req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.HandleCreateAccount(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["error"] != "invalid_json" {
		t.Errorf("expected error 'invalid_json', got '%s'", response["error"])
	}
}

func TestAccountsHandler_HandleCreateAccount_ServiceError_Conflict(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mock := &mockService{
		createAccountFunc: func(ctx context.Context, accountID int64, initialBalance string) error {
			return service.ErrConflict
		},
	}
	handler := &testAccountsHandler{svc: mock, logger: logger}

	body := `{"account_id": 123, "initial_balance": "100.50000"}`
	req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.HandleCreateAccount(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["error"] != "conflict" {
		t.Errorf("expected error 'conflict', got '%s'", response["error"])
	}
}

func TestAccountsHandler_HandleGetAccount_Success(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mock := &mockService{
		getAccountFunc: func(ctx context.Context, accountID int64) (service.Account, error) {
			return service.Account{
				AccountID: 123,
				Balance:   "250.12345",
			}, nil
		},
	}
	handler := &testAccountsHandler{svc: mock, logger: logger}

	req := httptest.NewRequest(http.MethodGet, "/accounts/123", nil)

	// Set up chi URL params
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("account_id", "123")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()

	handler.HandleGetAccount(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var response getAccountResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.AccountID != 123 {
		t.Errorf("expected account_id 123, got %d", response.AccountID)
	}

	if response.Balance != "250.12345" {
		t.Errorf("expected balance '250.12345', got '%s'", response.Balance)
	}
}

func TestAccountsHandler_HandleGetAccount_InvalidAccountID(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mock := &mockService{}
	handler := &testAccountsHandler{svc: mock, logger: logger}

	testCases := []struct {
		name      string
		accountID string
	}{
		{"non-numeric", "abc"},
		{"negative", "-1"},
		{"zero", "0"},
		{"empty", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/accounts/"+tc.accountID, nil)

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("account_id", tc.accountID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			rec := httptest.NewRecorder()

			handler.HandleGetAccount(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rec.Code)
			}

			var response map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if response["error"] != "invalid_account_id" {
				t.Errorf("expected error 'invalid_account_id', got '%s'", response["error"])
			}
		})
	}
}

func TestAccountsHandler_HandleGetAccount_NotFound(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mock := &mockService{
		getAccountFunc: func(ctx context.Context, accountID int64) (service.Account, error) {
			return service.Account{}, service.ErrNotFound
		},
	}
	handler := &testAccountsHandler{svc: mock, logger: logger}

	req := httptest.NewRequest(http.MethodGet, "/accounts/999", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("account_id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()

	handler.HandleGetAccount(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["error"] != "not_found" {
		t.Errorf("expected error 'not_found', got '%s'", response["error"])
	}
}

func TestTransactionsHandler_HandleCreateTransaction_Success(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mock := &mockService{}
	handler := &testTransactionsHandler{svc: mock, logger: logger}

	body := `{"source_account_id": 123, "destination_account_id": 456, "amount": "100.12345"}`
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.HandleCreateTransaction(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rec.Code)
	}

	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for 204, got %s", rec.Body.String())
	}
}

func TestTransactionsHandler_HandleCreateTransaction_InvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mock := &mockService{}
	handler := &testTransactionsHandler{svc: mock, logger: logger}

	body := `{"source_account_id": invalid}`
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.HandleCreateTransaction(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["error"] != "invalid_json" {
		t.Errorf("expected error 'invalid_json', got '%s'", response["error"])
	}
}

func TestTransactionsHandler_HandleCreateTransaction_InsufficientFunds(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mock := &mockService{
		createTransactionFunc: func(ctx context.Context, sourceID, destID int64, amount string) error {
			return service.ErrInsufficientFunds
		},
	}
	handler := &testTransactionsHandler{svc: mock, logger: logger}

	body := `{"source_account_id": 123, "destination_account_id": 456, "amount": "1000.00000"}`
	req := httptest.NewRequest(http.MethodPost, "/transactions", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.HandleCreateTransaction(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["error"] != "insufficient_funds" {
		t.Errorf("expected error 'insufficient_funds', got '%s'", response["error"])
	}
}

func TestAccountsHandler_HandleCreateAccount_InvalidInput(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	
	testCases := []struct {
		name string
		body string
	}{
		{
			name: "negative account ID",
			body: `{"account_id": -1, "initial_balance": "100.00000"}`,
		},
		{
			name: "too many decimals",
			body: `{"account_id": 123, "initial_balance": "100.123456"}`,
		},
		{
			name: "zero account ID",
			body: `{"account_id": 0, "initial_balance": "100.00000"}`,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockService{
				createAccountFunc: func(ctx context.Context, accountID int64, initialBalance string) error {
					return service.ErrInvalidInput
				},
			}
			handler := &testAccountsHandler{svc: mock, logger: logger}
			
			req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()
			
			handler.HandleCreateAccount(rec, req)
			
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rec.Code)
			}
			
			var response map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			
			if response["error"] != "invalid_input" {
				t.Errorf("expected error 'invalid_input', got '%s'", response["error"])
			}
		})
	}
}

func TestMapError(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "ErrInvalidInput",
			err:            service.ErrInvalidInput,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "invalid_input",
		},
		{
			name:           "ErrNotFound",
			err:            service.ErrNotFound,
			expectedStatus: http.StatusNotFound,
			expectedCode:   "not_found",
		},
		{
			name:           "ErrConflict",
			err:            service.ErrConflict,
			expectedStatus: http.StatusConflict,
			expectedCode:   "conflict",
		},
		{
			name:           "ErrInsufficientFunds",
			err:            service.ErrInsufficientFunds,
			expectedStatus: http.StatusConflict,
			expectedCode:   "insufficient_funds",
		},
		{
			name:           "UnknownError",
			err:            context.DeadlineExceeded,
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "internal_error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status, code := mapError(tc.err)

			if status != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, status)
			}

			if code != tc.expectedCode {
				t.Errorf("expected code '%s', got '%s'", tc.expectedCode, code)
			}
		})
	}
}
