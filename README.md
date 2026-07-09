# Internal Transfers System

A high-performance HTTP API for managing internal account transfers with atomic transaction guarantees and precise decimal handling.

## Assumptions

These reflect what the code actually enforces:

- **Single shared currency.** Accounts and amounts carry no currency field; every balance and
  transfer is in one implicit currency.
- **No authentication or authorization.** The service is intended to run inside a trusted
  internal network; no endpoint performs auth.
- **Client-supplied account IDs.** `account_id` is provided by the caller when creating an
  account (not server-generated), must be a positive integer, and is the primary key.
- **Fixed-scale decimal money.** Amounts and balances allow up to 5 decimal places and are
  stored and returned normalized to exactly 5 (e.g. `"100.00000"`). Values are exchanged as
  JSON strings; scientific notation and more than 5 decimal places are rejected as
  `invalid_input`.
- **Non-negative balances.** `initial_balance` may be zero but not negative, and a transfer
  cannot overdraw its source; a `CHECK (balance >= 0)` constraint backs the in-code funds check.
- **Positive transfer amounts.** A transfer `amount` must be greater than zero.
- **Accounts must pre-exist.** A transfer never creates accounts; if the source or destination
  is missing it returns `not_found` (404). Source and destination must differ.
- **Idempotency is opt-in.** `POST /transactions` is deduplicated only when an `Idempotency-Key`
  header is sent; without it the transfer runs normally and returns `204`.
- **Create and read only.** Accounts and transactions can be created and read; there are no
  update, delete, or list endpoints.

## Quick Start

### Prerequisites

