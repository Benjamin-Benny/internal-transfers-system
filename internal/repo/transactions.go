package repo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func (r *Repo) TransferTx(ctx context.Context, sourceID, destID int64, amount decimal.Decimal) error {
	// Validation
	if sourceID == destID {
		return ErrSameAccount
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("%w: amount must be positive", ErrInvalidAmount)
	}

	// Begin transaction
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock accounts in deterministic order (smaller ID first) to avoid deadlocks
	firstID, secondID := sourceID, destID
	if destID < sourceID {
		firstID, secondID = destID, sourceID
	}

	// Lock first account
	var firstAccountID int64
	var firstBalanceStr string
	query := `SELECT account_id, balance FROM accounts WHERE account_id = $1 FOR UPDATE`
	err = tx.QueryRow(ctx, query, firstID).Scan(&firstAccountID, &firstBalanceStr)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrAccountNotFound
		}
		return fmt.Errorf("failed to lock account %d: %w", firstID, err)
	}

	// Lock second account
	var secondAccountID int64
	var secondBalanceStr string
	err = tx.QueryRow(ctx, query, secondID).Scan(&secondAccountID, &secondBalanceStr)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrAccountNotFound
		}
		return fmt.Errorf("failed to lock account %d: %w", secondID, err)
	}

	// Map locked accounts back to source and destination
	sourceBalanceStr := firstBalanceStr
	destBalanceStr := secondBalanceStr
	if firstID != sourceID {
		// We locked dest first, so swap
		sourceBalanceStr = secondBalanceStr
		destBalanceStr = firstBalanceStr
	}

	// Parse balances
	sourceBalance, err := decimal.NewFromString(sourceBalanceStr)
	if err != nil {
		return fmt.Errorf("failed to parse source balance: %w", err)
	}
	destBalance, err := decimal.NewFromString(destBalanceStr)
	if err != nil {
		return fmt.Errorf("failed to parse destination balance: %w", err)
	}

	// Check sufficient funds
	if sourceBalance.LessThan(amount) {
		return ErrInsufficientFunds
	}

	// Calculate new balances
	newSourceBalance := sourceBalance.Sub(amount)
	newDestBalance := destBalance.Add(amount)

	// Update source account
	updateQuery := `UPDATE accounts SET balance = $1 WHERE account_id = $2`
	_, err = tx.Exec(ctx, updateQuery, newSourceBalance.StringFixed(5), sourceID)
	if err != nil {
		return fmt.Errorf("failed to update source account: %w", err)
	}

	// Update destination account
	_, err = tx.Exec(ctx, updateQuery, newDestBalance.StringFixed(5), destID)
	if err != nil {
		return fmt.Errorf("failed to update destination account: %w", err)
	}

	// Insert transaction log
	insertQuery := `INSERT INTO transactions (source_account_id, destination_account_id, amount) 
	                VALUES ($1, $2, $3)`
	_, err = tx.Exec(ctx, insertQuery, sourceID, destID, amount.StringFixed(5))
	if err != nil {
		return fmt.Errorf("failed to insert transaction log: %w", err)
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
