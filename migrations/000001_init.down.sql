-- Drop transactions table first (due to foreign key constraints)
DROP TABLE IF EXISTS transactions;

-- Drop trigger from accounts table
DROP TRIGGER IF EXISTS trigger_accounts_updated_at ON accounts;

-- Drop accounts table
DROP TABLE IF EXISTS accounts;

-- Drop the updated_at function
DROP FUNCTION IF EXISTS set_updated_at();
