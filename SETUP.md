# Setup Instructions

This document provides step-by-step instructions to get the Internal Transfers System running.

## Prerequisites

1. **Go 1.20 or higher** - Check with `go version`
2. **Docker and Docker Compose** - For PostgreSQL
3. **golang-migrate CLI** (optional, for migrations)
   - macOS: `brew install golang-migrate`
   - Linux: See [installation guide](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate)

## Step 1: Download Dependencies

Due to Go module dependencies, you need to download the required packages first:

```bash
go mod download
```

Or simply:

```bash
make tidy
```

If you encounter TLS certificate errors, try:

```bash
# Temporarily disable checksum verification
go env -w GOSUMDB=off
go mod tidy
go env -w GOSUMDB=sum.golang.org
```

## Step 2: Verify Build

Ensure the project compiles:

```bash
go build ./cmd/server
```

You should see a `server` binary in the root directory.

## Step 3: Set Up Environment

Copy the example environment file:

```bash
cp .env.example .env
```

The default values should work for local development:
- `DATABASE_URL=postgres://app:app@localhost:5432/transfers?sslmode=disable`
- `PORT=8080`
- `ENV=development`

## Step 4: Start PostgreSQL

Use Docker Compose to start a PostgreSQL container:

```bash
make db-up
```

Verify it's running:

```bash
docker ps
```

You should see a container named `transfers-postgres`.

## Step 5: Run Database Migrations

If you have migrations to run:

```bash
make migrate-up
```

(Currently, the migrations folder is empty, so this step is optional for now)

## Step 6: Start the Server

Run the server:

```bash
make run
```

Or directly:

```bash
go run ./cmd/server/main.go
```

You should see log output indicating the server has started:

```json
{"level":"INFO","msg":"configuration loaded","port":"8080","env":"local"}
{"level":"INFO","msg":"database connection established"}
{"level":"INFO","msg":"starting HTTP server","port":"8080"}
```

## Step 7: Test the Endpoints

In a new terminal, test the health endpoint:

```bash
curl http://localhost:8080/health
```

Expected response:
```json
{"status":"ok"}
```

Test other endpoints (currently not implemented):

```bash
# Create account (returns 501)
curl -X POST http://localhost:8080/accounts

# Get account (returns 501)
curl http://localhost:8080/accounts/123

# Create transaction (returns 501)
curl -X POST http://localhost:8080/transactions
```

## Step 8: Graceful Shutdown

To stop the server gracefully, press `Ctrl+C` in the terminal where the server is running. You should see:

```json
{"level":"INFO","msg":"shutdown signal received"}
{"level":"INFO","msg":"shutting down server..."}
{"level":"INFO","msg":"server stopped gracefully"}
```

## Troubleshooting

### Port Already in Use

If port 8080 is already in use, change the PORT in your `.env` file:

```bash
PORT=9000
```

### Database Connection Failed

Ensure PostgreSQL is running:

```bash
docker ps
```

Check the logs:

```bash
docker logs transfers-postgres
```

### Dependencies Not Downloaded

If you see "missing go.sum entry" errors, run:

```bash
make tidy
```

## Development Workflow

```bash
# Start PostgreSQL
make db-up

# Run the server (in one terminal)
make run

# Run tests (in another terminal)
make test

# When done, stop PostgreSQL
make db-down
```

## Clean Up

To remove all Docker containers and volumes:

```bash
docker-compose down -v
```

This will delete all data in the PostgreSQL database.
