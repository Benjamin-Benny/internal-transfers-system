package service

import (
	"context"

	"github.com/benjaminbenny/internal-transfers-system/internal/repo"
	"github.com/shopspring/decimal"
)

// AccountRepo defines the repository interface that the service depends on
type AccountRepo interface {
	CreateAccount(ctx context.Context, accountID int64, initialBalance decimal.Decimal) error
	GetAccount(ctx context.Context, accountID int64) (repo.Account, error)
	TransferTx(ctx context.Context, sourceID, destID int64, amount decimal.Decimal) error
	TransferTxIdempotent(ctx context.Context, sourceID, destID int64, amount decimal.Decimal, key string) (int64, error)
}
