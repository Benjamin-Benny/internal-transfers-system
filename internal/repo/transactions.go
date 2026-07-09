package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

// rowQuerier is satisfied by both *pgxpool.Pool and pgx.Tx, so the idempotency lookup can run
// either inside the transfer transaction or on its own connection.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// idempotencyLookupSQL fetches the transaction a prior key produced, joined to the transactions
// row so we can compare the original payload against the retry.
const idempotencyLookupSQL = `
	SELECT ik.transaction_id, t.source_account_id, t.destination_account_id, t.amount
	FROM idempotency_keys ik
	JOIN transactions t ON t.id = ik.transaction_id
	WHERE ik.key = $1`

// TransferTx performs an atomic transfer with no idempotency. Behavior is unchanged from the
// original contract; it now delegates the core steps to doTransfer.
func (r *Repo) TransferTx(ctx context.Context, sourceID, destID int64, amount decimal.Decimal) error {
	if sourceID == destID {
		return ErrSameAccount
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("%w: amount must be positive", ErrInvalidAmount)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := r.doTransfer(ctx, tx, sourceID, destID, amount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// TransferTxIdempotent performs a transfer that is safe to retry under a client-supplied key.
// The key lookup, the transfer, and the key insert all happen in ONE database transaction so the
// dedup decision and the money movement commit (or roll back) atomically.
func (r *Repo) TransferTxIdempotent(ctx context.Context, sourceID, destID int64, amount decimal.Decimal, key string) (int64, error) {
	if sourceID == destID {
		return 0, ErrSameAccount
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return 0, fmt.Errorf("%w: amount must be positive", ErrInvalidAmount)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1) Has this key already produced a transaction? If so, either return the original id
	//    (exact retry) or reject a payload mismatch — either way, move no money.
	existingID, found, err := lookupIdempotent(ctx, tx, key, sourceID, destID, amount)
	if err != nil {
		return 0, err // includes ErrIdempotencyKeyConflict on a payload mismatch
	}
	if found {
		return existingID, nil // read-only path; defer rolls back the empty tx
	}

	// 2) First time we've seen this key: run the transfer in this same tx.
	txID, err := r.doTransfer(ctx, tx, sourceID, destID, amount)
	if err != nil {
		return 0, err
	}

	// 3) Claim the key. The PK makes the claim atomic; a unique violation means a concurrent
	//    identical request beat us to it, so we undo our money movement and return their result.
	_, err = tx.Exec(ctx, `INSERT INTO idempotency_keys (key, transaction_id) VALUES ($1, $2)`, key, txID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			_ = tx.Rollback(ctx) // undo this transfer before resolving the winner
			winnerID, ok, rerr := lookupIdempotent(ctx, r.pool, key, sourceID, destID, amount)
			if rerr != nil {
				return 0, rerr
			}
			if !ok {
				return 0, fmt.Errorf("idempotency key %q not resolvable after conflict", key)
			}
			return winnerID, nil
		}
		return 0, fmt.Errorf("failed to store idempotency key: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return txID, nil
}

// lookupIdempotent resolves an existing key. It returns found=false when the key is new,
// found=true with the original transaction id on an exact retry, and ErrIdempotencyKeyConflict
// when the key was reused with a different source/dest/amount.
func lookupIdempotent(ctx context.Context, q rowQuerier, key string, sourceID, destID int64, amount decimal.Decimal) (int64, bool, error) {
	var existingID, exSrc, exDst int64
	var exAmountStr string
	err := q.QueryRow(ctx, idempotencyLookupSQL, key).Scan(&existingID, &exSrc, &exDst, &exAmountStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to look up idempotency key: %w", err)
	}
	// A malformed stored amount must fail loudly, never be coerced to zero.
	exAmount, err := decimal.NewFromString(exAmountStr)
	if err != nil {
		return 0, false, fmt.Errorf("failed to parse stored amount: %w", err)
	}
	if exSrc != sourceID || exDst != destID || !exAmount.Equal(amount) {
		return 0, true, ErrIdempotencyKeyConflict
	}
	return existingID, true, nil
}

// doTransfer runs the locked read-modify-write of a transfer on an existing tx and returns the
// new transaction id. Callers own the tx lifecycle (begin/commit/rollback).
func (r *Repo) doTransfer(ctx context.Context, tx pgx.Tx, sourceID, destID int64, amount decimal.Decimal) (int64, error) {
	// Lock accounts in deterministic order (smaller ID first) to avoid deadlocks.
	firstID, secondID := sourceID, destID
	if destID < sourceID {
		firstID, secondID = destID, sourceID
	}

	var firstAccountID int64
	var firstBalanceStr string
	query := `SELECT account_id, balance FROM accounts WHERE account_id = $1 FOR UPDATE`
	if err := tx.QueryRow(ctx, query, firstID).Scan(&firstAccountID, &firstBalanceStr); err != nil {
		// errors.Is (not ==) so a wrapped ErrNoRows still maps to 404, not a generic 500
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrAccountNotFound
		}
		return 0, fmt.Errorf("failed to lock account %d: %w", firstID, err)
	}

	var secondAccountID int64
	var secondBalanceStr string
	if err := tx.QueryRow(ctx, query, secondID).Scan(&secondAccountID, &secondBalanceStr); err != nil {
		// errors.Is (not ==) so a wrapped ErrNoRows still maps to 404, not a generic 500
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrAccountNotFound
		}
		return 0, fmt.Errorf("failed to lock account %d: %w", secondID, err)
	}

	// Map locked accounts back to source and destination.
	sourceBalanceStr := firstBalanceStr
	destBalanceStr := secondBalanceStr
	if firstID != sourceID {
		sourceBalanceStr = secondBalanceStr
		destBalanceStr = firstBalanceStr
	}

	// Parse balances; a malformed DB NUMERIC must abort the transfer, never silently become zero.
	sourceBalance, err := decimal.NewFromString(sourceBalanceStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse source balance: %w", err)
	}
	destBalance, err := decimal.NewFromString(destBalanceStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse destination balance: %w", err)
	}

	if sourceBalance.LessThan(amount) {
		return 0, ErrInsufficientFunds
	}

	newSourceBalance := sourceBalance.Sub(amount)
	newDestBalance := destBalance.Add(amount)

	updateQuery := `UPDATE accounts SET balance = $1 WHERE account_id = $2`
	if _, err := tx.Exec(ctx, updateQuery, newSourceBalance.StringFixed(5), sourceID); err != nil {
		return 0, fmt.Errorf("failed to update source account: %w", err)
	}
	if _, err := tx.Exec(ctx, updateQuery, newDestBalance.StringFixed(5), destID); err != nil {
		return 0, fmt.Errorf("failed to update destination account: %w", err)
	}

	// RETURNING id so callers (and the idempotency layer) can record and echo the ledger id.
	var txID int64
	insertQuery := `INSERT INTO transactions (source_account_id, destination_account_id, amount)
	                VALUES ($1, $2, $3) RETURNING id`
	if err := tx.QueryRow(ctx, insertQuery, sourceID, destID, amount.StringFixed(5)).Scan(&txID); err != nil {
		return 0, fmt.Errorf("failed to insert transaction log: %w", err)
	}

	return txID, nil
}
