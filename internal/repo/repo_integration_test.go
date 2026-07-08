package repo

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepo(t *testing.T) (*Repo, func()) {
	t.Helper()

	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION=1 to run.")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Skipping integration test. DATABASE_URL not set.")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err, "failed to connect to database")

	// Clean tables before test
	_, err = pool.Exec(ctx, "DELETE FROM transactions")
	require.NoError(t, err, "failed to clean transactions table")
	_, err = pool.Exec(ctx, "DELETE FROM accounts")
	require.NoError(t, err, "failed to clean accounts table")

	repo := New(pool)

	cleanup := func() {
		pool.Close()
	}

	return repo, cleanup
}

func TestCreateAccountAndGetAccount(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	accountID := int64(1001)
	initialBalance := decimal.NewFromFloat(100.50000).Truncate(5)

	// Create account
	err := repo.CreateAccount(ctx, accountID, initialBalance)
	require.NoError(t, err, "CreateAccount should succeed")

	// Get account
	account, err := repo.GetAccount(ctx, accountID)
	require.NoError(t, err, "GetAccount should succeed")

	assert.Equal(t, accountID, account.AccountID)
	assert.True(t, initialBalance.Equal(account.Balance), "balance should match: expected %s, got %s", initialBalance.String(), account.Balance.String())
}

func TestCreateAccountDuplicate(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	accountID := int64(2001)
	initialBalance := decimal.NewFromFloat(100.00000).Truncate(5)

	// Create account first time
	err := repo.CreateAccount(ctx, accountID, initialBalance)
	require.NoError(t, err, "first CreateAccount should succeed")

	// Try to create duplicate
	err = repo.CreateAccount(ctx, accountID, initialBalance)
	assert.ErrorIs(t, err, ErrAccountAlreadyExists, "should return ErrAccountAlreadyExists")
}

func TestGetAccountNotFound(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	accountID := int64(9999)

	// Try to get non-existent account
	_, err := repo.GetAccount(ctx, accountID)
	assert.ErrorIs(t, err, ErrAccountNotFound, "should return ErrAccountNotFound")
}

func TestTransferTxSuccess(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	sourceID := int64(3001)
	destID := int64(3002)
	sourceBalance := decimal.NewFromFloat(1000.00000).Truncate(5)
	destBalance := decimal.NewFromFloat(500.00000).Truncate(5)
	transferAmount := decimal.NewFromFloat(250.50000).Truncate(5)

	// Create accounts
	err := repo.CreateAccount(ctx, sourceID, sourceBalance)
	require.NoError(t, err)
	err = repo.CreateAccount(ctx, destID, destBalance)
	require.NoError(t, err)

	// Execute transfer
	err = repo.TransferTx(ctx, sourceID, destID, transferAmount)
	require.NoError(t, err, "TransferTx should succeed")

	// Verify source balance
	sourceAccount, err := repo.GetAccount(ctx, sourceID)
	require.NoError(t, err)
	expectedSourceBalance := sourceBalance.Sub(transferAmount)
	assert.True(t, expectedSourceBalance.Equal(sourceAccount.Balance),
		"source balance should be %s, got %s", expectedSourceBalance.String(), sourceAccount.Balance.String())

	// Verify destination balance
	destAccount, err := repo.GetAccount(ctx, destID)
	require.NoError(t, err)
	expectedDestBalance := destBalance.Add(transferAmount)
	assert.True(t, expectedDestBalance.Equal(destAccount.Balance),
		"dest balance should be %s, got %s", expectedDestBalance.String(), destAccount.Balance.String())

	// Verify transaction log entry exists
	var count int
	err = repo.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM transactions WHERE source_account_id = $1 AND destination_account_id = $2",
		sourceID, destID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "should have one transaction log entry")
}

func TestTransferTxInsufficientFunds(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	sourceID := int64(4001)
	destID := int64(4002)
	sourceBalance := decimal.NewFromFloat(100.00000).Truncate(5)
	destBalance := decimal.NewFromFloat(500.00000).Truncate(5)
	transferAmount := decimal.NewFromFloat(200.00000).Truncate(5) // More than source has

	// Create accounts
	err := repo.CreateAccount(ctx, sourceID, sourceBalance)
	require.NoError(t, err)
	err = repo.CreateAccount(ctx, destID, destBalance)
	require.NoError(t, err)

	// Try transfer with insufficient funds
	err = repo.TransferTx(ctx, sourceID, destID, transferAmount)
	assert.ErrorIs(t, err, ErrInsufficientFunds, "should return ErrInsufficientFunds")

	// Verify balances unchanged
	sourceAccount, err := repo.GetAccount(ctx, sourceID)
	require.NoError(t, err)
	assert.True(t, sourceBalance.Equal(sourceAccount.Balance), "source balance should be unchanged")

	destAccount, err := repo.GetAccount(ctx, destID)
	require.NoError(t, err)
	assert.True(t, destBalance.Equal(destAccount.Balance), "dest balance should be unchanged")

	// Verify no transaction log entry
	var count int
	err = repo.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM transactions WHERE source_account_id = $1 AND destination_account_id = $2",
		sourceID, destID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "should have no transaction log entry")
}

