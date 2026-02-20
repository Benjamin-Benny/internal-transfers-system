# Internal Transfers System

A high-performance HTTP API for managing internal account transfers with atomic transaction guarantees and precise decimal handling.

## Quick Start

### Prerequisites

- **Go 1.20+** - [Download](https://go.dev/dl/)
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
# 1. Clone and install dependencies
go mod download

# 2. Start PostgreSQL
make db-up

# 3. Configure environment
cp .env.example .env

# 4. Run migrations
make migrate-up

# 5. Start server
make run
```

The server runs on `http://localhost:8080` by default.

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

# Run with integration tests enabled
RUN_INTEGRATION=1 DATABASE_URL="postgres://app:app@localhost:5432/transfers?sslmode=disable" go test ./... -v
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
├── cmd/server/          # Application entrypoint
├── internal/
│   ├── config/          # Configuration management
│   ├── db/              # Database connection pool
│   ├── domain/          # Domain models and business logic
│   ├── http/            # HTTP server, handlers, middleware
│   ├── repo/            # Data access layer
│   └── service/         # Business services
├── migrations/          # SQL migrations
└── docker-compose.yml   # PostgreSQL container
```