- **Go 1.25.6+** - [Download](https://go.dev/dl/)
- **Docker & Docker Compose** - [Get Docker](https://docs.docker.com/get-docker/)
- **golang-migrate** - Database migration tool

**Install golang-migrate:**

```bash
# macOS
brew install golang-migrate

# Linux
curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xvz
sudo mv migrate /usr/local/bin/
```

### Setup

```bash
# 1. Install dependencies (run from the project root)
go mod download

# 2. Start PostgreSQL
make db-up

# 3. Run migrations (Make auto-creates .env from .env.example on first run)
make migrate-up

# 4. Start server
make run
```

The `make` targets load `.env` and create it from `.env.example` automatically the first time,
so no manual setup is needed. To customize configuration, edit `.env` (or copy it yourself with
`cp .env.example .env` before editing). The server runs on `http://localhost:8080` by default.

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make run` | Run the application |
| `make test` | Run all tests |
| `make db-up` | Start PostgreSQL container |
| `make db-down` | Stop PostgreSQL container |
| `make migrate-up` | Apply database migrations |
| `make migrate-down` | Rollback last migration |
| `make migrate-verify` | Run migrations and verify schema |
| `make tidy` | Tidy Go modules |
| `make help` | Show all available commands |

## API Reference

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/accounts` | Create account |
| GET | `/accounts/{id}` | Get account balance |
| POST | `/transactions` | Transfer funds |

### Examples

**Health Check**

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

**Create Account**

```bash
curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"account_id": 101, "initial_balance": "1000.00000"}'
# HTTP 204 No Content
```

**Get Account Balance**

```bash
curl http://localhost:8080/accounts/101
# {"account_id":101,"balance":"1000.00000"}
```

**Transfer Funds**

```bash
# Create two accounts
curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"account_id": 101, "initial_balance": "1000.00000"}'

curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"account_id": 102, "initial_balance": "500.00000"}'

# Transfer 250.75 from account 101 to 102
curl -X POST http://localhost:8080/transactions \
  -H "Content-Type: application/json" \
  -d '{
    "source_account_id": 101,
    "destination_account_id": 102,
    "amount": "250.75000"
  }'
# HTTP 204 No Content

# Verify balances
curl http://localhost:8080/accounts/101
# {"account_id":101,"balance":"749.25000"}

curl http://localhost:8080/accounts/102
# {"account_id":102,"balance":"750.75000"}
```

### Idempotent Transfers (optional extension)

`POST /transactions` accepts an **optional** `Idempotency-Key` header so a retried request
never double-applies a transfer. Behavior without the header is unchanged (`204 No Content`).

```bash
# With a key, the transfer is deduplicated and the created transaction id is returned (200).
curl -X POST http://localhost:8080/transactions \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 7c1f...-unique-per-attempt" \
  -d '{"source_account_id": 101, "destination_account_id": 102, "amount": "250.75000"}'
# HTTP 200 {"id": 42}

# Retrying the SAME key with the SAME body returns the SAME id and moves no money again.
# (identical command as above)
# HTTP 200 {"id": 42}

# Reusing the SAME key with a DIFFERENT body is rejected.
curl -X POST http://localhost:8080/transactions \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 7c1f...-unique-per-attempt" \
  -d '{"source_account_id": 101, "destination_account_id": 102, "amount": "999.00000"}'
# HTTP 409 {"error":"conflict"}
```

**How it works:** an `idempotency_keys` table (`key` PK → `transaction_id`) is written inside
the *same* database transaction as the transfer, so the dedup decision and the money movement
commit atomically. On a repeat key the original transaction is looked up and its payload is
compared against the retry (via the foreign key to `transactions`); a match returns the original
id, a mismatch returns `409 conflict`.

**Tradeoff (intentional):** this goes beyond the base transfer endpoint. It is included because
retries are unavoidable in payments — a client that times out and retries must not move money
twice — and the cost is small: one nullable-free table, one header, and an additive code path
that leaves the no-key behavior (and the base API contract) exactly as before.

### Error Examples

**Account Not Found (404)**

```bash
curl http://localhost:8080/accounts/999
# {"error":"not_found"}
```

**Insufficient Funds (409)**

```bash
curl -X POST http://localhost:8080/transactions \
  -H "Content-Type: application/json" \
  -d '{
    "source_account_id": 101,
    "destination_account_id": 102,
    "amount": "999999.00000"
  }'
# {"error":"insufficient_funds"}
```

**Account Already Exists (409)**

```bash
curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"account_id": 101, "initial_balance": "100.00000"}'
# {"error":"conflict"}
```

**Invalid Input (400)**

```bash
# Negative balance
curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"account_id": 103, "initial_balance": "-50.00000"}'
# {"error":"invalid_input"}

# Invalid account ID
curl http://localhost:8080/accounts/abc
# {"error":"invalid_account_id"}

# Malformed JSON
curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"account_id": 104'
# {"error":"invalid_json"}
```

## Testing

### Run All Tests

```bash
make test
```

### Test Types

**Unit Tests**
- Test individual components in isolation
- No external dependencies required
- Files: `*_test.go` (excluding `*_integration_test.go`)
- Always run by default

**Integration Tests**
- Test full request/response cycle through all layers
- Require PostgreSQL running via Docker
- Files: `*_integration_test.go`
- **Opt-in only**: Set `RUN_INTEGRATION=1` and `DATABASE_URL` to run

### Running Integration Tests

```bash
# Start PostgreSQL first
make db-up
make migrate-up

# Run with integration tests enabled.
# -p 1 runs the package test binaries serially: the integration tests share a single
# Postgres instance, so running them in parallel would let one package's cleanup race
# another package's inserts.
RUN_INTEGRATION=1 DATABASE_URL="postgres://app:app@localhost:5432/transfers?sslmode=disable" go test -race -p 1 ./... -v
```

Without `RUN_INTEGRATION=1`, integration tests are automatically skipped.

## Configuration

Configure via environment variables (see `.env.example`):

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | *required* | PostgreSQL connection string |
| `PORT` | `8080` | HTTP server port |
| `ENV` | `local` | Environment: local, development, production |

## Project Structure

```
.
├── cmd/server/          # Application entrypoint (main.go)
├── internal/
│   ├── config/          # Configuration from environment variables
│   ├── db/              # PostgreSQL connection pool
│   ├── http/            # HTTP server, router, handlers, middleware
│   ├── repo/            # Data access layer (SQL, transactions)
│   └── service/         # Business logic and validation
├── migrations/          # SQL migrations (golang-migrate)
└── docker-compose.yml   # PostgreSQL container
```