func TestTransferTxMissingAccount(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	sourceID := int64(5001)
	destID := int64(5002)
	nonExistentID := int64(9999)
	balance := decimal.NewFromFloat(1000.00000).Truncate(5)
	transferAmount := decimal.NewFromFloat(100.00000).Truncate(5)

	// Create only one account
	err := repo.CreateAccount(ctx, sourceID, balance)
	require.NoError(t, err)

	// Try transfer to non-existent destination
	err = repo.TransferTx(ctx, sourceID, nonExistentID, transferAmount)
	assert.ErrorIs(t, err, ErrAccountNotFound, "should return ErrAccountNotFound for missing destination")

	// Try transfer from non-existent source
	err = repo.CreateAccount(ctx, destID, balance)
	require.NoError(t, err)
	err = repo.TransferTx(ctx, nonExistentID, destID, transferAmount)
	assert.ErrorIs(t, err, ErrAccountNotFound, "should return ErrAccountNotFound for missing source")
}

func TestTransferTxSameAccount(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	accountID := int64(6001)
	balance := decimal.NewFromFloat(1000.00000).Truncate(5)
	transferAmount := decimal.NewFromFloat(100.00000).Truncate(5)

	// Create account
	err := repo.CreateAccount(ctx, accountID, balance)
	require.NoError(t, err)

	// Try transfer to same account
	err = repo.TransferTx(ctx, accountID, accountID, transferAmount)
	assert.ErrorIs(t, err, ErrSameAccount, "should return ErrSameAccount")
}

func TestTransferTxInvalidAmount(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	sourceID := int64(7001)
	destID := int64(7002)
	balance := decimal.NewFromFloat(1000.00000).Truncate(5)

	// Create accounts
	err := repo.CreateAccount(ctx, sourceID, balance)
	require.NoError(t, err)
	err = repo.CreateAccount(ctx, destID, balance)
	require.NoError(t, err)

	// Try transfer with zero amount
	err = repo.TransferTx(ctx, sourceID, destID, decimal.Zero)
	assert.ErrorIs(t, err, ErrInvalidAmount, "should return ErrInvalidAmount for zero amount")

	// Try transfer with negative amount
	err = repo.TransferTx(ctx, sourceID, destID, decimal.NewFromFloat(-100))
	assert.ErrorIs(t, err, ErrInvalidAmount, "should return ErrInvalidAmount for negative amount")
}

func TestTransferTxConcurrency(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	// Use unique account IDs and cleanup before test (already done by setupTestRepo)
	sourceID := int64(8001)
	destID := int64(8002)
	initialSourceBalance := decimal.NewFromFloat(1000.00000).Truncate(5)
	initialDestBalance := decimal.NewFromFloat(0.00000).Truncate(5)
	transferAmount := decimal.NewFromFloat(10.00000).Truncate(5)

	// Create accounts
	err := repo.CreateAccount(ctx, sourceID, initialSourceBalance)
	require.NoError(t, err)
	err = repo.CreateAccount(ctx, destID, initialDestBalance)
	require.NoError(t, err)

	// Get initial transaction count for these accounts
	var initialTxCount int
	err = repo.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM transactions WHERE (source_account_id = $1 AND destination_account_id = $2) OR (source_account_id = $2 AND destination_account_id = $1)",
		sourceID, destID).Scan(&initialTxCount)
	require.NoError(t, err)

	// Run concurrent transfers
	numTransfers := 50
	var wg sync.WaitGroup
	errors := make(chan error, numTransfers)

	for i := 0; i < numTransfers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.TransferTx(ctx, sourceID, destID, transferAmount)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Logf("Transfer error: %v", err)
	}

	// Verify final balances
	sourceAccount, err := repo.GetAccount(ctx, sourceID)
	require.NoError(t, err)
	destAccount, err := repo.GetAccount(ctx, destID)
	require.NoError(t, err)

	// Count successful transfers for these specific accounts
	var finalTxCount int
	err = repo.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM transactions WHERE source_account_id = $1 AND destination_account_id = $2",
		sourceID, destID).Scan(&finalTxCount)
	require.NoError(t, err)

	successfulTransfers := finalTxCount - initialTxCount
	N := int64(successfulTransfers)

	// Assert: transactions row count increased by N
	assert.Equal(t, numTransfers, successfulTransfers,
		"should have completed all %d transfers successfully", numTransfers)

	// Assert: final source balance = initial - (N * amount)
	expectedSourceBalance := initialSourceBalance.Sub(transferAmount.Mul(decimal.NewFromInt(N)))
	assert.True(t, expectedSourceBalance.Equal(sourceAccount.Balance),
		"source balance should be initial - (N * amount): expected %s (1000 - %d*10), got %s",
		expectedSourceBalance.String(), N, sourceAccount.Balance.String())

	// Assert: final dest balance = initial + (N * amount)
	expectedDestBalance := initialDestBalance.Add(transferAmount.Mul(decimal.NewFromInt(N)))
	assert.True(t, expectedDestBalance.Equal(destAccount.Balance),
		"dest balance should be initial + (N * amount): expected %s (0 + %d*10), got %s",
		expectedDestBalance.String(), N, destAccount.Balance.String())

	// Assert: total money conserved (sum of both accounts unchanged)
	totalBalance := sourceAccount.Balance.Add(destAccount.Balance)
	expectedTotal := initialSourceBalance.Add(initialDestBalance)
	assert.True(t, expectedTotal.Equal(totalBalance),
		"total money should be conserved: expected %s, got %s",
		expectedTotal.String(), totalBalance.String())

	// Additional safety check: source should never be negative
	assert.False(t, sourceAccount.Balance.IsNegative(),
		"source balance should never be negative, got %s", sourceAccount.Balance.String())

	t.Logf("Successfully completed all %d concurrent transfers", successfulTransfers)
	t.Logf("Source: %s -> %s (decreased by %s)",
		initialSourceBalance.String(), sourceAccount.Balance.String(),
		initialSourceBalance.Sub(sourceAccount.Balance).String())
	t.Logf("Dest: %s -> %s (increased by %s)",
		initialDestBalance.String(), destAccount.Balance.String(),
		destAccount.Balance.Sub(initialDestBalance).String())
}

