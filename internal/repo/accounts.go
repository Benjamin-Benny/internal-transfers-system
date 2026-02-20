package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

func (r *Repo) CreateAccount(ctx context.Context, accountID int64, initialBalance decimal.Decimal) error {
	// Minimal validation
	if accountID <= 0 {
		return fmt.Errorf("%w: account ID must be positive", ErrInvalidAmount)
	}
	if initialBalance.IsNegative() {
		return fmt.Errorf("%w: initial balance cannot be negative", ErrInvalidAmount)
	}

	// Store balance as string with 5 decimal places for NUMERIC(20,5)
	balanceStr := initialBalance.StringFixed(5)

	query := `INSERT INTO accounts (account_id, balance) VALUES ($1, $2)`
	_, err := r.pool.Exec(ctx, query, accountID, balanceStr)
	if err != nil {
		// Check for unique constraint violation (duplicate account_id)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrAccountAlreadyExists
		}
		return fmt.Errorf("failed to create account: %w", err)
	}

	return nil
}

func (r *Repo) GetAccount(ctx context.Context, accountID int64) (Account, error) {
	query := `SELECT account_id, balance FROM accounts WHERE account_id = $1`

	var account Account
	var balanceStr string

	err := r.pool.QueryRow(ctx, query, accountID).Scan(&account.AccountID, &balanceStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, ErrAccountNotFound
		}
		return Account{}, fmt.Errorf("failed to get account: %w", err)
	}

	// Parse numeric string to decimal
	balance, err := decimal.NewFromString(balanceStr)
	if err != nil {
		return Account{}, fmt.Errorf("failed to parse balance: %w", err)
	}
	account.Balance = balance

	return account, nil
}
