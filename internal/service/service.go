package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/benjaminbenny/internal-transfers-system/internal/repo"
	"github.com/shopspring/decimal"
)

// Account is the service-level DTO for account information
type Account struct {
	AccountID int64
	Balance   string // Always formatted with exactly 5 decimal places
}

// Service handles business logic and validation
type Service struct {
	repo AccountRepo
}

// New creates a new Service instance
func New(r AccountRepo) *Service {
	return &Service{repo: r}
}

// CreateAccount creates a new account with the given ID and initial balance
func (s *Service) CreateAccount(ctx context.Context, accountID int64, initialBalanceStr string) error {
	// Validate account ID
	if accountID <= 0 {
		return fmt.Errorf("%w: account ID must be positive", ErrInvalidInput)
	}

	// Parse and validate initial balance
	initialBalance, err := ParseAmount(initialBalanceStr)
	if err != nil {
		return err
	}

	// Validate non-negative balance
	if initialBalance.IsNegative() {
		return fmt.Errorf("%w: initial balance cannot be negative", ErrInvalidInput)
	}

	// Call repository
	err = s.repo.CreateAccount(ctx, accountID, initialBalance)
	if err != nil {
		return mapRepoError(err)
	}

	return nil
}

// GetAccount retrieves account information by ID
func (s *Service) GetAccount(ctx context.Context, accountID int64) (Account, error) {
	// Validate account ID
	if accountID <= 0 {
		return Account{}, fmt.Errorf("%w: account ID must be positive", ErrInvalidInput)
	}

	// Get account from repository
	repoAccount, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return Account{}, mapRepoError(err)
	}

	// Convert to service DTO with normalized balance
	return Account{
		AccountID: repoAccount.AccountID,
		Balance:   repoAccount.Balance.StringFixed(5),
	}, nil
}

// validateTransfer applies the shared transfer input rules and returns the parsed amount.
// Both the plain and idempotent paths use it so validation stays identical.
func (s *Service) validateTransfer(sourceID, destID int64, amountStr string) (decimal.Decimal, error) {
	if sourceID <= 0 {
		return decimal.Zero, fmt.Errorf("%w: source account ID must be positive", ErrInvalidInput)
	}
	if destID <= 0 {
		return decimal.Zero, fmt.Errorf("%w: destination account ID must be positive", ErrInvalidInput)
	}
	if sourceID == destID {
		return decimal.Zero, fmt.Errorf("%w: source and destination accounts must be different", ErrInvalidInput)
	}

	amount, err := ParseAmount(amountStr)
	if err != nil {
		return decimal.Zero, err
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("%w: transfer amount must be positive", ErrInvalidInput)
	}
	return amount, nil
}

// CreateTransaction executes a transfer between two accounts (no idempotency).
func (s *Service) CreateTransaction(ctx context.Context, sourceID, destID int64, amountStr string) error {
	amount, err := s.validateTransfer(sourceID, destID, amountStr)
	if err != nil {
		return err
	}

	if err := s.repo.TransferTx(ctx, sourceID, destID, amount); err != nil {
		return mapRepoError(err)
	}
	return nil
}

// CreateTransactionIdempotent executes a transfer that is safe to retry under the given key and
// returns the resulting transaction id. A retry with the same key and payload returns the
// original id without moving money; the same key with a different payload returns ErrConflict.
func (s *Service) CreateTransactionIdempotent(ctx context.Context, sourceID, destID int64, amountStr, key string) (int64, error) {
	amount, err := s.validateTransfer(sourceID, destID, amountStr)
	if err != nil {
		return 0, err
	}

	id, err := s.repo.TransferTxIdempotent(ctx, sourceID, destID, amount, key)
	if err != nil {
		return 0, mapRepoError(err)
	}
	return id, nil
}

// mapRepoError maps repository errors to service-level errors
func mapRepoError(err error) error {
	if err == nil {
		return nil
	}

	// Map known repository errors to service errors
	switch {
	case errors.Is(err, repo.ErrAccountNotFound):
		return ErrNotFound
	case errors.Is(err, repo.ErrAccountAlreadyExists):
		return ErrConflict
	case errors.Is(err, repo.ErrInsufficientFunds):
		return ErrInsufficientFunds
	case errors.Is(err, repo.ErrIdempotencyKeyConflict):
		return ErrConflict
	case errors.Is(err, repo.ErrInvalidAmount):
		return ErrInvalidInput
	case errors.Is(err, repo.ErrSameAccount):
		return ErrInvalidInput
	default:
		// Wrap unknown errors
		return fmt.Errorf("service error: %w", err)
	}
}