func TestCreateAccountValidation(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	// Test invalid account ID
	err := repo.CreateAccount(ctx, 0, decimal.NewFromFloat(100))
	assert.ErrorIs(t, err, ErrInvalidAmount, "should reject zero account ID")

	err = repo.CreateAccount(ctx, -1, decimal.NewFromFloat(100))
	assert.ErrorIs(t, err, ErrInvalidAmount, "should reject negative account ID")

	// Test negative balance
	err = repo.CreateAccount(ctx, 9001, decimal.NewFromFloat(-100))
	assert.ErrorIs(t, err, ErrInvalidAmount, "should reject negative balance")
}

// Why ReadCommitted + FOR UPDATE is sufficient for these tests (and for the money engine):
//
// Every TransferTx does `SELECT ... FOR UPDATE` on BOTH account rows before reading their
// balances, so it holds a row-level write lock across the whole check-funds -> debit -> credit
// sequence. Under READ COMMITTED, a statement sees the latest *committed* data, and FOR UPDATE
// blocks any other transaction from reading-for-update or writing those same rows until we
// commit or roll back. That serializes the read-modify-write per account: no lost updates, no
// dirty reads. We do NOT need SERIALIZABLE because a transfer never makes a decision over a
// *set* of rows that could change underneath it (no phantom/range problem) — it touches exactly
// two explicitly locked rows. Finally, locking the lower account id first gives every transfer
// the same global lock order, so concurrent transfers can never form a lock cycle => no DB
// deadlock. These two tests exercise exactly those guarantees under real concurrency.

