package service

import (
	"context"
	"errors"
	"testing"

	"github.com/benjaminbenny/internal-transfers-system/internal/repo"
	"github.com/shopspring/decimal"
)

// mockAccountRepo is a simple mock implementation of AccountRepo
type mockAccountRepo struct {
	createAccountFunc func(ctx context.Context, accountID int64, initialBalance decimal.Decimal) error
	getAccountFunc    func(ctx context.Context, accountID int64) (repo.Account, error)
	transferTxFunc    func(ctx context.Context, sourceID, destID int64, amount decimal.Decimal) error
}

func (m *mockAccountRepo) CreateAccount(ctx context.Context, accountID int64, initialBalance decimal.Decimal) error {
	if m.createAccountFunc != nil {
		return m.createAccountFunc(ctx, accountID, initialBalance)
	}
	return nil
}

func (m *mockAccountRepo) GetAccount(ctx context.Context, accountID int64) (repo.Account, error) {
	if m.getAccountFunc != nil {
		return m.getAccountFunc(ctx, accountID)
	}
	return repo.Account{}, nil
}

func (m *mockAccountRepo) TransferTx(ctx context.Context, sourceID, destID int64, amount decimal.Decimal) error {
	if m.transferTxFunc != nil {
		return m.transferTxFunc(ctx, sourceID, destID, amount)
	}
	return nil
}

func TestCreateAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("valid account creation", func(t *testing.T) {
		mockRepo := &mockAccountRepo{
			createAccountFunc: func(ctx context.Context, accountID int64, initialBalance decimal.Decimal) error {
				if accountID != 123 {
					t.Errorf("expected accountID 123, got %d", accountID)
				}
				expected := decimal.NewFromFloat(100.50)
				if !initialBalance.Equal(expected) {
					t.Errorf("expected balance %s, got %s", expected, initialBalance)
				}
				return nil
			},
		}

		svc := New(mockRepo)
		err := svc.CreateAccount(ctx, 123, "100.50")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("validates accountID must be positive", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		testCases := []struct {
			name      string
			accountID int64
		}{
			{"zero accountID", 0},
			{"negative accountID", -1},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := svc.CreateAccount(ctx, tc.accountID, "100.00")
				if err == nil {
					t.Error("expected error for invalid accountID")
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("expected ErrInvalidInput, got %v", err)
				}
			})
		}
	})

	t.Run("validates balance format", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		testCases := []struct {
			name    string
			balance string
		}{
			{"invalid format", "not-a-number"},
			{"empty string", ""},
			{"special chars", "100.00$"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := svc.CreateAccount(ctx, 123, tc.balance)
				if err == nil {
					t.Error("expected error for invalid balance format")
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("expected ErrInvalidInput, got %v", err)
				}
			})
		}
	})

	t.Run("validates max 5 decimal places", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		testCases := []struct {
			name    string
			balance string
			valid   bool
		}{
			{"5 decimals - valid", "100.12345", true},
			{"6 decimals - invalid", "100.123456", false},
			{"7 decimals - invalid", "100.1234567", false},
			{"no decimals - valid", "100", true},
			{"2 decimals - valid", "100.50", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := svc.CreateAccount(ctx, 123, tc.balance)
				if tc.valid {
					if err != nil {
						t.Errorf("expected no error for valid balance, got %v", err)
					}
				} else {
					if err == nil {
						t.Error("expected error for balance with >5 decimal places")
					}
					if !errors.Is(err, ErrInvalidInput) {
						t.Errorf("expected ErrInvalidInput, got %v", err)
					}
				}
			})
		}
	})

	t.Run("validates non-negative balance", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		err := svc.CreateAccount(ctx, 123, "-10.50")
		if err == nil {
			t.Error("expected error for negative balance")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("maps ErrAccountAlreadyExists to ErrConflict", func(t *testing.T) {
		mockRepo := &mockAccountRepo{
			createAccountFunc: func(ctx context.Context, accountID int64, initialBalance decimal.Decimal) error {
				return repo.ErrAccountAlreadyExists
			},
		}

		svc := New(mockRepo)
		err := svc.CreateAccount(ctx, 123, "100.00")
		if err == nil {
			t.Error("expected error")
		}
		if !errors.Is(err, ErrConflict) {
			t.Errorf("expected ErrConflict, got %v", err)
		}
	})
}

func TestGetAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("validates accountID must be positive", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		testCases := []struct {
			name      string
			accountID int64
		}{
			{"zero accountID", 0},
			{"negative accountID", -1},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := svc.GetAccount(ctx, tc.accountID)
				if err == nil {
					t.Error("expected error for invalid accountID")
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("expected ErrInvalidInput, got %v", err)
				}
			})
		}
	})

	t.Run("maps ErrAccountNotFound to ErrNotFound", func(t *testing.T) {
		mockRepo := &mockAccountRepo{
			getAccountFunc: func(ctx context.Context, accountID int64) (repo.Account, error) {
				return repo.Account{}, repo.ErrAccountNotFound
			},
		}

		svc := New(mockRepo)
		_, err := svc.GetAccount(ctx, 123)
		if err == nil {
			t.Error("expected error")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("formats balance to exactly 5 decimal places", func(t *testing.T) {
		testCases := []struct {
			name            string
			repoBalance     string
			expectedBalance string
		}{
			{"2 decimals", "100.50", "100.50000"},
			{"0 decimals", "100", "100.00000"},
			{"5 decimals", "100.12345", "100.12345"},
			{"3 decimals", "100.123", "100.12300"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				mockRepo := &mockAccountRepo{
					getAccountFunc: func(ctx context.Context, accountID int64) (repo.Account, error) {
						balance, _ := decimal.NewFromString(tc.repoBalance)
						return repo.Account{
							AccountID: accountID,
							Balance:   balance,
						}, nil
					},
				}

				svc := New(mockRepo)
				account, err := svc.GetAccount(ctx, 123)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if account.AccountID != 123 {
					t.Errorf("expected accountID 123, got %d", account.AccountID)
				}
				if account.Balance != tc.expectedBalance {
					t.Errorf("expected balance %s, got %s", tc.expectedBalance, account.Balance)
				}
			})
		}
	})
}

func TestCreateTransaction(t *testing.T) {
	ctx := context.Background()

	t.Run("valid transaction", func(t *testing.T) {
		mockRepo := &mockAccountRepo{
			transferTxFunc: func(ctx context.Context, sourceID, destID int64, amount decimal.Decimal) error {
				if sourceID != 100 || destID != 200 {
					t.Errorf("expected sourceID 100 and destID 200, got %d and %d", sourceID, destID)
				}
				expected := decimal.NewFromFloat(50.25)
				if !amount.Equal(expected) {
					t.Errorf("expected amount %s, got %s", expected, amount)
				}
				return nil
			},
		}

		svc := New(mockRepo)
		err := svc.CreateTransaction(ctx, 100, 200, "50.25")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("validates sourceID must be positive", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		testCases := []struct {
			name     string
			sourceID int64
		}{
			{"zero sourceID", 0},
			{"negative sourceID", -1},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := svc.CreateTransaction(ctx, tc.sourceID, 200, "50.00")
				if err == nil {
					t.Error("expected error for invalid sourceID")
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("expected ErrInvalidInput, got %v", err)
				}
			})
		}
	})

	t.Run("validates destID must be positive", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		testCases := []struct {
			name   string
			destID int64
		}{
			{"zero destID", 0},
			{"negative destID", -1},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := svc.CreateTransaction(ctx, 100, tc.destID, "50.00")
				if err == nil {
					t.Error("expected error for invalid destID")
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("expected ErrInvalidInput, got %v", err)
				}
			})
		}
	})

	t.Run("validates sourceID and destID must be different", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		err := svc.CreateTransaction(ctx, 100, 100, "50.00")
		if err == nil {
			t.Error("expected error for same source and destination")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("validates amount format", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		testCases := []struct {
			name   string
			amount string
		}{
			{"invalid format", "not-a-number"},
			{"empty string", ""},
			{"special chars", "50.00$"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := svc.CreateTransaction(ctx, 100, 200, tc.amount)
				if err == nil {
					t.Error("expected error for invalid amount format")
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("expected ErrInvalidInput, got %v", err)
				}
			})
		}
	})

	t.Run("validates max 5 decimal places", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		testCases := []struct {
			name   string
			amount string
			valid  bool
		}{
			{"5 decimals - valid", "50.12345", true},
			{"6 decimals - invalid", "50.123456", false},
			{"7 decimals - invalid", "50.1234567", false},
			{"no decimals - valid", "50", true},
			{"2 decimals - valid", "50.25", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := svc.CreateTransaction(ctx, 100, 200, tc.amount)
				if tc.valid {
					if err != nil {
						t.Errorf("expected no error for valid amount, got %v", err)
					}
				} else {
					if err == nil {
						t.Error("expected error for amount with >5 decimal places")
					}
					if !errors.Is(err, ErrInvalidInput) {
						t.Errorf("expected ErrInvalidInput, got %v", err)
					}
				}
			})
		}
	})

	t.Run("validates amount must be positive", func(t *testing.T) {
		mockRepo := &mockAccountRepo{}
		svc := New(mockRepo)

		testCases := []struct {
			name   string
			amount string
		}{
			{"zero amount", "0"},
			{"zero with decimals", "0.00"},
			{"negative amount", "-50.00"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := svc.CreateTransaction(ctx, 100, 200, tc.amount)
				if err == nil {
					t.Error("expected error for non-positive amount")
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("expected ErrInvalidInput, got %v", err)
				}
			})
		}
	})

	t.Run("maps ErrInsufficientFunds to ErrInsufficientFunds", func(t *testing.T) {
		mockRepo := &mockAccountRepo{
			transferTxFunc: func(ctx context.Context, sourceID, destID int64, amount decimal.Decimal) error {
				return repo.ErrInsufficientFunds
			},
		}

		svc := New(mockRepo)
		err := svc.CreateTransaction(ctx, 100, 200, "50.00")
		if err == nil {
			t.Error("expected error")
		}
		if !errors.Is(err, ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}
	})

	t.Run("maps ErrAccountNotFound to ErrNotFound", func(t *testing.T) {
		mockRepo := &mockAccountRepo{
			transferTxFunc: func(ctx context.Context, sourceID, destID int64, amount decimal.Decimal) error {
				return repo.ErrAccountNotFound
			},
		}

		svc := New(mockRepo)
		err := svc.CreateTransaction(ctx, 100, 200, "50.00")
		if err == nil {
			t.Error("expected error")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("maps ErrSameAccount to ErrInvalidInput", func(t *testing.T) {
		mockRepo := &mockAccountRepo{
			transferTxFunc: func(ctx context.Context, sourceID, destID int64, amount decimal.Decimal) error {
				return repo.ErrSameAccount
			},
		}

		svc := New(mockRepo)
		err := svc.CreateTransaction(ctx, 100, 200, "50.00")
		if err == nil {
			t.Error("expected error")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("wraps unknown errors", func(t *testing.T) {
		unknownErr := errors.New("database connection lost")
		mockRepo := &mockAccountRepo{
			transferTxFunc: func(ctx context.Context, sourceID, destID int64, amount decimal.Decimal) error {
				return unknownErr
			},
		}

		svc := New(mockRepo)
		err := svc.CreateTransaction(ctx, 100, 200, "50.00")
		if err == nil {
			t.Error("expected error")
		}
		if !errors.Is(err, unknownErr) {
			t.Errorf("expected wrapped error to contain original error")
		}
	})
}
