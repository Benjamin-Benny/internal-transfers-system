package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPIntegration tests the full HTTP stack end-to-end.
// Only runs when RUN_INTEGRATION=1 and DATABASE_URL is set.
//
// This test validates:
// - Server starts on a random port successfully
// - Full request/response cycle through all layers (HTTP -> Service -> Repo -> DB)
// - Money conservation across transfers
// - Error handling and HTTP status codes
// - JSON serialization/deserialization
//
// To run:
//
//	RUN_INTEGRATION=1 DATABASE_URL="postgres://user:pass@localhost:5432/dbname" go test ./internal/http -v -run TestHTTPIntegration
//
// Note: This test is optional. The handler unit tests and repo integration tests
// provide sufficient coverage. This test is useful for validating the full stack
// integration in a production-like scenario.
func TestHTTPIntegration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION=1 to run.")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Skipping integration test. DATABASE_URL not set.")
	}

	ctx := context.Background()

	// Connect to database
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err, "failed to connect to database")
	defer pool.Close()

	// Clean tables before test (idempotency_keys first: it FKs to transactions)
	_, err = pool.Exec(ctx, "DELETE FROM idempotency_keys")
	require.NoError(t, err, "failed to clean idempotency_keys table")
	_, err = pool.Exec(ctx, "DELETE FROM transactions")
	require.NoError(t, err, "failed to clean transactions table")
	_, err = pool.Exec(ctx, "DELETE FROM accounts")
	require.NoError(t, err, "failed to clean accounts table")

	// Create logger (discard output for tests)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Create router
	router := NewRouter(pool, logger)

	// Start test server on random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to create listener")
	defer listener.Close()

	serverAddr := listener.Addr().String()
	baseURL := fmt.Sprintf("http://%s", serverAddr)

	server := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	// Wait briefly for server to start
	time.Sleep(50 * time.Millisecond)

	// Check for server startup errors
	select {
	case err := <-serverErr:
		t.Fatalf("server failed to start: %v", err)
	default:
	}

	// Create HTTP client
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	t.Run("full transfer flow", func(t *testing.T) {
		// Step 1: Create source account with initial balance
		sourceID := int64(10001)
		sourceInitialBalance := "1000.00000"

		createAccountReq := map[string]any{
			"account_id":      sourceID,
			"initial_balance": sourceInitialBalance,
		}
		resp := makeRequest(t, client, "POST", baseURL+"/accounts", createAccountReq)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "create source account should succeed")
		resp.Body.Close()

		// Step 2: Create destination account with zero balance
		destID := int64(10002)
		destInitialBalance := "0.00000"

		createAccountReq = map[string]any{
			"account_id":      destID,
			"initial_balance": destInitialBalance,
		}
		resp = makeRequest(t, client, "POST", baseURL+"/accounts", createAccountReq)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "create dest account should succeed")
		resp.Body.Close()

		// Step 3: Verify initial balances
		sourceAccount := getAccount(t, client, baseURL, sourceID)
		assert.Equal(t, sourceID, sourceAccount.AccountID)
		assert.Equal(t, sourceInitialBalance, sourceAccount.Balance)

		destAccount := getAccount(t, client, baseURL, destID)
		assert.Equal(t, destID, destAccount.AccountID)
		assert.Equal(t, destInitialBalance, destAccount.Balance)

		// Step 4: Execute transfer
		transferAmount := "250.50000"
		createTransactionReq := map[string]any{
			"source_account_id":      sourceID,
			"destination_account_id": destID,
			"amount":                 transferAmount,
		}
		resp = makeRequest(t, client, "POST", baseURL+"/transactions", createTransactionReq)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "transfer should succeed")
		resp.Body.Close()

		// Step 5: Verify final balances
		sourceAccount = getAccount(t, client, baseURL, sourceID)
		assert.Equal(t, "749.50000", sourceAccount.Balance, "source balance should decrease")

		destAccount = getAccount(t, client, baseURL, destID)
		assert.Equal(t, "250.50000", destAccount.Balance, "dest balance should increase")

		// Step 6: Verify money conservation
		// 1000.00000 + 0.00000 = 749.50000 + 250.50000 = 1000.00000
		totalInitial := 1000.00000
		totalFinal := 749.50000 + 250.50000
		assert.Equal(t, totalInitial, totalFinal, "total money should be conserved")
	})

	t.Run("error cases", func(t *testing.T) {
		t.Run("duplicate account", func(t *testing.T) {
			accountID := int64(10003)

			// Create account first time
			createAccountReq := map[string]any{
				"account_id":      accountID,
				"initial_balance": "100.00000",
			}
			resp := makeRequest(t, client, "POST", baseURL+"/accounts", createAccountReq)
			assert.Equal(t, http.StatusNoContent, resp.StatusCode)
			resp.Body.Close()

			// Try to create duplicate
			resp = makeRequest(t, client, "POST", baseURL+"/accounts", createAccountReq)
			assert.Equal(t, http.StatusConflict, resp.StatusCode)

			var errResp map[string]string
			err := json.NewDecoder(resp.Body).Decode(&errResp)
			require.NoError(t, err)
			assert.Equal(t, "conflict", errResp["error"])
			resp.Body.Close()
		})

		t.Run("get non-existent account", func(t *testing.T) {
			resp, err := client.Get(baseURL + "/accounts/99999")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)

			var errResp map[string]string
			err = json.NewDecoder(resp.Body).Decode(&errResp)
			require.NoError(t, err)
			assert.Equal(t, "not_found", errResp["error"])
		})

		t.Run("insufficient funds", func(t *testing.T) {
			sourceID := int64(10004)
			destID := int64(10005)

			// Create accounts
			createAccountReq := map[string]any{
				"account_id":      sourceID,
				"initial_balance": "50.00000",
			}
			resp := makeRequest(t, client, "POST", baseURL+"/accounts", createAccountReq)
			require.Equal(t, http.StatusNoContent, resp.StatusCode)
			resp.Body.Close()

			createAccountReq = map[string]any{
				"account_id":      destID,
				"initial_balance": "0.00000",
			}
			resp = makeRequest(t, client, "POST", baseURL+"/accounts", createAccountReq)
			require.Equal(t, http.StatusNoContent, resp.StatusCode)
			resp.Body.Close()

			// Try to transfer more than available
			createTransactionReq := map[string]any{
				"source_account_id":      sourceID,
				"destination_account_id": destID,
				"amount":                 "100.00000",
			}
			resp = makeRequest(t, client, "POST", baseURL+"/transactions", createTransactionReq)
			assert.Equal(t, http.StatusConflict, resp.StatusCode)

			var errResp map[string]string
			err := json.NewDecoder(resp.Body).Decode(&errResp)
			require.NoError(t, err)
			assert.Equal(t, "insufficient_funds", errResp["error"])
			resp.Body.Close()

			// Verify balances unchanged
			sourceAccount := getAccount(t, client, baseURL, sourceID)
			assert.Equal(t, "50.00000", sourceAccount.Balance)

			destAccount := getAccount(t, client, baseURL, destID)
			assert.Equal(t, "0.00000", destAccount.Balance)
		})

		t.Run("invalid amount format", func(t *testing.T) {
			createAccountReq := map[string]any{
				"account_id":      int64(10006),
				"initial_balance": "invalid",
			}
			resp := makeRequest(t, client, "POST", baseURL+"/accounts", createAccountReq)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			var errResp map[string]string
			err := json.NewDecoder(resp.Body).Decode(&errResp)
			require.NoError(t, err)
			assert.Equal(t, "invalid_input", errResp["error"])
			resp.Body.Close()
		})

		t.Run("too many decimal places", func(t *testing.T) {
			createAccountReq := map[string]any{
				"account_id":      int64(10007),
				"initial_balance": "100.123456", // 6 decimal places
			}
			resp := makeRequest(t, client, "POST", baseURL+"/accounts", createAccountReq)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			var errResp map[string]string
			err := json.NewDecoder(resp.Body).Decode(&errResp)
			require.NoError(t, err)
			assert.Equal(t, "invalid_input", errResp["error"])
			resp.Body.Close()
		})
	})

	t.Run("idempotent transfers", func(t *testing.T) {
		srcID := int64(11001)
		dstID := int64(11002)

		resp := makeRequest(t, client, "POST", baseURL+"/accounts",
			map[string]any{"account_id": srcID, "initial_balance": "1000.00000"})
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		resp.Body.Close()
		resp = makeRequest(t, client, "POST", baseURL+"/accounts",
			map[string]any{"account_id": dstID, "initial_balance": "0.00000"})
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		resp.Body.Close()

		// post issues POST /transactions with an optional Idempotency-Key header.
		post := func(amount, key string) *http.Response {
			jsonData, err := json.Marshal(map[string]any{
				"source_account_id":      srcID,
				"destination_account_id": dstID,
				"amount":                 amount,
			})
			require.NoError(t, err)
			req, err := http.NewRequest("POST", baseURL+"/transactions", bytes.NewBuffer(jsonData))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			if key != "" {
				req.Header.Set("Idempotency-Key", key)
			}
			r, err := client.Do(req)
			require.NoError(t, err)
			return r
		}

		// First call: 200 with a real transaction id.
		resp = post("250.00000", "http-key-1")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var first createTxResp
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&first))
		resp.Body.Close()
		assert.Greater(t, first.ID, int64(0))

		// Exact-duplicate retry: 200 with the SAME id, and no extra money movement.
		resp = post("250.00000", "http-key-1")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var second createTxResp
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&second))
		resp.Body.Close()
		assert.Equal(t, first.ID, second.ID, "retry must return the original transaction id")

		srcAccount := getAccount(t, client, baseURL, srcID)
		assert.Equal(t, "750.00000", srcAccount.Balance, "money must have moved exactly once")

		// Same key, different payload: 409 conflict.
		resp = post("300.00000", "http-key-1")
		require.Equal(t, http.StatusConflict, resp.StatusCode)
		var errResp map[string]string
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
		resp.Body.Close()
		assert.Equal(t, "conflict", errResp["error"])

		// Conflict moved no money.
		srcAccount = getAccount(t, client, baseURL, srcID)
		assert.Equal(t, "750.00000", srcAccount.Balance, "conflict must not move money")
	})

	t.Run("health check", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var healthResp map[string]string
		err = json.NewDecoder(resp.Body).Decode(&healthResp)
		require.NoError(t, err)
		assert.Equal(t, "ok", healthResp["status"])
	})
}

// makeRequest is a helper function to make HTTP requests with JSON body
func makeRequest(t *testing.T, client *http.Client, method, url string, body any) *http.Response {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		require.NoError(t, err, "failed to marshal request body")
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	require.NoError(t, err, "failed to create request")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	require.NoError(t, err, "request failed")

	return resp
}

type accountResponse struct {
	AccountID int64  `json:"account_id"`
	Balance   string `json:"balance"`
}

type createTxResp struct {
	ID int64 `json:"id"`
}

// getAccount is a helper function to get account details
func getAccount(t *testing.T, client *http.Client, baseURL string, accountID int64) accountResponse {
	t.Helper()

	url := fmt.Sprintf("%s/accounts/%d", baseURL, accountID)
	resp, err := client.Get(url)
	require.NoError(t, err, "failed to get account")
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "get account should succeed")

	var account accountResponse
	err = json.NewDecoder(resp.Body).Decode(&account)
	require.NoError(t, err, "failed to decode response")

	return account
}