// TestTransferConcurrencyMoneyIntegrity is the strongest statement we can make about the money
// engine: under heavy concurrent load it must neither create nor destroy value, and it must
// never let an account go negative — with no application- or database-level deadlock.
func TestTransferConcurrencyMoneyIntegrity(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	const (
		numAccounts = 10  // K accounts to spread contention across
		numWorkers  = 200 // N concurrent transfers
	)

	// Seed K accounts with a known balance and record the exact starting total. Because
	// transfers only ever move value *between* these accounts, this total is the invariant.
	initialBalance := decimal.NewFromInt(1000).Truncate(5)
	accountIDs := make([]int64, numAccounts)
	expectedTotal := decimal.Zero
	for i := 0; i < numAccounts; i++ {
		id := int64(20000 + i)
		accountIDs[i] = id
		require.NoError(t, repo.CreateAccount(ctx, id, initialBalance))
		expectedTotal = expectedTotal.Add(initialBalance)
	}

	// Launch N goroutines, each doing one random source->dest transfer of a random valid
	// amount. A WaitGroup lets us join them; a buffered channel collects only *unexpected*
	// errors — insufficient_funds is a legitimate business outcome (a source can be drained
	// by concurrent transfers), so it must NOT fail the test.
	var wg sync.WaitGroup
	unexpected := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerSeed int64) {
			defer wg.Done()
			// Per-goroutine RNG: avoids the global rand mutex and any data race.
			rng := rand.New(rand.NewSource(workerSeed))

			// Pick two DISTINCT accounts so we exercise the transfer path, not ErrSameAccount.
			si := rng.Intn(numAccounts)
			di := rng.Intn(numAccounts - 1)
			if di >= si {
				di++
			}
			// Random valid amount in [0.01, 100.00]: decimal.New(v, -2) => v * 10^-2, so at
			// most 2 decimal places (well within the 5-place contract).
			amount := decimal.New(rng.Int63n(10000)+1, -2)

			err := repo.TransferTx(ctx, accountIDs[si], accountIDs[di], amount)
			switch {
			case err == nil:
				// success
			case errors.Is(err, ErrInsufficientFunds):
				// expected business error under contention — tolerated
			default:
				unexpected <- err
			}
		}(int64(i) + 1)
	}

	// Join with a timeout so an *application-level* deadlock (goroutines blocked forever)
	// surfaces as an explicit failure instead of hanging the whole suite.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for transfers — likely a deadlock (application-level or DB)")
	}
	close(unexpected)

	// Assertion (c): NO unexpected error. A Postgres deadlock (SQLSTATE 40P01) would arrive
	// here as a non-nil, non-insufficient-funds error — deterministic lock ordering is what
	// keeps this channel empty.
	for err := range unexpected {
		t.Errorf("unexpected transfer error (a DB deadlock or corruption would surface here): %v", err)
	}

	total := decimal.Zero
	for _, id := range accountIDs {
		acc, err := repo.GetAccount(ctx, id)
		require.NoError(t, err)
		// Assertion (b): NO negative balance. Protected by the in-transaction funds check on
		// the FOR UPDATE-locked row AND, as a backstop, the CHECK (balance >= 0) constraint.
		assert.False(t, acc.Balance.IsNegative(), "account %d went negative: %s", id, acc.Balance.String())
		total = total.Add(acc.Balance)
	}

	// Assertion (a): CONSERVATION. Value only moved between seeded accounts, so the sum of all
	// balances must exactly equal the recorded starting total, regardless of how many transfers
	// succeeded or hit insufficient_funds.
	assert.True(t, expectedTotal.Equal(total),
		"money not conserved: started with %s, ended with %s", expectedTotal.String(), total.String())

	t.Logf("integrity holds: total conserved at %s across %d accounts and %d concurrent transfers; no negative balances; no deadlock",
		total.String(), numAccounts, numWorkers)
}

// TestTransferDeadlockOrdering is the tight, targeted proof that deterministic lock ordering
// works: two accounts transferring to each other in OPPOSITE directions at the same time is the
// classic deadlock setup. Because TransferTx always locks the lower id first, both directions
// acquire locks in the same order and can never form a cycle. Without that ordering this test
// would intermittently hang on a Postgres deadlock; with it, it completes cleanly.
func TestTransferDeadlockOrdering(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()

	a := int64(21001)
	b := int64(21002)
	// Large starting balances so neither side runs dry — we want to test locking, not funds.
	start := decimal.NewFromInt(1000000).Truncate(5)
	require.NoError(t, repo.CreateAccount(ctx, a, start))
	require.NoError(t, repo.CreateAccount(ctx, b, start))

	const iterations = 300
	amount := decimal.NewFromInt(1).Truncate(5)

	var wg sync.WaitGroup
	errs := make(chan error, 2*iterations)

	wg.Add(2)
	go func() { // A -> B, repeatedly
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := repo.TransferTx(ctx, a, b, amount); err != nil && !errors.Is(err, ErrInsufficientFunds) {
				errs <- err
			}
		}
	}()
	go func() { // B -> A, repeatedly — opposite direction, simultaneously
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if err := repo.TransferTx(ctx, b, a, amount); err != nil && !errors.Is(err, ErrInsufficientFunds) {
				errs <- err
			}
		}
	}()

	// Timeout guard: if the ordering were wrong, the two goroutines would deadlock and this
	// WaitGroup would never complete — the timeout converts that hang into a clear failure.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("A<->B transfers timed out — deterministic lock ordering is not preventing deadlock")
	}
	close(errs)

	// Any error here (in particular a Postgres deadlock, SQLSTATE 40P01) means ordering failed.
	for err := range errs {
		t.Errorf("unexpected error during A<->B contention (a DB deadlock would surface here): %v", err)
	}

	// Conservation must still hold under bidirectional contention.
	accA, err := repo.GetAccount(ctx, a)
	require.NoError(t, err)
	accB, err := repo.GetAccount(ctx, b)
	require.NoError(t, err)
	got := accA.Balance.Add(accB.Balance)
	want := start.Add(start)
	assert.True(t, want.Equal(got),
		"money not conserved under A<->B contention: want %s, got %s", want.String(), got.String())

	t.Logf("A<->B ran %d iterations each direction with no deadlock; total conserved at %s", iterations, got.String())
}
